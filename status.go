package nylonpay

import (
	"context"
	"strings"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/internal/utils"
	"github.com/nile-squad/nylonpay-go/types"
)

// GetStatus returns the current status of a transaction by reference.
func (c *NylonPayClient) GetStatus(ctx context.Context, reference string) (*types.StatusResponse, error) {
	if strings.TrimSpace(reference) == "" {
		return nil, &core.SDKError{Category: "validation", Message: "reference is required"}
	}

	var resp types.StatusResponse
	payload := map[string]string{"reference": reference}
	if err := c.transport.Send(ctx, core.TransportRequest{Action: "sdk-get-status", Payload: payload}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTransaction retrieves a full transaction record by ID or reference.
func (c *NylonPayClient) GetTransaction(ctx context.Context, input types.GetTransactionInput) (*types.Transaction, error) {
	if input.ID == "" && input.Reference == "" {
		return nil, &core.SDKError{Category: "validation", Message: "id or reference is required"}
	}

	var tx types.Transaction
	if err := c.transport.Send(ctx, core.TransportRequest{Action: "sdk-get-transaction", Payload: input}, &tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

// VerifyPhone checks whether a phone number is registered and returns the account holder name.
func (c *NylonPayClient) VerifyPhone(ctx context.Context, phoneNumber string) (*types.PhoneVerification, error) {
	if strings.TrimSpace(phoneNumber) == "" {
		return nil, &core.SDKError{Category: "validation", Message: "phoneNumber is required"}
	}

	var resp types.PhoneVerification
	payload := map[string]string{"phoneNumber": utils.NormalizePhone(phoneNumber)}
	if err := c.transport.Send(ctx, core.TransportRequest{Action: "sdk-verify-phone", Payload: payload}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
