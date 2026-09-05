package nylonpay

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nile-squad/nylonpay-go/internal/utils"
	"github.com/nile-squad/nylonpay-go/types"
)

// Minimum amounts the backend enforces, mirrored here so bad input fails
// before a network round-trip.
const (
	minCollectionAmount = 500
	minPayoutAmount     = 5000
	maxInvoiceItems     = 50
)

// normalizedPhonePattern matches a phone number after normalization. It is
// deliberately loose, the provider is the real authority; this only catches
// input that could never be a phone number.
var normalizedPhonePattern = regexp.MustCompile(`^[0-9]{9,15}$`)

func validationErr(format string, args ...any) error {
	return &types.SDKError{
		Category: types.CategoryValidation,
		Message:  fmt.Sprintf(format, args...),
	}
}

// validateAmount rejects non-positive and sub-minimum amounts. Amounts are
// int64 throughout the SDK, so a non-integer amount cannot be expressed and the
// spec's integer-only wire rule holds structurally.
func validateAmount(amount int64, minimum int64, label string) error {
	if amount <= 0 {
		return validationErr("amount must be a positive integer")
	}
	if amount < minimum {
		return validationErr("%s amount must be at least %d UGX", label, minimum)
	}
	return nil
}

func requireNonEmpty(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return validationErr("%s is required", field)
	}
	return nil
}

// validateCurrency accepts an empty value, which lets the backend apply its
// default of UGX.
func validateCurrency(currency types.Currency) error {
	if currency == "" {
		return nil
	}
	if !types.SupportedCurrencies[currency] {
		return validationErr("currency %q is not supported", currency)
	}
	return nil
}

func validatePhone(normalized, field string) error {
	if !normalizedPhonePattern.MatchString(normalized) {
		return validationErr("%s must be a valid phone number", field)
	}
	return nil
}

// prepareCollect validates and normalizes a collection input, returning the
// exact value that should go on the wire.
//
// It is safe to run more than once, which is what lets a before* hook's output
// be re-checked: normalization is idempotent, and an already-valid reference is
// returned unchanged rather than regenerated.
func (c *NylonPayClient) prepareCollect(input types.CollectPaymentInput) (types.CollectPaymentInput, error) {
	reference, err := resolveReference(input.Reference)
	if err != nil {
		return input, err
	}
	input.Reference = reference

	if err := validateAmount(input.Amount, minCollectionAmount, "Collection"); err != nil {
		return input, err
	}
	if err := validateCurrency(input.Currency); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.Customer.Name, "customer.name"); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.Customer.PhoneNumber, "customer.phoneNumber"); err != nil {
		return input, err
	}

	input.Customer.PhoneNumber = utils.NormalizePhone(input.Customer.PhoneNumber)
	if err := validatePhone(input.Customer.PhoneNumber, "customer.phoneNumber"); err != nil {
		return input, err
	}

	if err := requireNonEmpty(input.Description, "description"); err != nil {
		return input, err
	}
	if input.Method != nil && *input.Method == types.PaymentMethodBank && input.Bank == nil {
		return input, validationErr(`bank details are required when method is "bank"`)
	}

	return input, nil
}

// preparePayout validates and normalizes a payout input. Like prepareCollect it
// is safe to run twice.
func (c *NylonPayClient) preparePayout(input types.MakePayoutInput) (types.MakePayoutInput, error) {
	reference, err := resolveReference(input.Reference)
	if err != nil {
		return input, err
	}
	input.Reference = reference

	if err := validateAmount(input.Amount, minPayoutAmount, "Payout"); err != nil {
		return input, err
	}
	if err := validateCurrency(input.Currency); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.Customer.Name, "customer.name"); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.Customer.PhoneNumber, "customer.phoneNumber"); err != nil {
		return input, err
	}

	input.Customer.PhoneNumber = utils.NormalizePhone(input.Customer.PhoneNumber)
	if err := validatePhone(input.Customer.PhoneNumber, "customer.phoneNumber"); err != nil {
		return input, err
	}

	if err := requireNonEmpty(input.Description, "description"); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.Destination.AccountHolderName, "destination.accountHolderName"); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.Destination.AccountNumber, "destination.accountNumber"); err != nil {
		return input, err
	}

	return input, nil
}

// prepareInvoice validates and normalizes an invoice input.
func (c *NylonPayClient) prepareInvoice(input types.CreateInvoiceInput) (types.CreateInvoiceInput, error) {
	if err := validateAmount(input.Amount, minCollectionAmount, "Invoice"); err != nil {
		return input, err
	}
	if err := validateCurrency(input.Currency); err != nil {
		return input, err
	}
	if err := requireNonEmpty(input.CustomerEmail, "customerEmail"); err != nil {
		return input, err
	}

	if input.CustomerPhone != nil {
		normalized := utils.NormalizePhone(*input.CustomerPhone)
		if err := validatePhone(normalized, "customerPhone"); err != nil {
			return input, err
		}
		input.CustomerPhone = &normalized
	}

	// A merchant reference is optional here, but when supplied it must be a
	// UUID like any other reference.
	if input.MerchantReference != "" {
		if err := validateReferenceFormat(input.MerchantReference); err != nil {
			return input, err
		}
	}

	if len(input.Items) > maxInvoiceItems {
		return input, validationErr("items must not exceed %d", maxInvoiceItems)
	}
	for _, item := range input.Items {
		if err := requireNonEmpty(item.Name, "item name"); err != nil {
			return input, err
		}
		if item.Quantity <= 0 {
			return input, validationErr("item quantity must be a positive integer")
		}
		if item.UnitPrice <= 0 {
			return input, validationErr("item unitPrice must be a positive integer")
		}
	}

	return input, nil
}
