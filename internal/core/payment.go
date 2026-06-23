package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

type PaymentInstance struct {
	mu               sync.RWMutex
	reference        string
	status           string
	transaction      *types.Transaction
	err              error
	resolved         bool
	done             chan struct{}
	fetchStatus      func(ctx context.Context, ref string) (string, error)
	fetchTransaction func(ctx context.Context, ref string) (*types.Transaction, error)
	pollInterval     time.Duration
	maxPollDuration  time.Duration
	maxPollAttempts  int
}

// NewPaymentInstance starts background polling immediately.
func NewPaymentInstance(cfg PaymentInstanceConfig) *PaymentInstance {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.MaxPollDuration == 0 {
		cfg.MaxPollDuration = DefaultPollDuration
	}
	if cfg.MaxPollAttempts == 0 {
		cfg.MaxPollAttempts = DefaultPollAttempts
	}

	pi := &PaymentInstance{
		reference:        cfg.Reference,
		status:           cfg.InitialStatus,
		done:             make(chan struct{}),
		fetchStatus:      cfg.FetchStatus,
		fetchTransaction: cfg.FetchTransaction,
		pollInterval:     cfg.PollInterval,
		maxPollDuration:  cfg.MaxPollDuration,
		maxPollAttempts:  cfg.MaxPollAttempts,
	}

	go pi.startPolling()

	return pi
}

func (pi *PaymentInstance) Reference() string {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.reference
}

func (pi *PaymentInstance) Status() string {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.status
}

func (pi *PaymentInstance) Transaction() *types.Transaction {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.transaction
}

// Wait blocks until terminal or ctx is done.
func (pi *PaymentInstance) Wait(ctx context.Context) (*types.Transaction, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-pi.done:
		pi.mu.RLock()
		defer pi.mu.RUnlock()
		return pi.transaction, pi.err
	}
}

func (pi *PaymentInstance) startPolling() {
	if terminalStates[pi.status] {
		pi.resolveTerminalState(context.Background(), pi.status)
		return
	}

	startTime := time.Now()
	attempts := 0

	for {
		if attempts >= pi.maxPollAttempts {
			pi.resolveWithError(errors.New("polling timeout: exceeded maximum attempts"))
			return
		}
		if time.Since(startTime) >= pi.maxPollDuration {
			pi.resolveWithError(errors.New("polling timeout: exceeded maximum duration"))
			return
		}

		jitter := time.Duration(rand.Float64() * float64(PollJitter))
		time.Sleep(pi.pollInterval + jitter)
		attempts++

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		rawStatus, err := pi.fetchStatus(ctx, pi.reference)
		cancel()

		if err != nil {
			var sdkErr *SDKError
			if errors.As(err, &sdkErr) && sdkErr.Category == "not_found" {
				continue
			}
			pi.resolveWithError(fmt.Errorf("polling status update failed: %w", err))
			return
		}

		newStatus := normalizeStatus(rawStatus)
		pi.mu.Lock()
		pi.status = newStatus
		isTerminal := terminalStates[newStatus]
		pi.mu.Unlock()

		if isTerminal {
			pi.resolveTerminalState(context.Background(), newStatus)
			return
		}
	}
}

func (pi *PaymentInstance) resolveTerminalState(ctx context.Context, status string) {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	if pi.resolved {
		return
	}

	tx, err := pi.fetchTransaction(ctx, pi.reference)
	if err != nil {
		pi.err = fmt.Errorf("failed to fetch final transaction record: %w", err)
	} else {
		pi.transaction = tx
		if status == "failed" && tx.FailureReason != nil {
			pi.err = &SDKError{Category: "provider", Message: *tx.FailureReason}
		}
	}

	pi.resolved = true
	close(pi.done)
}

func (pi *PaymentInstance) resolveWithError(err error) {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	if pi.resolved {
		return
	}

	pi.err = err
	pi.resolved = true
	close(pi.done)
}

func normalizeStatus(raw string) string {
	if raw == "completed" {
		return "successful"
	}
	return raw
}
