package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// MakePayout initiates a disbursement and returns a PaymentInstance that tracks
// it to a terminal state.
//
// Payouts use the same async primitive as collections, so the handling is
// identical. One difference in practice: a payout may sit in "on_hold" while it
// is reviewed, which is reported as a "processing" event and keeps polling.
//
// As with CollectPayment, the returned error covers only client-side
// validation; a server-side rejection arrives as an "error" event on the
// instance.
func (c *NylonPayClient) MakePayout(ctx context.Context, input types.MakePayoutInput) (*core.PaymentInstance, error) {
	prepared, err := c.preparePayoutWithHook(input)
	if err != nil {
		return nil, err
	}

	var initiated struct {
		Reference string                  `json:"reference"`
		Status    types.TransactionStatus `json:"status"`
	}
	sendErr := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionMakePayout,
		Payload: prepared,
	}, &initiated)

	c.runAfterPayout(hookResult(prepared.Reference, initiated.Reference, initiated.Status, sendErr), prepared, input)

	if sendErr != nil {
		return c.newInstance(ctx, prepared.Reference, "", sendErr), nil
	}
	return c.newInstance(ctx, initiated.Reference, initiated.Status, nil), nil
}

// MakePayoutAndResolve initiates a disbursement and blocks until it reaches a
// terminal state.
//
// Webhooks remain the authoritative record for payout completion: providers can
// settle well after the SDK stops waiting.
func (c *NylonPayClient) MakePayoutAndResolve(ctx context.Context, input types.MakePayoutInput) (*types.Transaction, error) {
	prepared, err := c.preparePayoutWithHook(input)
	if err != nil {
		return nil, err
	}

	var transaction types.Transaction
	sendErr := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionMakePayoutAndResolve,
		Payload: prepared,
	}, &transaction)

	c.runAfterPayout(hookResult(prepared.Reference, transaction.Reference, transaction.Status, sendErr), prepared, input)

	if sendErr != nil {
		return nil, sendErr
	}
	return c.continueResolveIfNeeded(ctx, &transaction)
}

// preparePayoutWithHook validates, applies the beforePayout hook, then
// validates again. See prepareCollectWithHook for why the second pass matters.
func (c *NylonPayClient) preparePayoutWithHook(input types.MakePayoutInput) (types.MakePayoutInput, error) {
	prepared, err := c.preparePayout(input)
	if err != nil {
		return prepared, err
	}

	mutated := c.runBeforePayout(prepared)
	if mutated.Reference == "" {
		mutated.Reference = prepared.Reference
	}
	return c.preparePayout(mutated)
}
