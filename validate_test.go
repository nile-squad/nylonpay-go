package nylonpay_test

import (
	"context"
	"testing"

	nylonpay "github.com/nile-squad/nylonpay-go"
	"github.com/nile-squad/nylonpay-go/types"
)

// testClient returns a client pointing at a non-existent URL.
// Safe for validation tests because errors are returned before any HTTP call.
func testClient(t *testing.T) *nylonpay.NylonPayClient {
	t.Helper()
	c, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
		BaseURL:   "http://localhost:0", // unreachable — validation fires first
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var bgCtx = context.Background()

// ── CollectPayment validation ─────────────────────────────────────────────────

func TestCollectPayment_AmountZero(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      0,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_AmountBelowMinimum(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      100, // minimum is 500
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_MissingCustomerName(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{PhoneNumber: "0771234567"},
		Description: "test",
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_MissingCustomerPhone(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane"},
		Description: "test",
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_MissingDescription(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:   1000,
		Customer: nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_BankMethodWithoutBankDetails(t *testing.T) {
	m := types.PaymentMethodBank
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Method:      &m,
		Bank:        nil,
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_ReferenceTooshort(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Reference:   "short", // < 13 chars
	})
	assertSDKError(t, err, "validation")
}

func TestCollectPayment_ReferenceTooLong(t *testing.T) {
	_, err := testClient(t).CollectPayment(bgCtx, nylonpay.CollectPaymentPayload{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
		Reference:   "thisreferenceiswaaaytoolong", // > 15 chars
	})
	assertSDKError(t, err, "validation")
}

// ── MakePayout validation ─────────────────────────────────────────────────────

func TestMakePayout_AmountBelowMinimum(t *testing.T) {
	_, err := testClient(t).MakePayout(bgCtx, nylonpay.MakePayoutPayload{
		Amount:      1000, // minimum is 5000
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "payout",
		Destination: nylonpay.Destination{AccountHolderName: "Jane", AccountNumber: "077123456"},
	})
	assertSDKError(t, err, "validation")
}

func TestMakePayout_MissingDestinationAccountHolder(t *testing.T) {
	_, err := testClient(t).MakePayout(bgCtx, nylonpay.MakePayoutPayload{
		Amount:      6000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "payout",
		Destination: nylonpay.Destination{AccountNumber: "077123456"},
	})
	assertSDKError(t, err, "validation")
}

func TestMakePayout_MissingDestinationAccountNumber(t *testing.T) {
	_, err := testClient(t).MakePayout(bgCtx, nylonpay.MakePayoutPayload{
		Amount:      6000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "payout",
		Destination: nylonpay.Destination{AccountHolderName: "Jane"},
	})
	assertSDKError(t, err, "validation")
}

// ── GetStatus validation ──────────────────────────────────────────────────────

func TestGetStatus_EmptyReference(t *testing.T) {
	_, err := testClient(t).GetStatus(bgCtx, "")
	assertSDKError(t, err, "validation")
}

func TestGetStatus_WhitespaceReference(t *testing.T) {
	_, err := testClient(t).GetStatus(bgCtx, "   ")
	assertSDKError(t, err, "validation")
}

// ── GetTransaction validation ─────────────────────────────────────────────────

func TestGetTransaction_NeitherIDNorReference(t *testing.T) {
	_, err := testClient(t).GetTransaction(bgCtx, nylonpay.GetTransactionInput{})
	assertSDKError(t, err, "validation")
}

// ── VerifyPhone validation ────────────────────────────────────────────────────

func TestVerifyPhone_EmptyPhone(t *testing.T) {
	_, err := testClient(t).VerifyPhone(bgCtx, "")
	assertSDKError(t, err, "validation")
}

// ── CreateInvoice validation ──────────────────────────────────────────────────

func TestCreateInvoice_AmountZero(t *testing.T) {
	_, err := testClient(t).CreateInvoice(bgCtx, nylonpay.CreateInvoicePayload{
		Amount:      0,
		Description: "test invoice",
	})
	assertSDKError(t, err, "validation")
}

func TestCreateInvoice_MissingDescription(t *testing.T) {
	_, err := testClient(t).CreateInvoice(bgCtx, nylonpay.CreateInvoicePayload{
		Amount: 1000,
	})
	assertSDKError(t, err, "validation")
}

func TestCreateInvoice_TooManyItems(t *testing.T) {
	items := make([]nylonpay.InvoiceItem, 51)
	for i := range items {
		items[i] = nylonpay.InvoiceItem{Name: "item", Quantity: 1, UnitPrice: 100}
	}
	_, err := testClient(t).CreateInvoice(bgCtx, nylonpay.CreateInvoicePayload{
		Amount:      1000,
		Description: "test",
		Items:       &items,
	})
	assertSDKError(t, err, "validation")
}

func TestCreateInvoice_ItemZeroQuantity(t *testing.T) {
	items := []nylonpay.InvoiceItem{{Name: "bad", Quantity: 0, UnitPrice: 100}}
	_, err := testClient(t).CreateInvoice(bgCtx, nylonpay.CreateInvoicePayload{
		Amount:      1000,
		Description: "test",
		Items:       &items,
	})
	assertSDKError(t, err, "validation")
}

func TestCreateInvoice_ItemZeroUnitPrice(t *testing.T) {
	items := []nylonpay.InvoiceItem{{Name: "bad", Quantity: 1, UnitPrice: 0}}
	_, err := testClient(t).CreateInvoice(bgCtx, nylonpay.CreateInvoicePayload{
		Amount:      1000,
		Description: "test",
		Items:       &items,
	})
	assertSDKError(t, err, "validation")
}
