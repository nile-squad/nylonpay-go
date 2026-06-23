package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

const fastPoll = 1 * time.Millisecond

func mockFetchStatus(sequence []string) func(context.Context, string) (string, error) {
	i := 0
	return func(_ context.Context, _ string) (string, error) {
		if i >= len(sequence) {
			return "successful", nil
		}
		s := sequence[i]
		i++
		return s, nil
	}
}

func mockFetchTransaction(tx *types.Transaction) func(context.Context, string) (*types.Transaction, error) {
	return func(_ context.Context, _ string) (*types.Transaction, error) {
		return tx, nil
	}
}

func fakeTx(ref string, status types.TransactionStatus) *types.Transaction {
	return &types.Transaction{
		ID:        "txn_" + ref,
		Reference: ref,
		Status:    status,
		Amount:    1000,
		Currency:  types.UGX,
	}
}

func newTestInstance(cfg PaymentInstanceConfig) *PaymentInstance {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = fastPoll
	}
	return NewPaymentInstance(cfg)
}

// ── Wait resolves correctly ───────────────────────────────────────────────────

func TestPaymentInstance_ResolvesOnSuccess(t *testing.T) {
	ref := "ref_success"
	tx := fakeTx(ref, types.TransactionStatusSuccessful)

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     ref,
		InitialStatus: "pending",
		FetchStatus:   mockFetchStatus([]string{"pending", "successful"}),
		FetchTransaction: mockFetchTransaction(tx),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := pi.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected transaction, got nil")
	}
	if got.Reference != ref {
		t.Errorf("reference = %q, want %q", got.Reference, ref)
	}
}

func TestPaymentInstance_ResolvesOnFailure(t *testing.T) {
	reason := "insufficient funds"
	tx := &types.Transaction{
		Reference:     "ref_fail",
		Status:        types.TransactionStatusFailed,
		FailureReason: &reason,
	}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        "ref_fail",
		InitialStatus:    "pending",
		FetchStatus:      mockFetchStatus([]string{"failed"}),
		FetchTransaction: mockFetchTransaction(tx),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pi.Wait(ctx)
	if err == nil {
		t.Fatal("expected error for failed payment, got nil")
	}
	var sdkErr *SDKError
	if !errors.As(err, &sdkErr) {
		t.Fatalf("expected *SDKError, got %T: %v", err, err)
	}
	if sdkErr.Category != "provider" {
		t.Errorf("category = %q, want provider", sdkErr.Category)
	}
	if sdkErr.Message != reason {
		t.Errorf("message = %q, want %q", sdkErr.Message, reason)
	}
}

func TestPaymentInstance_ResolvesOnCancelled(t *testing.T) {
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        "ref_cancel",
		InitialStatus:    "pending",
		FetchStatus:      mockFetchStatus([]string{"cancelled"}),
		FetchTransaction: mockFetchTransaction(fakeTx("ref_cancel", types.TransactionStatusCancelled)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := pi.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error for cancelled payment: %v", err)
	}
	if tx == nil {
		t.Fatal("expected transaction for cancelled status, got nil")
	}
}

func TestPaymentInstance_SkipsNotFoundErrors(t *testing.T) {
	// not_found during early polling is transient — must be skipped, not fatal.
	calls := 0
	fetchStatus := func(_ context.Context, _ string) (string, error) {
		calls++
		if calls < 3 {
			return "", &SDKError{Category: "not_found", Message: "tx not indexed yet"}
		}
		return "successful", nil
	}

	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        "ref_nf",
		InitialStatus:    "pending",
		FetchStatus:      fetchStatus,
		FetchTransaction: mockFetchTransaction(fakeTx("ref_nf", types.TransactionStatusSuccessful)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pi.Wait(ctx)
	if err != nil {
		t.Fatalf("not_found errors should be skipped, got: %v", err)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 fetchStatus calls, got %d", calls)
	}
}

func TestPaymentInstance_TerminatesOnMaxAttempts(t *testing.T) {
	// Always return pending — polling must stop at MaxPollAttempts.
	pi := NewPaymentInstance(PaymentInstanceConfig{
		Reference:        "ref_timeout",
		InitialStatus:    "pending",
		PollInterval:     fastPoll,
		MaxPollAttempts:  3,
		MaxPollDuration:  10 * time.Second,
		FetchStatus:      mockFetchStatus(nil), // always returns "successful" but MaxPollAttempts wins
		FetchTransaction: mockFetchTransaction(fakeTx("ref_timeout", types.TransactionStatusSuccessful)),
	})

	// Override fetchStatus to always return pending.
	pi.fetchStatus = func(_ context.Context, _ string) (string, error) {
		return "pending", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pi.Wait(ctx)
	if err == nil {
		t.Fatal("expected timeout error after max attempts, got nil")
	}
}

func TestPaymentInstance_WaitRespectsContextCancellation(t *testing.T) {
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     "ref_ctx",
		InitialStatus: "pending",
		FetchStatus: func(_ context.Context, _ string) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "pending", nil
		},
		FetchTransaction: mockFetchTransaction(nil),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := pi.Wait(ctx)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestPaymentInstance_StartsTerminalStatus_ResolvesImmediately(t *testing.T) {
	// If InitialStatus is already terminal, polling should not even start.
	fetchCalled := false
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:     "ref_imm",
		InitialStatus: "successful",
		FetchStatus: func(_ context.Context, _ string) (string, error) {
			fetchCalled = true
			return "successful", nil
		},
		FetchTransaction: mockFetchTransaction(fakeTx("ref_imm", types.TransactionStatusSuccessful)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := pi.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx == nil {
		t.Fatal("expected transaction, got nil")
	}
	if fetchCalled {
		t.Error("fetchStatus must not be called when initial status is already terminal")
	}
}

func TestPaymentInstance_StatusAccessible(t *testing.T) {
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        "ref_status",
		InitialStatus:    "pending",
		FetchStatus:      mockFetchStatus([]string{"successful"}),
		FetchTransaction: mockFetchTransaction(fakeTx("ref_status", types.TransactionStatusSuccessful)),
	})

	// Status before resolution.
	if pi.Status() == "" {
		t.Error("status must not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pi.Wait(ctx)

	if pi.Status() != "successful" {
		t.Errorf("status after resolution = %q, want successful", pi.Status())
	}
}

func TestPaymentInstance_ReferenceAccessible(t *testing.T) {
	pi := newTestInstance(PaymentInstanceConfig{
		Reference:        "my_ref_123456",
		InitialStatus:    "pending",
		FetchStatus:      mockFetchStatus([]string{"successful"}),
		FetchTransaction: mockFetchTransaction(fakeTx("my_ref_123456", types.TransactionStatusSuccessful)),
	})
	if pi.Reference() != "my_ref_123456" {
		t.Errorf("Reference() = %q, want my_ref_123456", pi.Reference())
	}
}
