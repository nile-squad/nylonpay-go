package nylonpay_test

import (
	"context"
	"testing"

	nylonpay "github.com/nile-squad/nylonpay-go"
)

// testClient points at an unreachable URL on purpose. Every test here asserts a
// client-side rejection, so a request that somehow escaped validation would fail
// loudly rather than quietly reaching a real backend.
func testClient(t *testing.T) *nylonpay.NylonPayClient {
	t.Helper()
	c, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
		BaseURL:   "http://127.0.0.1:1",
		Force:     true,
		// Negative disables retries: these tests want the transport to fail
		// fast, not to spend seconds backing off against a closed port.
		MaxRetries: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var bgCtx = context.Background()

func validCollectInput() nylonpay.CollectPaymentInput {
	return nylonpay.CollectPaymentInput{
		Amount:      5000,
		Currency:    nylonpay.UGX,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
	}
}

func validPayoutInput() nylonpay.MakePayoutInput {
	return nylonpay.MakePayoutInput{
		Amount:      6000,
		Currency:    nylonpay.UGX,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "payout",
		Destination: nylonpay.Destination{
			AccountHolderName: "Jane Doe",
			AccountNumber:     "1234567890",
		},
	}
}

func validInvoiceInput() nylonpay.CreateInvoiceInput {
	return nylonpay.CreateInvoiceInput{
		Amount:        5000,
		Currency:      nylonpay.UGX,
		CustomerEmail: "jane@example.com",
	}
}

// collectRejects asserts that a mutated collection input fails validation.
func collectRejects(t *testing.T, mutate func(*nylonpay.CollectPaymentInput)) {
	t.Helper()
	input := validCollectInput()
	mutate(&input)
	_, err := testClient(t).CollectPayment(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

// ── Amounts ───────────────────────────────────────────────────────────────────

func TestCollectPayment_AmountZero(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Amount = 0 })
}

func TestCollectPayment_AmountNegative(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Amount = -100 })
}

func TestCollectPayment_AmountBelowMinimum(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Amount = 499 })
}

func TestCollectPayment_AmountAtMinimumIsAccepted(t *testing.T) {
	input := validCollectInput()
	input.Amount = 500
	// Reaches the transport and fails there, which is what proves validation passed.
	_, err := testClient(t).CollectPayment(bgCtx, input)
	if err != nil {
		t.Fatalf("500 UGX is the minimum and must pass validation, got: %v", err)
	}
}

func TestMakePayout_AmountBelowMinimum(t *testing.T) {
	input := validPayoutInput()
	input.Amount = 4999
	_, err := testClient(t).MakePayout(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

// ── Required fields ───────────────────────────────────────────────────────────

func TestCollectPayment_MissingCustomerName(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Customer.Name = "" })
}

func TestCollectPayment_WhitespaceCustomerName(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Customer.Name = "   " })
}

func TestCollectPayment_MissingCustomerPhone(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Customer.PhoneNumber = "" })
}

func TestCollectPayment_MissingDescription(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Description = "" })
}

func TestMakePayout_MissingDestinationAccountHolder(t *testing.T) {
	input := validPayoutInput()
	input.Destination.AccountHolderName = ""
	_, err := testClient(t).MakePayout(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestMakePayout_MissingDestinationAccountNumber(t *testing.T) {
	input := validPayoutInput()
	input.Destination.AccountNumber = ""
	_, err := testClient(t).MakePayout(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

// ── Currency ──────────────────────────────────────────────────────────────────

func TestCollectPayment_UnsupportedCurrency(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Currency = "XYZ" })
}

func TestCollectPayment_EmptyCurrencyDefersToServerDefault(t *testing.T) {
	input := validCollectInput()
	input.Currency = ""
	if _, err := testClient(t).CollectPayment(bgCtx, input); err != nil {
		t.Fatalf("an omitted currency must be left to the server default, got: %v", err)
	}
}

// ── Phone formats ─────────────────────────────────────────────────────────────

func TestCollectPayment_InvalidPhoneFormats(t *testing.T) {
	for _, phone := range []string{"not-a-phone", "123", "07684", "0000000000000000000"} {
		t.Run(phone, func(t *testing.T) {
			collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Customer.PhoneNumber = phone })
		})
	}
}

func TestCollectPayment_AcceptedPhoneFormats(t *testing.T) {
	for _, phone := range []string{"0768499027", "+256768499027", "256768499027", "+256 768 499 027"} {
		t.Run(phone, func(t *testing.T) {
			input := validCollectInput()
			input.Customer.PhoneNumber = phone
			if _, err := testClient(t).CollectPayment(bgCtx, input); err != nil {
				t.Fatalf("%q is an accepted format, got: %v", phone, err)
			}
		})
	}
}

// ── Payment method ────────────────────────────────────────────────────────────

func TestCollectPayment_BankMethodWithoutBankDetails(t *testing.T) {
	bank := nylonpay.PaymentMethodBank
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Method = &bank })
}

func TestCollectPayment_BankMethodWithBankDetails(t *testing.T) {
	bank := nylonpay.PaymentMethodBank
	input := validCollectInput()
	input.Method = &bank
	input.Bank = &nylonpay.BankDetails{AccountNumber: "123456", BankName: "Test Bank"}
	if _, err := testClient(t).CollectPayment(bgCtx, input); err != nil {
		t.Fatalf("bank details supplied, expected validation to pass, got: %v", err)
	}
}

// ── Reference format ──────────────────────────────────────────────────────────

func TestCollectPayment_NonUUIDReferenceIsRejected(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Reference = "ORDER-2026-001" })
}

// The format the SDK generated before references became UUIDs.
func TestCollectPayment_FifteenCharReferenceIsRejected(t *testing.T) {
	collectRejects(t, func(in *nylonpay.CollectPaymentInput) { in.Reference = "a1b2c3d4e5f6789" })
}

// A UUID is what the backend requires, so validation must let it through.
func TestCollectPayment_UUIDReferenceIsAccepted(t *testing.T) {
	input := validCollectInput()
	input.Reference = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	// The call fails at the network against an unreachable BaseURL; only a
	// validation error would mean the reference itself was refused.
	if _, err := testClient(t).CollectPayment(bgCtx, input); isValidationError(err) {
		t.Fatalf("a UUID reference must pass validation, got: %v", err)
	}
}

// ── GetStatus / GetTransaction / VerifyPhone ──────────────────────────────────

func TestGetStatus_EmptyReference(t *testing.T) {
	_, err := testClient(t).GetStatus(bgCtx, nylonpay.GetStatusInput{Reference: ""})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestGetStatus_WhitespaceReference(t *testing.T) {
	_, err := testClient(t).GetStatus(bgCtx, nylonpay.GetStatusInput{Reference: "   "})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestGetTransaction_NeitherIDNorReference(t *testing.T) {
	_, err := testClient(t).GetTransaction(bgCtx, nylonpay.GetTransactionInput{})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

// Supplying both is allowed; the server reconciles them.
func TestGetTransaction_BothIDAndReferencePassesValidation(t *testing.T) {
	_, err := testClient(t).GetTransaction(bgCtx, nylonpay.GetTransactionInput{
		ID:        "txn_1",
		Reference: "ref_0123456789",
	})
	assertSDKError(t, err, nylonpay.CategoryNetwork)
}

func TestVerifyPhone_EmptyPhone(t *testing.T) {
	_, err := testClient(t).VerifyPhone(bgCtx, nylonpay.VerifyPhoneInput{PhoneNumber: ""})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestVerifyPhone_InvalidPhone(t *testing.T) {
	_, err := testClient(t).VerifyPhone(bgCtx, nylonpay.VerifyPhoneInput{PhoneNumber: "abc"})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

// ── Invoices ──────────────────────────────────────────────────────────────────

func TestCreateInvoice_AmountZero(t *testing.T) {
	input := validInvoiceInput()
	input.Amount = 0
	_, err := testClient(t).CreateInvoice(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestCreateInvoice_MissingCustomerEmail(t *testing.T) {
	input := validInvoiceInput()
	input.CustomerEmail = ""
	_, err := testClient(t).CreateInvoice(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestCreateInvoice_TooManyItems(t *testing.T) {
	input := validInvoiceInput()
	input.Items = make([]nylonpay.InvoiceItem, 51)
	for i := range input.Items {
		input.Items[i] = nylonpay.InvoiceItem{Name: "item", Quantity: 1, UnitPrice: 100}
	}
	_, err := testClient(t).CreateInvoice(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestCreateInvoice_ItemNonPositiveQuantity(t *testing.T) {
	for _, quantity := range []int64{0, -1} {
		input := validInvoiceInput()
		input.Items = []nylonpay.InvoiceItem{{Name: "item", Quantity: quantity, UnitPrice: 100}}
		_, err := testClient(t).CreateInvoice(bgCtx, input)
		assertSDKError(t, err, nylonpay.CategoryValidation)
	}
}

func TestCreateInvoice_ItemNonPositiveUnitPrice(t *testing.T) {
	for _, unitPrice := range []int64{0, -1} {
		input := validInvoiceInput()
		input.Items = []nylonpay.InvoiceItem{{Name: "item", Quantity: 1, UnitPrice: unitPrice}}
		_, err := testClient(t).CreateInvoice(bgCtx, input)
		assertSDKError(t, err, nylonpay.CategoryValidation)
	}
}

func TestCreateInvoice_NonUUIDMerchantReferenceIsRejected(t *testing.T) {
	input := validInvoiceInput()
	input.MerchantReference = "not-a-uuid"
	_, err := testClient(t).CreateInvoice(bgCtx, input)
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

// The field is optional, so an empty value must not trip the UUID check.
func TestCreateInvoice_EmptyMerchantReferenceIsAccepted(t *testing.T) {
	input := validInvoiceInput()
	input.MerchantReference = ""
	if _, err := testClient(t).CreateInvoice(bgCtx, input); isValidationError(err) {
		t.Fatalf("an omitted merchantReference must pass validation, got: %v", err)
	}
}

func TestCreateInvoice_UUIDMerchantReferenceIsAccepted(t *testing.T) {
	input := validInvoiceInput()
	input.MerchantReference = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	if _, err := testClient(t).CreateInvoice(bgCtx, input); isValidationError(err) {
		t.Fatalf("a UUID merchantReference must pass validation, got: %v", err)
	}
}

// ── Listing ───────────────────────────────────────────────────────────────────

func TestListTransactions_LimitOutOfRange(t *testing.T) {
	_, err := testClient(t).ListTransactions(bgCtx, nylonpay.ListTransactionsInput{Limit: 101})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestListTransactions_NegativeOffset(t *testing.T) {
	_, err := testClient(t).ListTransactions(bgCtx, nylonpay.ListTransactionsInput{Offset: -1})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}

func TestGetTransactionsByTag_EmptyTag(t *testing.T) {
	_, err := testClient(t).GetTransactionsByTag(bgCtx, "", nylonpay.ListTransactionsInput{})
	assertSDKError(t, err, nylonpay.CategoryValidation)
}
