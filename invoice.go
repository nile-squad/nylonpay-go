package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// CreateInvoice generates a hosted invoice and emails it to the customer.
//
// The returned PaymentLink opens a checkout page where the customer pays; a
// receipt email follows automatically on success. This is also the supported
// route for card payments, which the SDK deliberately does not accept directly.
//
// Invoices are live-mode only: called with a sandbox key, the server returns a
// validation error.
func (c *NylonPayClient) CreateInvoice(ctx context.Context, input types.CreateInvoiceInput) (*types.InvoiceResponse, error) {
	prepared, err := c.prepareInvoice(input)
	if err != nil {
		return nil, err
	}

	var invoice types.InvoiceResponse
	if err := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionCreateInvoice,
		Payload: prepared,
	}, &invoice); err != nil {
		return nil, err
	}
	return &invoice, nil
}
