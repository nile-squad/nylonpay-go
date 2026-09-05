package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

const fastPoll = 1 * time.Millisecond

// statusSequence serves each status once, then repeats the last one so a test
// that polls more times than it scripted does not fall off the end.
func statusSequence(reference string, statuses ...types.TransactionStatus) func(context.Context, string) (*types.StatusResponse, error) {
	var mu sync.Mutex
	index := 0
	return func(_ context.Context, _ string) (*types.StatusResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		status := statuses[len(statuses)-1]
		if index < len(statuses) {
			status = statuses[index]
			index++
		}
		return &types.StatusResponse{Reference: reference, Status: status}, nil
	}
}

func delayedStatus(reference string, status types.TransactionStatus) func(context.Context, string) (*types.StatusResponse, error) {
	delayed := true
	return func(_ context.Context, _ string) (*types.StatusResponse, error) {
		return &types.StatusResponse{Reference: reference, Status: status, Delayed: &delayed}, nil
	}
}

func failingStatus(err error) func(context.Context, string) (*types.StatusResponse, error) {
	return func(_ context.Context, _ string) (*types.StatusResponse, error) {
		return nil, err
	}
}

func mockFetchTransaction(tx *types.Transaction) func(context.Context, string) (*types.Transaction, error) {
	return func(_ context.Context, _ string) (*types.Transaction, error) {
		return tx, nil
	}
}

func fakeTx(reference string, status types.TransactionStatus) *types.Transaction {
	return &types.Transaction{
		ID:        "txn_" + reference,
		Reference: reference,
		Status:    status,
		Amount:    1000,
		Currency:  types.UGX,
	}
}

func newTestInstance(cfg PaymentInstanceConfig) *PaymentInstance {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = fastPoll
	}
	if cfg.FetchTransaction == nil {
		cfg.FetchTransaction = mockFetchTransaction(fakeTx(cfg.Reference, types.TransactionStatusSuccessful))
	}
	return NewPaymentInstance(cfg)
}

// recorder collects the events an instance emits, in order.
type recorder struct {
	mu     sync.Mutex
	events []types.EventData
}

func (r *recorder) handler() types.PaymentEventHandler {
	return func(data types.EventData) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.events = append(r.events, data)
	}
}

func (r *recorder) names() []types.PaymentEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]types.PaymentEvent, len(r.events))
	for i, event := range r.events {
		names[i] = event.Event
	}
	return names
}

// subscribeAll registers one recorder across every lifecycle event.
func subscribeAll(pi *PaymentInstance, r *recorder) {
	for _, event := range []types.PaymentEvent{
		types.PaymentEventProcessing,
		types.PaymentEventSuccess,
		types.PaymentEventFailed,
		types.PaymentEventCancelled,
		types.PaymentEventError,
	} {
		pi.On(event, r.handler())
	}
}

func waitFor(t *testing.T, pi *PaymentInstance) (*types.Transaction, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return pi.Wait(ctx)
}

func assertEvents(t *testing.T, got []types.PaymentEvent, want ...types.PaymentEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}

// ── Status transitions ────────────────────────────────────────────────────────

func TestPaymentInstance_PendingToProcessingToSuccessful(t *testing.T) {
	const ref = "ref_success_01"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: statusSequence(ref,
			types.TransactionStatusProcessing,
			types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	subscribeAll(pi, rec)

	tx, err := waitFor(t, pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx == nil || tx.Reference != ref {
		t.Fatalf("transaction = %+v, want reference %s", tx, ref)
	}

	// "processing" covers pending and processing alike, so it fires once.
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventSuccess)
}

func TestPaymentInstance_ProcessingFiresOnceAcrossStatusFlap(t *testing.T) {
	const ref = "ref_flapping_1"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: statusSequence(ref,
			types.TransactionStatusProcessing,
			types.TransactionStatusPending,
			types.TransactionStatusOnHold,
			types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	subscribeAll(pi, rec)

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventSuccess)
}

// A payment that resolves between polls must still report that it was in
// flight before it reports the outcome.
func TestPaymentInstance_ProcessingPrecedesTerminalEvenWhenFast(t *testing.T) {
	const ref = "ref_fastpath_1"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	subscribeAll(pi, rec)

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventSuccess)
}

func TestPaymentInstance_FailedCarriesReason(t *testing.T) {
	const ref = "ref_failure_01"
	reason := "Insufficient funds"
	tx := fakeTx(ref, types.TransactionStatusFailed)
	tx.FailureReason = &reason
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusFailed),
		FetchTransaction: mockFetchTransaction(tx),
	})
	subscribeAll(pi, rec)

	got, err := waitFor(t, pi)
	if got != nil {
		t.Errorf("Wait must not return a transaction for a failed payment, got %+v", got)
	}
	var sdkErr *types.SDKError
	if !asSDKError(err, &sdkErr) {
		t.Fatalf("error = %v, want *types.SDKError", err)
	}
	if sdkErr.Category != types.CategoryProvider || sdkErr.Message != reason {
		t.Errorf("error = %+v, want provider/%q", sdkErr, reason)
	}
	// The record stays reachable so the reason and metadata are not lost.
	if pi.Transaction() == nil || pi.Transaction().FailureReason == nil {
		t.Error("failed transaction record must remain available via Transaction()")
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventFailed)
}

func TestPaymentInstance_Cancelled(t *testing.T) {
	const ref = "ref_cancelled1"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusCancelled),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusCancelled)),
	})
	subscribeAll(pi, rec)

	if _, err := waitFor(t, pi); err == nil {
		t.Error("expected an error for a cancelled payment")
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventCancelled)
}

// on_hold is a payout under review: it reports as "processing" and keeps
// polling rather than resolving.
func TestPaymentInstance_OnHoldIsNotTerminal(t *testing.T) {
	const ref = "ref_onhold_001"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusOnHold,
		FetchStatus: statusSequence(ref,
			types.TransactionStatusOnHold,
			types.TransactionStatusOnHold,
			types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	subscribeAll(pi, rec)

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventSuccess)
}

func TestPaymentInstance_TerminalInitialStatusResolvesWithoutPolling(t *testing.T) {
	const ref = "ref_immediate1"
	polled := false

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusSuccessful,
		FetchStatus: func(context.Context, string) (*types.StatusResponse, error) {
			polled = true
			return &types.StatusResponse{Reference: ref, Status: types.TransactionStatusSuccessful}, nil
		},
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if polled {
		t.Error("an already-terminal initiation response must not be polled")
	}
}

// ── Failure modes that stop polling ───────────────────────────────────────────

func TestPaymentInstance_ReferenceMismatchStops(t *testing.T) {
	const ref = "ref_mismatch_1"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: func(context.Context, string) (*types.StatusResponse, error) {
			return &types.StatusResponse{Reference: "someone_elses", Status: types.TransactionStatusSuccessful}, nil
		},
	})
	subscribeAll(pi, rec)

	if _, err := waitFor(t, pi); err == nil {
		t.Fatal("expected an error when a status update names a different transaction")
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventError)
}

// A not_found early on means the transaction has not propagated to the read
// path yet, which is expected rather than fatal.
func TestPaymentInstance_ToleratesNotFoundThenSucceeds(t *testing.T) {
	const ref = "ref_notfound_1"
	var mu sync.Mutex
	calls := 0

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: func(context.Context, string) (*types.StatusResponse, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls <= 2 {
				return nil, &types.SDKError{Category: types.CategoryNotFound, Message: "not found"}
			}
			return &types.StatusResponse{Reference: ref, Status: types.TransactionStatusSuccessful}, nil
		},
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("not_found should not end polling, got: %v", err)
	}
}

func TestPaymentInstance_PollErrorStops(t *testing.T) {
	const ref = "ref_pollerror1"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: failingStatus(&types.SDKError{
			Category: types.CategoryNetwork,
			Message:  "Could not reach the server",
		}),
	})
	subscribeAll(pi, rec)

	_, err := waitFor(t, pi)
	var sdkErr *types.SDKError
	if !asSDKError(err, &sdkErr) || sdkErr.Category != types.CategoryNetwork {
		t.Fatalf("error = %v, want a network SDKError", err)
	}
	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventError)
}

// ── Merchant-configured caps ──────────────────────────────────────────────────

func TestPaymentInstance_MaxPollAttemptsCap(t *testing.T) {
	const ref = "ref_attempts_1"

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:       ref,
		InitialStatus:   types.TransactionStatusPending,
		FetchStatus:     statusSequence(ref, types.TransactionStatusPending),
		MaxPollAttempts: 3,
	})

	_, err := waitFor(t, pi)
	var sdkErr *types.SDKError
	if !asSDKError(err, &sdkErr) || sdkErr.Category != types.CategoryTimeout {
		t.Fatalf("error = %v, want a timeout SDKError", err)
	}
	if sdkErr.Message != msgPollTimeout {
		t.Errorf("message = %q, want %q", sdkErr.Message, msgPollTimeout)
	}
}

func TestPaymentInstance_MaxPollDurationCap(t *testing.T) {
	const ref = "ref_duration_1"

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:       ref,
		InitialStatus:   types.TransactionStatusPending,
		FetchStatus:     statusSequence(ref, types.TransactionStatusPending),
		MaxPollDuration: 20 * time.Millisecond,
	})

	_, err := waitFor(t, pi)
	var sdkErr *types.SDKError
	if !asSDKError(err, &sdkErr) || sdkErr.Category != types.CategoryTimeout {
		t.Fatalf("error = %v, want a timeout SDKError", err)
	}
}

// With no caps configured the instance polls until the transaction settles,
// however many polls that takes.
func TestPaymentInstance_NoCapsPollsUntilTerminal(t *testing.T) {
	const ref = "ref_uncapped_1"
	pending := make([]types.TransactionStatus, 0, 26)
	for i := 0; i < 25; i++ {
		pending = append(pending, types.TransactionStatusPending)
	}
	pending = append(pending, types.TransactionStatusSuccessful)

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, pending...),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("uncapped polling should reach terminal, got: %v", err)
	}
}

// ── Delayed payments ──────────────────────────────────────────────────────────

func TestPaymentInstance_DelayedWithReturnResolvesPending(t *testing.T) {
	const ref = "ref_delayed_r1"
	pending := fakeTx(ref, types.TransactionStatusPending)

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      delayedStatus(ref, types.TransactionStatusPending),
		FetchTransaction: mockFetchTransaction(pending),
		OnDelayed:        types.OnDelayedReturn,
	})

	tx, err := waitFor(t, pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx == nil || tx.Status != types.TransactionStatusPending {
		t.Fatalf("transaction = %+v, want a still-pending record", tx)
	}
}

func TestPaymentInstance_DelayedWithWaitKeepsPolling(t *testing.T) {
	const ref = "ref_delayed_w1"
	var mu sync.Mutex
	calls := 0
	delayed := true

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: func(context.Context, string) (*types.StatusResponse, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			status := types.TransactionStatusPending
			if calls > 3 {
				status = types.TransactionStatusSuccessful
			}
			return &types.StatusResponse{Reference: ref, Status: status, Delayed: &delayed}, nil
		},
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
		OnDelayed:        types.OnDelayedWait,
	})

	tx, err := waitFor(t, pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.Status != types.TransactionStatusSuccessful {
		t.Errorf("status = %s, want successful", tx.Status)
	}
}

// ── Initiation failure ────────────────────────────────────────────────────────

// A backend rejection means no transaction exists to poll, so it surfaces as an
// "error" event rather than a returned error.
func TestPaymentInstance_InitialErrorEmitsErrorEvent(t *testing.T) {
	const ref = "ref_initerror1"
	rec := &recorder{}
	initErr := &types.SDKError{Category: types.CategoryAuth, Message: "API key was not found", Retryable: false}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:    ref,
		InitialError: initErr,
		FetchStatus: func(context.Context, string) (*types.StatusResponse, error) {
			t.Error("an instance carrying an initiation error must never poll")
			return nil, nil
		},
	})
	subscribeAll(pi, rec)

	tx, err := waitFor(t, pi)
	if tx != nil {
		t.Errorf("transaction = %+v, want nil", tx)
	}
	if err != initErr {
		t.Errorf("error = %v, want the initiation error", err)
	}
	assertEvents(t, rec.names(), types.PaymentEventError)

	events := rec.events
	if events[0].Category != types.CategoryAuth {
		t.Errorf("category = %s, want auth", events[0].Category)
	}
	if events[0].Reference != ref {
		t.Errorf("reference = %q, want %q", events[0].Reference, ref)
	}
}

// ── Subscription semantics ────────────────────────────────────────────────────

// Go has no event loop, so a handler registered after the operation returned
// could otherwise miss an event that already fired.
func TestPaymentInstance_LateSubscriberStillReceivesEvents(t *testing.T) {
	const ref = "ref_late_sub_1"

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Subscribing only now, well after both events fired.
	rec := &recorder{}
	subscribeAll(pi, rec)

	assertEvents(t, rec.names(), types.PaymentEventProcessing, types.PaymentEventSuccess)
}

func TestPaymentInstance_OffRemovesHandler(t *testing.T) {
	const ref = "ref_off_handler"
	rec := &recorder{}
	handler := rec.handler()

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	pi.On(types.PaymentEventSuccess, handler)
	pi.Off(types.PaymentEventSuccess, handler)

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := rec.names(); len(names) != 0 {
		t.Errorf("removed handler still fired: %v", names)
	}
}

func TestPaymentInstance_OffUnregisteredHandlerIsNoOp(t *testing.T) {
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     "ref_off_noop_1",
		InitialStatus: types.TransactionStatusSuccessful,
	})
	defer pi.Close()

	// Must not panic.
	pi.Off(types.PaymentEventSuccess, func(types.EventData) {})
}

func TestPaymentInstance_OnceFiresAtMostOnce(t *testing.T) {
	const ref = "ref_once_fires"
	var mu sync.Mutex
	count := 0

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: statusSequence(ref,
			types.TransactionStatusProcessing,
			types.TransactionStatusPending,
			types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	pi.Once(types.PaymentEventProcessing, func(types.EventData) {
		mu.Lock()
		defer mu.Unlock()
		count++
	})

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("once handler fired %d times, want 1", count)
	}
}

func TestPaymentInstance_OnceCancelledBeforeEventNeverFires(t *testing.T) {
	const ref = "ref_once_off_1"
	rec := &recorder{}
	handler := rec.handler()

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	pi.Once(types.PaymentEventSuccess, handler)
	pi.Off(types.PaymentEventSuccess, handler)

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := rec.names(); len(names) != 0 {
		t.Errorf("cancelled once handler still fired: %v", names)
	}
}

// A merchant's logging callback must never be able to abort a payment.
func TestPaymentInstance_HandlerPanicIsContained(t *testing.T) {
	const ref = "ref_panicky_h1"
	rec := &recorder{}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})
	pi.On(types.PaymentEventProcessing, func(types.EventData) { panic("merchant bug") })
	pi.On(types.PaymentEventSuccess, rec.handler())

	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("a panicking handler must not fail the payment, got: %v", err)
	}
	assertEvents(t, rec.names(), types.PaymentEventSuccess)
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

func TestPaymentInstance_WaitAfterTerminalReturnsSameResult(t *testing.T) {
	const ref = "ref_wait_twice"

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})

	first, err := waitFor(t, pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := waitFor(t, pi)
	if err != nil {
		t.Fatalf("second Wait errored: %v", err)
	}
	if first != second {
		t.Error("Wait after a terminal state must return the same transaction")
	}
}

func TestPaymentInstance_WaitRespectsContextCancellation(t *testing.T) {
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     "ref_ctx_cancel",
		InitialStatus: types.TransactionStatusPending,
		FetchStatus:   statusSequence("ref_ctx_cancel", types.TransactionStatusPending),
		PollInterval:  time.Hour,
	})
	defer pi.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := pi.Wait(ctx); err == nil {
		t.Error("Wait must return once its context is cancelled")
	}
}

func TestPaymentInstance_CloseStopsPolling(t *testing.T) {
	const ref = "ref_close_stop"
	var mu sync.Mutex
	calls := 0

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: types.TransactionStatusPending,
		FetchStatus: func(context.Context, string) (*types.StatusResponse, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return &types.StatusResponse{Reference: ref, Status: types.TransactionStatusPending}, nil
		},
		PollInterval: 2 * time.Millisecond,
	})

	time.Sleep(30 * time.Millisecond)
	pi.Close()

	mu.Lock()
	atClose := calls
	mu.Unlock()

	time.Sleep(40 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// One poll may already have been in flight when Close landed.
	if calls > atClose+1 {
		t.Errorf("polling continued after Close: %d calls at close, %d after", atClose, calls)
	}
}

func TestPaymentInstance_AccessorsReflectState(t *testing.T) {
	const ref = "ref_accessors1"

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        ref,
		InitialStatus:    types.TransactionStatusPending,
		FetchStatus:      statusSequence(ref, types.TransactionStatusSuccessful),
		FetchTransaction: mockFetchTransaction(fakeTx(ref, types.TransactionStatusSuccessful)),
	})

	if pi.Reference() != ref {
		t.Errorf("Reference() = %q, want %q", pi.Reference(), ref)
	}
	if _, err := waitFor(t, pi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pi.Status() != types.TransactionStatusSuccessful {
		t.Errorf("Status() = %s, want successful", pi.Status())
	}
}

// ── Poll pacing ───────────────────────────────────────────────────────────────

func TestResolvePollInterval_BackoffSchedule(t *testing.T) {
	base := 2 * time.Second
	cases := []struct {
		elapsed time.Duration
		want    time.Duration
	}{
		{0, 2 * time.Second},
		{119 * time.Second, 2 * time.Second},
		{2 * time.Minute, 4 * time.Second},
		{4 * time.Minute, 8 * time.Second},
		{6 * time.Minute, 15 * time.Second}, // 16s, capped
		{30 * time.Minute, 15 * time.Second},
		{100 * time.Hour, 15 * time.Second}, // no overflow into a negative
	}
	for _, tc := range cases {
		if got := resolvePollInterval(base, tc.elapsed); got != tc.want {
			t.Errorf("resolvePollInterval(2s, %v) = %v, want %v", tc.elapsed, got, tc.want)
		}
	}
}

func asSDKError(err error, target **types.SDKError) bool {
	sdkErr, ok := err.(*types.SDKError)
	if ok {
		*target = sdkErr
	}
	return ok
}
