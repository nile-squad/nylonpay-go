package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/internal/utils"
	"github.com/nile-squad/nylonpay-go/types"
)

// CollectPayment initiates a payment collection and returns a PaymentInstance.
// Call instance.Wait(ctx) to block until the payment settles.
func (c *NylonPayClient) CollectPayment(ctx context.Context, input types.CollectPaymentPayload) (*core.PaymentInstance, error) {
	ref, err := c.resolveReference(input.Reference)
	if err != nil {
		return nil, err
	}
	input.Reference = ref

	if err := c.validateCollection(input); err != nil {
		return nil, err
	}
	input.Customer.PhoneNumber = utils.NormalizePhone(input.Customer.PhoneNumber)

	payload := &input
	if c.cfg.Hooks != nil && c.cfg.Hooks.BeforeCollect != nil {
		payload = c.runBeforeCollectHook(c.cfg.Hooks.BeforeCollect, payload)
	}

	var initResp struct {
		Reference string `json:"reference"`
		Status    string `json:"status"`
	}
	if err := c.transport.Send(ctx, core.TransportRequest{Action: "sdk-collect-payment", Payload: payload}, &initResp); err != nil {
		if c.cfg.Hooks != nil && c.cfg.Hooks.AfterCollect != nil {
			c.runAfterCollectHook(c.cfg.Hooks.AfterCollect, payload, "", "", err)
		}
		return nil, err
	}

	if c.cfg.Hooks != nil && c.cfg.Hooks.AfterCollect != nil {
		c.runAfterCollectHook(c.cfg.Hooks.AfterCollect, payload, initResp.Reference, initResp.Status, nil)
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

// CollectPaymentAndResolve initiates a collection and blocks until terminal.
func (c *NylonPayClient) CollectPaymentAndResolve(ctx context.Context, input types.CollectPaymentPayload) (*types.Transaction, error) {
	ref, err := c.resolveReference(input.Reference)
	if err != nil {
		return nil, err
	}
	input.Reference = ref

	if err := c.validateCollection(input); err != nil {
		return nil, err
	}
	input.Customer.PhoneNumber = utils.NormalizePhone(input.Customer.PhoneNumber)

	payload := &input
	if c.cfg.Hooks != nil && c.cfg.Hooks.BeforeCollect != nil {
		payload = c.runBeforeCollectHook(c.cfg.Hooks.BeforeCollect, payload)
	}

	var tx types.Transaction
	err = c.transport.Send(ctx, core.TransportRequest{Action: "sdk-collect-payment-and-resolve", Payload: payload}, &tx)

	if c.cfg.Hooks != nil && c.cfg.Hooks.AfterCollect != nil {
		c.runAfterCollectHook(c.cfg.Hooks.AfterCollect, payload, tx.Reference, string(tx.Status), err)
	}

	if err != nil {
		return nil, err
	}
	return &tx, nil
}