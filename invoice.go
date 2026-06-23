package nylonpay

import (
	"context"
	"strings"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

func (c *NylonPayClient) CreateInvoice(ctx context.Context, input types.CreateInvoicePayload) (*types.InvoiceResponse, error) {
	ref, err := c.resolveReference(input.Reference)
	if err != nil {
		return nil, err
	}
	input.Reference = ref

	if err := c.validateAmount(input.Amount, 500, "Collection"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Description) == "" {
		return nil, &core.SDKError{Category: "validation", Message: "description is required"}
	}

	if input.Items != nil {
		if len(*input.Items) > 50 {
			return nil, &core.SDKError{Category: "validation", Message: "items must not exceed 50"}
		}
		for _, item := range *input.Items {
			if item.Quantity <= 0 {
				return nil, &core.SDKError{Category: "validation", Message: "item quantity must be a positive integer"}
			}
			if item.UnitPrice <= 0 {
				return nil, &core.SDKError{Category: "validation", Message: "item unitPrice must be a positive integer"}
			}
		}
	}

	var resp types.InvoiceResponse
	if err := c.transport.Send(ctx, core.TransportRequest{Action: "sdk-create-invoice", Payload: input}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
