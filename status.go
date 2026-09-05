package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/internal/utils"
	"github.com/nile-squad/nylonpay-go/types"
)

// GetStatus is a one-shot status check. It does not poll; use CollectPayment or
// MakePayout when you want an instance that follows a transaction to
// completion.
//
// A Delayed flag on the response means the payment has been in flight for more
// than about three minutes. It is a flag, not a status value.
func (c *NylonPayClient) GetStatus(ctx context.Context, input types.GetStatusInput) (*types.StatusResponse, error) {
	if err := requireNonEmpty(input.Reference, "reference"); err != nil {
		return nil, err
	}

	var status types.StatusResponse
	if err := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionGetStatus,
		Payload: input,
	}, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// GetTransaction looks up a full transaction record. At least one of ID or
// Reference is required; supplying both is allowed and the server reconciles
// them.
func (c *NylonPayClient) GetTransaction(ctx context.Context, input types.GetTransactionInput) (*types.Transaction, error) {
	if input.ID == "" && input.Reference == "" {
		return nil, validationErr("id or reference is required")
	}

	var transaction types.Transaction
	if err := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionGetTransaction,
		Payload: input,
	}, &transaction); err != nil {
		return nil, err
	}
	return &transaction, nil
}

// VerifyPhone pre-validates a phone number with the provider and returns the
// name registered on the account.
//
// The number may be in any accepted format; it is normalized to international
// form before the request is signed.
func (c *NylonPayClient) VerifyPhone(ctx context.Context, input types.VerifyPhoneInput) (*types.PhoneVerification, error) {
	if err := requireNonEmpty(input.PhoneNumber, "phoneNumber"); err != nil {
		return nil, err
	}

	input.PhoneNumber = utils.NormalizePhone(input.PhoneNumber)
	if err := validatePhone(input.PhoneNumber, "phoneNumber"); err != nil {
		return nil, err
	}

	var verification types.PhoneVerification
	if err := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionVerifyPhone,
		Payload: input,
	}, &verification); err != nil {
		return nil, err
	}
	return &verification, nil
}
