package core

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

// Poll pacing. For the first pollBackoffPeriod the gap is the configured base
// interval; after that it doubles every period, up to maxPollInterval. Long
// waits therefore stay cheap for the status endpoint without making short ones
// sluggish.
const (
	pollBackoffPeriod = 2 * time.Minute
	maxPollInterval   = 15 * time.Second
)

// PaymentInstance tracks one transaction to a terminal state.
//
// Subscribe with On, Once and Off, or block with Wait. Polling starts
// immediately and stops on the first terminal event, so an instance is
// disposable: it is scoped to exactly one transaction.
//
// Registering a handler after the operation returned still delivers events that
// have already fired. Go has no event loop to defer the first emit onto, so
// instead each lifecycle event is remembered and replayed to a late subscriber.
// Every event fires at most once per instance, so a handler sees each event
// exactly once whether it subscribed before or after the fact.
type PaymentInstance struct {
	mu sync.RWMutex

	reference   string
	status      types.TransactionStatus
	transaction *types.Transaction
	err         error

	resolved bool
	// earlyReturn records that polling stopped because the payment was flagged
	// delayed and OnDelayed was "return", so Wait hands back the pending record.
	earlyReturn bool
	// lastStatusEvent dedupes by event rather than by raw status: pending and
	// processing both mean "processing" to a merchant, and a status flap must
	// not re-fire it.
	lastStatusEvent types.PaymentEvent
	// fired remembers each emitted event so a late subscriber still receives it.
	fired map[types.PaymentEvent]types.EventData

	done     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once

	events *emitter

	fetchStatus      func(ctx context.Context, reference string) (*types.StatusResponse, error)
	fetchTransaction func(ctx context.Context, reference string) (*types.Transaction, error)

	pollInterval    time.Duration
	maxPollDuration time.Duration
	maxPollAttempts int
	onDelayed       types.OnDelayedBehavior
}

// NewPaymentInstance builds an instance and begins tracking immediately.
func NewPaymentInstance(cfg PaymentInstanceConfig) *PaymentInstance {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.OnDelayed == "" {
		cfg.OnDelayed = types.OnDelayedWait
	}
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}

	pi := &PaymentInstance{
		reference:        cfg.Reference,
		status:           normalizeStatus(cfg.InitialStatus),
		fired:            make(map[types.PaymentEvent]types.EventData),
		done:             make(chan struct{}),
		stop:             make(chan struct{}),
		events:           newEmitter(),
		fetchStatus:      cfg.FetchStatus,
		fetchTransaction: cfg.FetchTransaction,
		pollInterval:     cfg.PollInterval,
		maxPollDuration:  cfg.MaxPollDuration,
		maxPollAttempts:  cfg.MaxPollAttempts,
		onDelayed:        cfg.OnDelayed,
	}

	// The backend rejected initiation, so there is no transaction to poll. The
	// failure surfaces as an "error" event rather than a returned error: by the
	// time a merchant holds an instance, the event channel is where outcomes
	// arrive.
	if cfg.InitialError != nil {
		go pi.resolveWithError(cfg.InitialError)
		return pi
	}

	go pi.run(ctx)
	return pi
}

// Reference is the transaction reference. It is fixed at creation.
func (pi *PaymentInstance) Reference() string {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.reference
}

// Status is the most recently observed transaction status.
func (pi *PaymentInstance) Status() types.TransactionStatus {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.status
}

// Transaction is the full record, available once a terminal state is reached.
// It is populated even when Wait reports a failure, so the failure reason and
// metadata remain reachable.
func (pi *PaymentInstance) Transaction() *types.Transaction {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.transaction
}

// On registers a handler for an event and returns the instance for chaining.
//
// If the event has already fired, the handler is invoked immediately with the
// original event data, so subscribing after the operation returned is safe.
func (pi *PaymentInstance) On(event types.PaymentEvent, handler types.PaymentEventHandler) *PaymentInstance {
	pi.subscribe(event, handler, false)
	return pi
}

// Once registers a handler that fires at most once, then unsubscribes itself.
func (pi *PaymentInstance) Once(event types.PaymentEvent, handler types.PaymentEventHandler) *PaymentInstance {
	pi.subscribe(event, handler, true)
	return pi
}

// Off removes a handler. Removing one that was never registered is a no-op.
//
// Handlers are matched by function identity. Two distinct closures created from
// the same function literal share that identity, so Off removes both; give a
// handler you intend to remove its own named function or variable.
func (pi *PaymentInstance) Off(event types.PaymentEvent, handler types.PaymentEventHandler) *PaymentInstance {
	pi.events.remove(event, handler)
	return pi
}

// subscribe registers a handler, or replays the event to it if that event has
// already fired.
//
// Both branches are decided under one lock, and they are exclusive. Registering
// first and then checking whether the event had fired would double-deliver to
// any handler that subscribed just as the event landed: once from emit, once
// from the replay. Since every event fires at most once per instance, a handler
// that arrives late needs the replay and nothing else.
func (pi *PaymentInstance) subscribe(event types.PaymentEvent, handler types.PaymentEventHandler, once bool) {
	if handler == nil {
		return
	}

	pi.mu.Lock()
	data, alreadyFired := pi.fired[event]
	if !alreadyFired {
		pi.events.add(event, handler, once)
	}
	pi.mu.Unlock()

	if alreadyFired {
		invokeHandler(handler, data)
	}
}

// Wait blocks until the transaction reaches a terminal state, the instance is
// closed, or ctx is done.
//
// It returns the full transaction on success. On failure, cancellation or a
// lifecycle error it returns a nil transaction and an *SDKError carrying the
// category, so callers can branch without attaching a handler. The record
// itself, including any failure reason, stays available via Transaction.
//
// A payment resolved early because it was flagged delayed returns the
// still-pending transaction with a nil error.
func (pi *PaymentInstance) Wait(ctx context.Context) (*types.Transaction, error) {
	select {
	case <-ctx.Done():
		return nil, &types.SDKError{Category: types.CategoryTimeout, Message: msgCancelled}
	case <-pi.done:
		pi.mu.RLock()
		defer pi.mu.RUnlock()
		if pi.err != nil {
			return nil, pi.err
		}
		return pi.transaction, nil
	}
}

// Close stops polling and releases the instance's goroutine.
//
// Call it when abandoning an instance you are not going to Wait on. An
// unclosed, never-terminal payment would otherwise keep polling for the life of
// the process, spending the account's request budget for nothing. Close is safe
// to call more than once, and safe to call after the instance has resolved.
func (pi *PaymentInstance) Close() {
	pi.stopOnce.Do(func() { close(pi.stop) })
}

// run drives the poll loop until a terminal state, a cap, or cancellation.
func (pi *PaymentInstance) run(ctx context.Context) {
	if pi.status.IsTerminal() {
		pi.handleTerminalState(ctx, pi.status)
		return
	}

	// Announce the in-flight state before any polling. A payment that resolves
	// between polls must still report "processing" before its terminal event.
	if event, ok := statusToEvent(pi.status); ok {
		pi.mu.Lock()
		pi.lastStatusEvent = event
		pi.mu.Unlock()
		pi.emit(event, nil, nil)
	}

	start := time.Now()
	attempts := 0

	for {
		elapsed := time.Since(start)
		if pi.maxPollAttempts > 0 && attempts >= pi.maxPollAttempts {
			pi.resolveWithError(&types.SDKError{Category: types.CategoryTimeout, Message: msgPollTimeout})
			return
		}
		if pi.maxPollDuration > 0 && elapsed >= pi.maxPollDuration {
			pi.resolveWithError(&types.SDKError{Category: types.CategoryTimeout, Message: msgPollTimeout})
			return
		}

		if !pi.sleep(ctx, nextPollDelay(pi.pollInterval, elapsed)) {
			return
		}
		attempts++

		// Exactly one status request is ever in flight: the next is scheduled
		// only after this one returns.
		status, err := pi.fetchStatus(ctx, pi.reference)
		if err != nil {
			if pi.handlePollError(err) {
				return
			}
			continue
		}

		if pi.handleStatus(ctx, status) {
			return
		}
	}
}

// sleep waits out a poll interval, reporting false if the instance should stop.
func (pi *PaymentInstance) sleep(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		pi.resolveWithError(&types.SDKError{Category: types.CategoryTimeout, Message: msgCancelled})
		return false
	case <-pi.stop:
		pi.resolveWithError(&types.SDKError{Category: types.CategoryTimeout, Message: msgCancelled})
		return false
	case <-timer.C:
		return true
	}
}

// handlePollError decides whether a failed status poll ends the instance.
//
// A not_found early on is expected rather than fatal: the transaction may not
// have propagated to the read path yet. Anything else is a real failure.
func (pi *PaymentInstance) handlePollError(err error) bool {
	if sdkErr, ok := err.(*types.SDKError); ok && sdkErr.Category == types.CategoryNotFound {
		return false
	}
	pi.resolveWithError(err)
	return true
}

// handleStatus applies one status response, reporting whether polling is done.
func (pi *PaymentInstance) handleStatus(ctx context.Context, status *types.StatusResponse) bool {
	pi.mu.Lock()
	if pi.resolved {
		pi.mu.Unlock()
		return true
	}
	if status.Reference != pi.reference {
		pi.mu.Unlock()
		pi.resolveWithError(&types.SDKError{
			Category: types.CategoryInternal,
			Message:  "Received a status update for a different transaction",
		})
		return true
	}

	next := normalizeStatus(status.Status)
	pi.status = next
	pi.mu.Unlock()

	if status.Delayed != nil && *status.Delayed && pi.onDelayed == types.OnDelayedReturn && !next.IsTerminal() {
		pi.returnPendingEarly(ctx)
		return true
	}

	event, ok := statusToEvent(next)
	if !ok {
		return false
	}

	pi.mu.Lock()
	duplicate := event == pi.lastStatusEvent
	if !duplicate {
		pi.lastStatusEvent = event
	}
	pi.mu.Unlock()

	if next.IsTerminal() {
		pi.handleTerminalState(ctx, next)
		return true
	}
	if !duplicate {
		pi.emit(event, nil, nil)
	}
	return false
}

// handleTerminalState fetches the full record and emits the terminal event.
func (pi *PaymentInstance) handleTerminalState(ctx context.Context, status types.TransactionStatus) {
	// Fetched outside the lock: this is a network round-trip, and holding the
	// write lock across it would block every accessor for its duration.
	transaction, err := pi.fetchTransaction(ctx, pi.reference)

	pi.mu.Lock()
	if pi.resolved {
		pi.mu.Unlock()
		return
	}
	if err != nil {
		pi.mu.Unlock()
		pi.resolveWithError(err)
		return
	}

	pi.transaction = transaction
	pi.status = status

	var outcome error
	switch status {
	case types.TransactionStatusFailed:
		outcome = &types.SDKError{Category: types.CategoryProvider, Message: failureMessage(transaction)}
	case types.TransactionStatusCancelled:
		outcome = &types.SDKError{Category: types.CategoryProvider, Message: "The payment was cancelled"}
	}
	pi.err = outcome
	pi.mu.Unlock()

	event, ok := statusToEvent(status)
	if !ok {
		event = types.PaymentEventError
	}
	pi.emit(event, transaction, outcome)
	pi.finish()
}

// returnPendingEarly resolves with a still-pending transaction because it was
// flagged delayed and the merchant asked not to keep waiting.
func (pi *PaymentInstance) returnPendingEarly(ctx context.Context) {
	transaction, err := pi.fetchTransaction(ctx, pi.reference)

	pi.mu.Lock()
	if pi.resolved {
		pi.mu.Unlock()
		return
	}
	if err == nil {
		pi.transaction = transaction
	}
	pi.earlyReturn = true
	pi.mu.Unlock()

	pi.finish()
}

// resolveWithError ends the instance with a failure and emits "error".
func (pi *PaymentInstance) resolveWithError(err error) {
	pi.mu.Lock()
	if pi.resolved {
		pi.mu.Unlock()
		return
	}
	pi.err = err
	pi.mu.Unlock()

	pi.emit(types.PaymentEventError, nil, err)
	pi.finish()
}

// finish marks the instance resolved and unblocks Wait. After this point no
// further events are emitted, so a poll still in flight is ignored.
func (pi *PaymentInstance) finish() {
	pi.mu.Lock()
	if pi.resolved {
		pi.mu.Unlock()
		return
	}
	pi.resolved = true
	pi.mu.Unlock()

	pi.Close()
	close(pi.done)
}

// emit records and delivers one lifecycle event.
func (pi *PaymentInstance) emit(event types.PaymentEvent, transaction *types.Transaction, err error) {
	data := types.EventData{
		Event:       event,
		Reference:   pi.Reference(),
		Transaction: transaction,
		Err:         err,
		At:          time.Now().UTC(),
	}
	if sdkErr, ok := err.(*types.SDKError); ok {
		data.Category = sdkErr.Category
		data.Retryable = sdkErr.Retryable
	}

	// Marking the event fired and collecting its handlers happen together, so
	// a handler subscribing concurrently either lands in this batch or takes
	// the replay path, never both.
	pi.mu.Lock()
	if _, already := pi.fired[event]; already {
		pi.mu.Unlock()
		return
	}
	pi.fired[event] = data
	handlers := pi.events.take(event)
	pi.mu.Unlock()

	// Fired outside the lock: merchant code may block, and a handler is free to
	// call back into the instance.
	for _, handler := range handlers {
		invokeHandler(handler, data)
	}
}

// statusToEvent maps a status onto the lifecycle event a merchant cares about.
// pending, processing and on_hold are one moment to a merchant: accepted and in
// flight.
func statusToEvent(status types.TransactionStatus) (types.PaymentEvent, bool) {
	switch status {
	case types.TransactionStatusPending, types.TransactionStatusProcessing, types.TransactionStatusOnHold:
		return types.PaymentEventProcessing, true
	case types.TransactionStatusSuccessful:
		return types.PaymentEventSuccess, true
	case types.TransactionStatusFailed:
		return types.PaymentEventFailed, true
	case types.TransactionStatusCancelled:
		return types.PaymentEventCancelled, true
	default:
		return "", false
	}
}

// normalizeStatus folds the backend's "completed" onto "successful" so events
// fire correctly regardless of which spelling arrives.
func normalizeStatus(raw types.TransactionStatus) types.TransactionStatus {
	if raw == "completed" {
		return types.TransactionStatusSuccessful
	}
	return raw
}

func failureMessage(transaction *types.Transaction) string {
	if transaction != nil && transaction.FailureReason != nil && *transaction.FailureReason != "" {
		return *transaction.FailureReason
	}
	return "The payment failed"
}

// nextPollDelay spaces out the next status check.
//
// Jitter is added to every interval so that many instances started at once do
// not synchronise into a burst against the status endpoint.
func nextPollDelay(base, elapsed time.Duration) time.Duration {
	interval := resolvePollInterval(base, elapsed)

	// Jitter never exceeds the interval it is perturbing. At the default 2s
	// interval this is simply the full PollJitter; it only bites for intervals
	// shorter than the jitter itself, where a fixed 250ms would swamp the
	// configured pace rather than spread it.
	jitter := PollJitter
	if interval < jitter {
		jitter = interval
	}
	return interval + time.Duration(rand.Int63n(int64(jitter)+1))
}

// resolvePollInterval returns the base interval for the first two minutes, then
// doubles it every two minutes up to the 15s ceiling.
func resolvePollInterval(base, elapsed time.Duration) time.Duration {
	if elapsed < pollBackoffPeriod {
		return base
	}

	periods := int(elapsed / pollBackoffPeriod) // >= 1 here
	// Guard the shift rather than relying on the cap: a long-lived instance
	// would otherwise overflow the duration and produce a negative interval.
	if periods > 20 {
		return maxPollInterval
	}

	interval := base << periods
	if interval <= 0 || interval > maxPollInterval {
		return maxPollInterval
	}
	return interval
}
