package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// CollectPayment initiates a collection and returns a PaymentInstance that
// tracks it to a terminal state.
//
// The returned error covers only client-side validation, the cases a merchant
// can fix in code: a sub-minimum amount, a missing field, an out-of-range
// reference. A server-side rejection (bad key, provider refusal, network
// failure) does not come back here. It arrives as an "error" event on the
// instance, because by then the merchant is holding one and the event channel
// is where every other outcome shows up:
//
//	instance, err := client.CollectPayment(ctx, input)
//	if err != nil {
//	    return err // your input was wrong
//	}
//	instance.On(nylonpay.PaymentEventError, func(e nylonpay.EventData) {
//	    log.Printf("collection failed to start: %v (%s)", e.Err, e.Category)
//	})
//
// ctx governs both the initiation request and the polling that follows. Pass a
// context that outlives the HTTP request you are serving if you want polling to
// continue past it.
func (c *NylonPayClient) CollectPayment(ctx context.Context, input types.CollectPaymentInput) (*core.PaymentInstance, error) {
	prepared, err := c.prepareCollectWithHook(input)
	if err != nil {
		return nil, err
	}

	var initiated struct {
		Reference string                  `json:"reference"`
		Status    types.TransactionStatus `json:"status"`
	}
	sendErr := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionCollectPayment,
		Payload: prepared,
	}, &initiated)

	c.runAfterCollect(hookResult(prepared.Reference, initiated.Reference, initiated.Status, sendErr), prepared, input)

	if sendErr != nil {
		return c.newInstance(ctx, prepared.Reference, "", sendErr), nil
	}
	return c.newInstance(ctx, initiated.Reference, initiated.Status, nil), nil
}

// CollectPaymentAndResolve initiates a collection and blocks until it reaches a
// terminal state.
//
// The server resolves inline for about a minute; if the payment is still
// pending when that budget runs out, the SDK keeps polling client-side until it
// settles, or until a configured poll cap or OnDelayedReturn ends the wait.
//
// The full transaction is returned even when the payment failed, so the failure
// reason and metadata stay available alongside the error.
func (c *NylonPayClient) CollectPaymentAndResolve(ctx context.Context, input types.CollectPaymentInput) (*types.Transaction, error) {
	prepared, err := c.prepareCollectWithHook(input)
	if err != nil {
		return nil, err
	}

	var transaction types.Transaction
	sendErr := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionCollectPaymentAndResolve,
		Payload: prepared,
	}, &transaction)

	c.runAfterCollect(hookResult(prepared.Reference, transaction.Reference, transaction.Status, sendErr), prepared, input)

	if sendErr != nil {
		return nil, sendErr
	}
	return c.continueResolveIfNeeded(ctx, &transaction)
}

// prepareCollectWithHook validates the input, gives the beforeCollect hook its
// chance to change it, then validates again.
//
// The second pass is the point: a hook runs after validation, so without it a
// hook could put an out-of-range reference, a zero amount or an unnormalized
// phone number straight onto the wire. Re-running the same checks means a hook
// can smuggle nothing past them that the original caller could not.
func (c *NylonPayClient) prepareCollectWithHook(input types.CollectPaymentInput) (types.CollectPaymentInput, error) {
	prepared, err := c.prepareCollect(input)
	if err != nil {
		return prepared, err
	}

	mutated := c.runBeforeCollect(prepared)
	// A hook that cleared the reference must not cause a fresh one to be
	// generated: that would change the transaction's identity behind the
	// merchant's back and break idempotency.
	if mutated.Reference == "" {
		mutated.Reference = prepared.Reference
	}
	return c.prepareCollect(mutated)
}
