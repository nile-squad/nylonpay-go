//go:build integration

package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	nylonpay "github.com/nile-squad/nylonpay-go"
)

// Run integration tests with:
//
//	NYLONPAY_API_KEY=npk_... NYLONPAY_API_SECRET=nps_... go test ./tests/integration/ -tags integration -v

func newClient(t *testing.T) *nylonpay.NylonPayClient {
	t.Helper()
	apiKey := os.Getenv("NYLONPAY_API_KEY")
	apiSecret := os.Getenv("NYLONPAY_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		t.Skip("NYLONPAY_API_KEY and NYLONPAY_API_SECRET must be set to run integration tests")
	}
	c, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    apiKey,
		APISecret: apiSecret,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func ctx() context.Context {
	c, _ := context.WithTimeout(context.Background(), 30*time.Second)
	return c
}

// ── Phone verification ────────────────────────────────────────────────────────

func TestIntegration_VerifyPhone_ValidNumber(t *testing.T) {
	c := newClient(t)
	phone := os.Getenv("NYLONPAY_TEST_PHONE")
	if phone == "" {
		t.Skip("NYLONPAY_TEST_PHONE not set")
	}

	resp, err := c.VerifyPhone(ctx(), phone)
	if err != nil {
		t.Fatalf("VerifyPhone: %v", err)
	}
	if resp.PhoneNumber == "" {
		t.Error("expected non-empty PhoneNumber in response")
	}
}

func TestIntegration_VerifyPhone_EmptyPhone_ReturnsValidationError(t *testing.T) {
	c := newClient(t)
	_, err := c.VerifyPhone(ctx(), "")
	if err == nil {
		t.Fatal("expected validation error for empty phone")
	}
}

// ── Invoice creation ──────────────────────────────────────────────────────────

func TestIntegration_CreateInvoice(t *testing.T) {
	c := newClient(t)

	resp, err := c.CreateInvoice(ctx(), nylonpay.CreateInvoicePayload{
		Amount:      5000,
		Currency:    nylonpay.UGX,
		Description: "Integration test invoice",
	})
	if err != nil {
		if strings.Contains(err.Error(), "sandbox mode") {
			t.Skipf("CreateInvoice not available in sandbox: %v", err)
		}
		t.Fatalf("CreateInvoice: %v", err)
	}
	if resp.Url == "" {
		t.Error("expected invoice URL in response")
	}
	if resp.Token == "" {
		t.Error("expected invoice token in response")
	}
}

func TestIntegration_CreateInvoice_WithItems(t *testing.T) {
	c := newClient(t)

	items := []nylonpay.InvoiceItem{
		{Name: "Widget A", Quantity: 2, UnitPrice: 1500},
		{Name: "Widget B", Quantity: 1, UnitPrice: 2000},
	}
	resp, err := c.CreateInvoice(ctx(), nylonpay.CreateInvoicePayload{
		Amount:      5000,
		Currency:    nylonpay.UGX,
		Description: "Integration test invoice with items",
		Items:       &items,
	})
	if err != nil {
		if strings.Contains(err.Error(), "sandbox mode") {
			t.Skipf("CreateInvoice not available in sandbox: %v", err)
		}
		t.Fatalf("CreateInvoice with items: %v", err)
	}
	if resp.Url == "" {
		t.Error("expected invoice URL")
	}
}

// ── Payment initiation ────────────────────────────────────────────────────────

func TestIntegration_CollectPayment_InitiatesAndReturnsPendingInstance(t *testing.T) {
	c := newClient(t)
	phone := os.Getenv("NYLONPAY_TEST_PHONE")
	if phone == "" {
		t.Skip("NYLONPAY_TEST_PHONE not set")
	}

	instance, err := c.CollectPayment(ctx(), nylonpay.CollectPaymentPayload{
		Amount:      500,
		Currency:    nylonpay.UGX,
		Description: "Integration test collection",
		Customer: nylonpay.Customer{
			Name:        "Test User",
			PhoneNumber: phone,
		},
	})
	if err != nil {
		t.Fatalf("CollectPayment: %v", err)
	}
	if instance == nil {
		t.Fatal("expected non-nil PaymentInstance")
	}
	if instance.Reference() == "" {
		t.Error("expected non-empty reference")
	}
}

func TestIntegration_GetStatus_AfterInitiation(t *testing.T) {
	c := newClient(t)
	phone := os.Getenv("NYLONPAY_TEST_PHONE")
	if phone == "" {
		t.Skip("NYLONPAY_TEST_PHONE not set")
	}

	instance, err := c.CollectPayment(ctx(), nylonpay.CollectPaymentPayload{
		Amount:      500,
		Currency:    nylonpay.UGX,
		Description: "Status check test",
		Customer:    nylonpay.Customer{Name: "Test User", PhoneNumber: phone},
	})
	if err != nil {
		t.Fatalf("CollectPayment: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	status, err := c.GetStatus(ctx(), instance.Reference())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.Reference != instance.Reference() {
		t.Errorf("reference mismatch: got %q, want %q", status.Reference, instance.Reference())
	}
}

func TestIntegration_GetTransaction_ByReference(t *testing.T) {
	c := newClient(t)
	phone := os.Getenv("NYLONPAY_TEST_PHONE")
	if phone == "" {
		t.Skip("NYLONPAY_TEST_PHONE not set")
	}

	instance, err := c.CollectPayment(ctx(), nylonpay.CollectPaymentPayload{
		Amount:      500,
		Currency:    nylonpay.UGX,
		Description: "GetTransaction test",
		Customer:    nylonpay.Customer{Name: "Test User", PhoneNumber: phone},
	})
	if err != nil {
		t.Fatalf("CollectPayment: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	tx, err := c.GetTransaction(ctx(), nylonpay.GetTransactionInput{
		Reference: instance.Reference(),
	})
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if tx.Reference != instance.Reference() {
		t.Errorf("reference = %q, want %q", tx.Reference, instance.Reference())
	}
}

// ── Auth errors ───────────────────────────────────────────────────────────────

func TestIntegration_WrongCredentials_ReturnsAuthError(t *testing.T) {
	c, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_invalid_key_for_testing",
		APISecret: "nps_invalid_secret_for_testing",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.GetStatus(ctx(), "someref1234567")
	if err == nil {
		t.Fatal("expected auth error for wrong credentials")
	}
}

// ── Idempotency ───────────────────────────────────────────────────────────────

func TestIntegration_SameReference_IsIdempotent(t *testing.T) {
	c := newClient(t)
	phone := os.Getenv("NYLONPAY_TEST_PHONE")
	if phone == "" {
		t.Skip("NYLONPAY_TEST_PHONE not set")
	}

	ref := "inttest1234567" // fixed 14-char reference

	payload := nylonpay.CollectPaymentPayload{
		Amount:      500,
		Currency:    nylonpay.UGX,
		Description: "Idempotency test",
		Reference:   ref,
		Customer:    nylonpay.Customer{Name: "Test User", PhoneNumber: phone},
	}

	i1, err := c.CollectPayment(ctx(), payload)
	if err != nil {
		t.Fatalf("first CollectPayment: %v", err)
	}

	i2, err := c.CollectPayment(ctx(), payload)
	if err != nil {
		t.Fatalf("second CollectPayment (same ref): %v", err)
	}

	if i1.Reference() != i2.Reference() {
		t.Errorf("references must match for idempotent calls: %q vs %q", i1.Reference(), i2.Reference())
	}
}
