package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// newInstance builds a PaymentInstance wired to this client's status and
// transaction lookups. A non-nil initialError means initiation failed, so the
// instance reports it rather than polling for a transaction that never started.
func (c *NylonPayClient) newInstance(
	ctx context.Context,
	reference string,
	status types.TransactionStatus,
	initialError error,
) *core.PaymentInstance {
	return core.NewPaymentInstance(core.PaymentInstanceConfig{
		Reference:        reference,
		InitialStatus:    status,
		FetchStatus:      c.fetchStatus,
		FetchTransaction: c.fetchTransaction,
		PollInterval:     c.cfg.MaxPollInterval,
		MaxPollDuration:  c.cfg.MaxPollDuration,
		MaxPollAttempts:  c.cfg.MaxPollAttempts,
		OnDelayed:        c.cfg.OnDelayed,
		InitialError:     initialError,
		Context:          ctx,
	})
}

func (c *NylonPayClient) fetchStatus(ctx context.Context, reference string) (*types.StatusResponse, error) {
	return c.GetStatus(ctx, types.GetStatusInput{Reference: reference})
}

func (c *NylonPayClient) fetchTransaction(ctx context.Context, reference string) (*types.Transaction, error) {
	return c.GetTransaction(ctx, types.GetTransactionInput{Reference: reference})
}

// continueResolveIfNeeded finishes a blocking resolve.
//
// The server resolves inline for roughly a minute. When that budget ends with
// the payment still in flight it answers with a non-terminal transaction, and
// the wait continues here rather than handing the merchant a pending record
// they did not ask for.
func (c *NylonPayClient) continueResolveIfNeeded(ctx context.Context, transaction *types.Transaction) (*types.Transaction, error) {
	if transaction.Status.IsTerminal() {
		return resolveOutcome(transaction)
	}

	instance := c.newInstance(ctx, transaction.Reference, transaction.Status, nil)
	defer instance.Close()

	resolved, err := instance.Wait(ctx)
	if resolved == nil {
		// A failed or cancelled payment still has a full record. Wait reports
		// the failure; the record stays reachable so the caller can read the
		// failure reason and metadata off it.
		resolved = instance.Transaction()
	}
	if resolved == nil {
		return nil, err
	}
	return resolved, err
}

// resolveOutcome pairs a terminal transaction with the error, if any, that its
// status represents. The transaction is returned either way.
func resolveOutcome(transaction *types.Transaction) (*types.Transaction, error) {
	switch transaction.Status {
	case types.TransactionStatusFailed:
		message := "The payment failed"
		if transaction.FailureReason != nil && *transaction.FailureReason != "" {
			message = *transaction.FailureReason
		}
		return transaction, &types.SDKError{Category: types.CategoryProvider, Message: message}
	case types.TransactionStatusCancelled:
		return transaction, &types.SDKError{Category: types.CategoryProvider, Message: "The payment was cancelled"}
	default:
		return transaction, nil
	}
}

// hookResult assembles what an after* hook is told about the call. The
// reference falls back to the one that was sent, since a failed initiation
// returns no body to read it from.
func hookResult(sentReference, returnedReference string, status types.TransactionStatus, err error) HookResult {
	reference := returnedReference
	if reference == "" {
		reference = sentReference
	}
	return HookResult{Reference: reference, Status: string(status), Err: err}
}
