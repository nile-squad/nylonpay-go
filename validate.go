package nylonpay

import (
	"fmt"
	"strings"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

func (c *NylonPayClient) validateAmount(amount int64, min int64, label string) error {
	if amount <= 0 {
		return &core.SDKError{Category: "validation", Message: "amount must be a positive integer"}
	}
	if amount < min {
		return &core.SDKError{Category: "validation", Message: fmt.Sprintf("%s amount must be at least %d UGX", label, min)}
	}
	return nil
}

func (c *NylonPayClient) validateCollection(in types.CollectPaymentPayload) error {
	if err := c.validateAmount(in.Amount, 500, "Collection"); err != nil {
		return err
	}
	if strings.TrimSpace(in.Customer.Name) == "" {
		return &core.SDKError{Category: "validation", Message: "customer.name is required"}
	}
	if strings.TrimSpace(in.Customer.PhoneNumber) == "" {
		return &core.SDKError{Category: "validation", Message: "customer.phoneNumber is required"}
	}
	if strings.TrimSpace(in.Description) == "" {
		return &core.SDKError{Category: "validation", Message: "description is required"}
	}
	if in.Method != nil && *in.Method == types.PaymentMethodBank && in.Bank == nil {
		return &core.SDKError{Category: "validation", Message: `bank details are required when method is "bank"`}
	}
	return nil
}

func (c *NylonPayClient) validatePayout(in types.MakePayoutPayload) error {
	if err := c.validateAmount(in.Amount, 5000, "Payout"); err != nil {
		return err
	}
	if strings.TrimSpace(in.Customer.Name) == "" {
		return &core.SDKError{Category: "validation", Message: "customer.name is required"}
	}
	if strings.TrimSpace(in.Customer.PhoneNumber) == "" {
		return &core.SDKError{Category: "validation", Message: "customer.phoneNumber is required"}
	}
	if strings.TrimSpace(in.Description) == "" {
		return &core.SDKError{Category: "validation", Message: "description is required"}
	}
	if strings.TrimSpace(in.Destination.AccountHolderName) == "" {
		return &core.SDKError{Category: "validation", Message: "destination.accountHolderName is required"}
	}
	if strings.TrimSpace(in.Destination.AccountNumber) == "" {
		return &core.SDKError{Category: "validation", Message: "destination.accountNumber is required"}
	}
	return nil
}
