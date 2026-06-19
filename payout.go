package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/internal/utils"
	"github.com/nile-squad/nylonpay-go/types"
)

// MakePayout initiates an asynchronous outbound disbursement and returns a
// PaymentInstance for tracking the transfer status.
//
// An error is returned (and no instance created) if the initiation request
// itself fails.
func (c *NylonPayClient) MakePayout(ctx context.Context, input types.MakePayoutPayload) (*core.PaymentInstance, error) {
	ref, err := c.resolveReference(input.Reference)
	if err != nil {
		return nil, err
	}
	input.Reference = ref

	if err := c.validatePayout(input); err != nil {
		return nil, err
	}
	input.Customer.PhoneNumber = utils.NormalizePhone(input.Customer.PhoneNumber)

	payload := &input
	if c.cfg.Hooks != nil && c.cfg.Hooks.BeforePayout != nil {
		payload = c.runBeforePayoutHook(c.cfg.Hooks.BeforePayout, payload)
	}

	var initResp struct {
		Reference string `json:"reference"`
		Status    string `json:"status"`
	}
	if err := c.transport.Send(ctx, core.TransportRequest{Action: "make_payout", Payload: payload}, &initResp); err != nil {
		if c.cfg.Hooks != nil && c.cfg.Hooks.AfterPayout != nil {
			c.runAfterPayoutHook(c.cfg.Hooks.AfterPayout, payload, "", "", err)
		}
		return nil, err
	}

	if c.cfg.Hooks != nil && c.cfg.Hooks.AfterPayout != nil {
		c.runAfterPayoutHook(c.cfg.Hooks.AfterPayout, payload, initResp.Reference, initResp.Status, nil)
	}

	return core.NewPaymentInstance(core.PaymentInstanceConfig{
		Reference:        input.Reference,
		InitialStatus:    initResp.Status,
		FetchStatus:      c.rawFetchStatus,
		FetchTransaction: c.rawFetchTransaction,
		PollInterval:     c.cfg.MaxPollInterval,
		MaxPollDuration:  c.cfg.MaxPollDuration,
		MaxPollAttempts:  c.cfg.MaxPollAttempts,
	}), nil
}

// MakePayoutAndResolve initiates a disbursement and blocks until it reaches a
// terminal state, then returns the completed transaction.
func (c *NylonPayClient) MakePayoutAndResolve(ctx context.Context, input types.MakePayoutPayload) (*types.Transaction, error) {
	ref, err := c.resolveReference(input.Reference)
	if err != nil {
		return nil, err
	}
	input.Reference = ref

	if err := c.validatePayout(input); err != nil {
		return nil, err
	}
	input.Customer.PhoneNumber = utils.NormalizePhone(input.Customer.PhoneNumber)

	payload := &input
	if c.cfg.Hooks != nil && c.cfg.Hooks.BeforePayout != nil {
		payload = c.runBeforePayoutHook(c.cfg.Hooks.BeforePayout, payload)
	}

	var tx types.Transaction
	err = c.transport.Send(ctx, core.TransportRequest{Action: "make_payout_and_resolve", Payload: payload}, &tx)

	if c.cfg.Hooks != nil && c.cfg.Hooks.AfterPayout != nil {
		c.runAfterPayoutHook(c.cfg.Hooks.AfterPayout, payload, tx.Reference, string(tx.Status), err)
	}

	if err != nil {
		return nil, err
	}
	return &tx, nil
}
