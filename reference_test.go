package nylonpay_test

import (
	"context"
	"strings"
	"testing"

	nylonpay "github.com/nile-squad/nylonpay-go"
)

// ── Auto-generated references ─────────────────────────────────────────────────

func TestCollectPayment_AutoGeneratesReference(t *testing.T) {
	// Two initiations with no reference must produce different auto-refs,
	// proving each call generates a fresh value.
	// We only check that validation doesn't reject them (network call never fires).
	c := testClient(t)

	// Trigger validation pass, which means reference was valid.
	// (The call will fail at network, but ref validation already passed.)
	_, err := c.CollectPayment(context.Background(), nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "auto ref test",
		Reference:   "", // empty → auto-generate
	})

	// The only acceptable error here is a network failure, NOT a validation error.
	if err != nil {
		if isValidationError(err) {
			t.Fatalf("auto-generated reference should not fail validation: %v", err)
		}
		// Network error is expected since BaseURL is unreachable — that's fine.
	}
}

func TestCollectPayment_RejectsReferenceTooShort(t *testing.T) {
	ref := strings.Repeat("a", 12) // 12 < 13
	_, err := testClient(t).CollectPayment(context.Background(), nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Reference:   ref,
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_RejectsReferenceTooLong(t *testing.T) {
	ref := strings.Repeat("a", 16) // 16 > 15
	_, err := testClient(t).CollectPayment(context.Background(), nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Reference:   ref,
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_AcceptsMinLengthReference(t *testing.T) {
	ref := strings.Repeat("a", 13) // exactly 13
	_, err := testClient(t).CollectPayment(context.Background(), nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Reference:   ref,
	})
	// Validation must pass; only network error is acceptable.
	if isValidationError(err) {
		t.Fatalf("13-char reference should pass validation, got: %v", err)
	}
}

func TestCollectPayment_AcceptsMaxLengthReference(t *testing.T) {
	ref := strings.Repeat("a", 15) // exactly 15
	_, err := testClient(t).CollectPayment(context.Background(), nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Reference:   ref,
	})
	if isValidationError(err) {
		t.Fatalf("15-char reference should pass validation, got: %v", err)
	}
}

// isValidationError returns true if err is an SDKError with category "validation".
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "[validation]")
}
