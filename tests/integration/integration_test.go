//go:build integration

// Package integration_test is the canonical Integration Test suite (I1-I19)
// every Nylon Pay SDK ships, run against a real sandbox backend rather than
// mocked transport. Test names carry their spec ID so coverage can be traced
// from the spec to the code.
//
//	go test -tags=integration ./tests/integration/
//
// Requires NYLONPAY_API_KEY and NYLONPAY_API_SECRET; the suite skips cleanly
// without them. Tests that need live-only behaviour are gated on
// NYLONPAY_TEST_MODE=live.
package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	nylonpay "github.com/nile-squad/nylonpay-go"
	"github.com/nile-squad/nylonpay-go/types"
)

const (
	// The sandbox settles in roughly 2-8 seconds; the margin is for latency,
	// not for asserting on it.
	resolveTimeout = 90 * time.Second
)

// newClient builds a fresh client per test. Force bypasses the singleton cache
// so no state leaks between tests.
// requireCredentials skips a test unless sandbox credentials are configured,
// and returns them.
func requireCredentials(t *testing.T) (string, string) {
	t.Helper()

	if os.Getenv("GO_ENV") == "testing" {
		err := godotenv.Load()
		if err != nil {
			log.Fatalf("Error loading .env file")
		}
		log.Println("Loaded .env var file")
	}

	apiKey := os.Getenv("NYLONPAY_API_KEY")
	apiSecret := os.Getenv("NYLONPAY_API_SECRET")
	// log.println("apiKey: ", apiKey)
	// log.println("apiSecret: ", apiSecret)
	if apiKey == "" || apiSecret == "" {
		t.Skip("set NYLONPAY_API_KEY and NYLONPAY_API_SECRET to run integration tests")
	}
	return apiKey, apiSecret
}

func newClient(t *testing.T) *nylonpay.NylonPayClient {
	t.Helper()

	apiKey, apiSecret := requireCredentials(t)

	cfg := nylonpay.Config{
		APIKey:          apiKey,
		APISecret:       apiSecret,
		Force:           true,
		MaxPollDuration: resolveTimeout,
	}
	if baseURL := os.Getenv("NYLONPAY_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	client, err := nylonpay.NewClient(cfg)
	if err != nil {
		t.Fatalf("client construction failed: %v", err)
	}
	return client
}

func testPhone(t *testing.T) string {
	t.Helper()
	if phone := os.Getenv("NYLONPAY_TEST_PHONE"); phone != "" {
		return phone
	}
	return "0768499027"
}

func requireLiveMode(t *testing.T) {
	t.Helper()
	if os.Getenv("NYLONPAY_TEST_MODE") != "live" {
		t.Skip("live-only: set NYLONPAY_TEST_MODE=live")
	}
}

// uniqueReference returns a fresh v4 UUID, so tests never collide with each
// other's idempotency.
//
// This deliberately does not call the SDK's own generator: a test that borrows
// the code under test cannot catch a generator that drifted off the format the
// backend accepts.
func uniqueReference(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("reference generation failed: %v", err)
	}

	buf[6] = buf[6]&0x0f | 0x40 // version 4
	buf[8] = buf[8]&0x3f | 0x80 // RFC 4122 variant

	encoded := hex.EncodeToString(buf)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

// requireNoImmediateError fails the test if initiation was rejected by the
// server.
//
// A create call reports a server-side rejection on the instance rather than
// returning it, and populates Reference() from the input either way. So a
// payment the backend refused outright still passes a Reference() check; the
// error event is the only place the refusal is visible. Without this, a suite
// running against a backend that rejects every request reports green.
func requireNoImmediateError(t *testing.T, instance *nylonpay.PaymentInstance) {
	t.Helper()

	failure := make(chan error, 1)
	instance.Once(types.PaymentEventError, func(data types.EventData) {
		select {
		case failure <- data.Err:
		default:
		}
	})

	select {
	case err := <-failure:
		t.Fatalf("initiation was rejected: %v", err)
	case <-time.After(2 * time.Second):
	}
}

// getTransactionEventually looks a transaction up, tolerating a not_found while
// the write propagates to the read path.
//
// The SDK's own poller makes the same allowance for the same reason. A direct
// lookup issued immediately after initiation does not, which turns ordinary
// replication lag into a failed test. Any other error returns straight away, so
// a real failure still fails fast.
func getTransactionEventually(
	t *testing.T,
	client nylonpay.Client,
	ctx context.Context,
	reference string,
) (*types.Transaction, error) {
	t.Helper()

	const (
		attempts = 15
		interval = time.Second
	)

	var err error
	for attempt := range attempts {
		var transaction *types.Transaction
		transaction, err = client.GetTransaction(ctx, types.GetTransactionInput{Reference: reference})
		if err == nil {
			return transaction, nil
		}

		var sdkErr *nylonpay.SDKError
		if !errors.As(err, &sdkErr) || sdkErr.Category != types.CategoryNotFound {
			return nil, err
		}
		if attempt < attempts-1 {
			time.Sleep(interval)
		}
	}
	return nil, err
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout+30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func collectInput(t *testing.T, reference string) types.CollectPaymentInput {
	t.Helper()
	return types.CollectPaymentInput{
		Amount:      1000,
		Currency:    nylonpay.UGX,
		Reference:   reference,
		Description: "Integration test collection",
		Customer:    types.Customer{Name: "Integration Tester", PhoneNumber: testPhone(t)},
		Metadata:    map[string]string{"suite": "integration", "reference": reference},
	}
}

func payoutInput(t *testing.T, reference string) types.MakePayoutInput {
	t.Helper()
	return types.MakePayoutInput{
		Amount:      5000,
		Currency:    nylonpay.UGX,
		Reference:   reference,
		Description: "Integration test payout",
		Customer:    types.Customer{Name: "Integration Tester", PhoneNumber: testPhone(t)},
		Destination: types.Destination{
			AccountHolderName: "Integration Tester",
			AccountNumber:     testPhone(t),
		},
		Metadata: map[string]string{"suite": "integration"},
	}
}

func categoryOf(t *testing.T, err error) types.ErrorCategory {
	t.Helper()
	var sdkErr *nylonpay.SDKError
	if !errors.As(err, &sdkErr) {
		t.Fatalf("error %v is %T, want *nylonpay.SDKError", err, err)
	}
	return sdkErr.Category
}

// ── I1-I3: collections ────────────────────────────────────────────────────────

func TestI1_CollectPaymentHappyPath(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)

	instance, err := client.CollectPayment(ctx, collectInput(t, reference))
	if err != nil {
		t.Fatalf("I1: client-side validation failed: %v", err)
	}
	defer instance.Close()
	requireNoImmediateError(t, instance)

	if instance.Reference() != reference {
		t.Errorf("I1: reference = %q, want %q", instance.Reference(), reference)
	}
	if status := instance.Status(); status.IsTerminal() {
		t.Logf("I1: sandbox resolved immediately with %s", status)
	}
}

func TestI2_GetTransactionAfterCollect(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)

	instance, err := client.CollectPayment(ctx, collectInput(t, reference))
	if err != nil {
		t.Fatalf("I2: collect failed: %v", err)
	}
	defer instance.Close()
	requireNoImmediateError(t, instance)

	transaction, err := getTransactionEventually(t, client, ctx, reference)
	if err != nil {
		t.Fatalf("I2: lookup failed: %v", err)
	}
	if transaction.Reference != reference {
		t.Errorf("I2: reference = %q, want %q", transaction.Reference, reference)
	}
	if transaction.Type != types.TransactionTypeCollection {
		t.Errorf("I2: type = %q, want collection", transaction.Type)
	}
}

// Same reference means same transaction: the server replays rather than
// charging again.
func TestI3_IdempotencyOnCollect(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)
	input := collectInput(t, reference)

	first, err := client.CollectPayment(ctx, input)
	if err != nil {
		t.Fatalf("I3: first collect failed: %v", err)
	}
	defer first.Close()
	requireNoImmediateError(t, first)

	second, err := client.CollectPayment(ctx, input)
	if err != nil {
		t.Fatalf("I3: second collect failed: %v", err)
	}
	defer second.Close()
	requireNoImmediateError(t, second)

	if first.Reference() != second.Reference() {
		t.Errorf("I3: reusing a reference produced two transactions: %q and %q",
			first.Reference(), second.Reference())
	}

	transaction, err := getTransactionEventually(t, client, ctx, reference)
	if err != nil {
		t.Fatalf("I3: lookup failed: %v", err)
	}
	if transaction.Reference != reference {
		t.Errorf("I3: reference = %q, want %q", transaction.Reference, reference)
	}
}

// ── I4-I6: payouts ────────────────────────────────────────────────────────────

func TestI4_MakePayoutHappyPath(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)

	instance, err := client.MakePayout(ctx, payoutInput(t, reference))
	if err != nil {
		t.Fatalf("I4: client-side validation failed: %v", err)
	}
	defer instance.Close()
	requireNoImmediateError(t, instance)

	if instance.Reference() != reference {
		t.Errorf("I4: reference = %q, want %q", instance.Reference(), reference)
	}
}

func TestI5_GetTransactionAfterPayout(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)

	instance, err := client.MakePayout(ctx, payoutInput(t, reference))
	if err != nil {
		t.Fatalf("I5: payout failed: %v", err)
	}
	defer instance.Close()
	requireNoImmediateError(t, instance)

	transaction, err := getTransactionEventually(t, client, ctx, reference)
	if err != nil {
		t.Fatalf("I5: lookup failed: %v", err)
	}
	if transaction.Type != types.TransactionTypePayout {
		t.Errorf("I5: type = %q, want payout", transaction.Type)
	}
}

func TestI6_IdempotencyOnPayout(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)
	input := payoutInput(t, reference)

	first, err := client.MakePayout(ctx, input)
	if err != nil {
		t.Fatalf("I6: first payout failed: %v", err)
	}
	defer first.Close()
	requireNoImmediateError(t, first)

	second, err := client.MakePayout(ctx, input)
	if err != nil {
		t.Fatalf("I6: second payout failed: %v", err)
	}
	defer second.Close()
	requireNoImmediateError(t, second)

	if first.Reference() != second.Reference() {
		t.Errorf("I6: reusing a reference produced two payouts: %q and %q",
			first.Reference(), second.Reference())
	}
}

// ── I7: phone verification ────────────────────────────────────────────────────

func TestI7_VerifyPhone(t *testing.T) {
	client, ctx := newClient(t), testContext(t)

	verification, err := client.VerifyPhone(ctx, types.VerifyPhoneInput{PhoneNumber: testPhone(t)})
	if err != nil {
		t.Fatalf("I7: verification failed: %v", err)
	}
	if !verification.Verified {
		t.Errorf("I7: verified = false for %s", testPhone(t))
	}
	if verification.CustomerName == "" {
		t.Error("I7: expected a customer name from the provider")
	}
}

// ── I8-I11: credential validation, before any network call ────────────────────

func TestI8_MissingAPIKeyIsRejected(t *testing.T) {
	client, err := nylonpay.NewClient(nylonpay.Config{APISecret: "nps_secret", Force: true})
	if err == nil || client != nil {
		t.Fatal("I8: a missing apiKey must be rejected at construction")
	}
	if got := categoryOf(t, err); got != types.CategoryValidation {
		t.Errorf("I8: category = %s, want validation", got)
	}
}

func TestI9_BadAPIKeyPrefixIsRejected(t *testing.T) {
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey: "pk_wrong_prefix", APISecret: "nps_secret", Force: true,
	})
	if err == nil || client != nil {
		t.Fatal(`I9: an apiKey without the "npk_" prefix must be rejected`)
	}
	if got := categoryOf(t, err); got != types.CategoryValidation {
		t.Errorf("I9: category = %s, want validation", got)
	}
}

func TestI10_MissingAPISecretIsRejected(t *testing.T) {
	client, err := nylonpay.NewClient(nylonpay.Config{APIKey: "npk_key", Force: true})
	if err == nil || client != nil {
		t.Fatal("I10: a missing apiSecret must be rejected at construction")
	}
	if got := categoryOf(t, err); got != types.CategoryValidation {
		t.Errorf("I10: category = %s, want validation", got)
	}
}

func TestI11_BadAPISecretPrefixIsRejected(t *testing.T) {
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey: "npk_key", APISecret: "sk_wrong_prefix", Force: true,
	})
	if err == nil || client != nil {
		t.Fatal(`I11: an apiSecret without the "nps_" prefix must be rejected`)
	}
	if got := categoryOf(t, err); got != types.CategoryValidation {
		t.Errorf("I11: category = %s, want validation", got)
	}
}

// ── I12: singleton behaviour ──────────────────────────────────────────────────

func TestI12_SingletonReturnsSameInstance(t *testing.T) {
	apiKey, apiSecret := requireCredentials(t)
	cfg := nylonpay.Config{APIKey: apiKey, APISecret: apiSecret}

	first, err := nylonpay.NewClient(cfg)
	if err != nil {
		t.Fatalf("I12: %v", err)
	}
	second, err := nylonpay.NewClient(cfg)
	if err != nil {
		t.Fatalf("I12: %v", err)
	}
	if first != second {
		t.Error("I12: a second call without Force must return the same client")
	}

	cfg.Force = true
	forced, err := nylonpay.NewClient(cfg)
	if err != nil {
		t.Fatalf("I12: %v", err)
	}
	if forced == first {
		t.Error("I12: Force must return a new client")
	}
}

// ── I13: unknown reference ────────────────────────────────────────────────────

func TestI13_UnknownReferenceReturnsError(t *testing.T) {
	client, ctx := newClient(t), testContext(t)

	transaction, err := client.GetTransaction(ctx, types.GetTransactionInput{
		Reference: uniqueReference(t), // never used for a transaction
	})
	if err == nil {
		t.Fatalf("I13: expected an error, got transaction %+v", transaction)
	}
	if got := categoryOf(t, err); got != types.CategoryNotFound {
		t.Logf("I13: category = %s (not_found preferred, but any error is a pass here)", got)
	}
}

// ── I14: minimum amounts ──────────────────────────────────────────────────────

func TestI14_SubMinimumCollectionAmountIsRejected(t *testing.T) {
	client, ctx := newClient(t), testContext(t)

	input := collectInput(t, uniqueReference(t))
	input.Amount = 499
	_, err := client.CollectPayment(ctx, input)
	if err == nil {
		t.Fatal("I14: an amount below 500 UGX must be rejected")
	}
	if got := categoryOf(t, err); got != types.CategoryValidation {
		t.Errorf("I14: category = %s, want validation", got)
	}
}

func TestI14b_SubMinimumPayoutAmountIsRejected(t *testing.T) {
	client, ctx := newClient(t), testContext(t)

	input := payoutInput(t, uniqueReference(t))
	input.Amount = 4999
	_, err := client.MakePayout(ctx, input)
	if err == nil {
		t.Fatal("I14b: an amount below 5000 UGX must be rejected")
	}
	if got := categoryOf(t, err); got != types.CategoryValidation {
		t.Errorf("I14b: category = %s, want validation", got)
	}
}

// ── I15, I16: authentication failures ─────────────────────────────────────────

// I15 needs a genuinely revoked key, which only exists in live mode.
func TestI15_RevokedKeyIsRejected(t *testing.T) {
	requireLiveMode(t)

	revokedKey := os.Getenv("NYLONPAY_REVOKED_API_KEY")
	revokedSecret := os.Getenv("NYLONPAY_REVOKED_API_SECRET")
	if revokedKey == "" || revokedSecret == "" {
		t.Skip("set NYLONPAY_REVOKED_API_KEY and NYLONPAY_REVOKED_API_SECRET for I15")
	}

	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey: revokedKey, APISecret: revokedSecret, Force: true,
	})
	if err != nil {
		t.Fatalf("I15: a revoked key is still well-formed, so construction must succeed: %v", err)
	}

	instance, err := client.CollectPayment(testContext(t), collectInput(t, uniqueReference(t)))
	if err != nil {
		t.Fatalf("I15: a revoked key is a server-side rejection, not a validation error: %v", err)
	}
	defer instance.Close()

	if _, waitErr := instance.Wait(testContext(t)); waitErr == nil {
		t.Fatal("I15: expected the collection to fail with a revoked key")
	} else if got := categoryOf(t, waitErr); got != types.CategoryAuth {
		t.Errorf("I15: category = %s, want auth", got)
	}
}

// A well-formed but unknown key: getStatus returns an error result, while
// collectPayment surfaces it on the instance rather than returning it.
func TestI16_UnknownKeyYieldsAuthCategory(t *testing.T) {
	// Gated on credentials like the rest of the suite: this test talks to a
	// backend, and which backend matters. Without the gate it would reach the
	// production endpoint from any machine that merely ran the suite.
	requireCredentials(t)

	cfg := nylonpay.Config{
		APIKey:     "npk_test_unknown_key_000000000000",
		APISecret:  "nps_test_unknown_secret_0000000000",
		Force:      true,
		MaxRetries: -1,
	}
	if baseURL := os.Getenv("NYLONPAY_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	client, err := nylonpay.NewClient(cfg)
	if err != nil {
		t.Fatalf("I16: a well-formed unknown key must still construct: %v", err)
	}
	ctx := testContext(t)

	if _, statusErr := client.GetStatus(ctx, types.GetStatusInput{Reference: uniqueReference(t)}); statusErr == nil {
		t.Error("I16: getStatus must return an error result for an unknown key")
	} else if got := categoryOf(t, statusErr); got != types.CategoryAuth {
		t.Errorf("I16: getStatus category = %s, want auth", got)
	}

	instance, initErr := client.CollectPayment(ctx, collectInput(t, uniqueReference(t)))
	if initErr != nil {
		t.Fatalf("I16: a server-side rejection must not be returned from CollectPayment: %v", initErr)
	}
	defer instance.Close()

	waitErr := errorFromInstance(t, instance, ctx)
	if waitErr == nil {
		t.Fatal("I16: collectPayment must surface the auth failure on the instance")
	}
	if got := categoryOf(t, waitErr); got != types.CategoryAuth {
		t.Errorf("I16: instance category = %s, want auth", got)
	}
}

// errorFromInstance reads the failure an instance reports, checking both
// channels: the error event and Wait.
func errorFromInstance(t *testing.T, instance *nylonpay.PaymentInstance, ctx context.Context) error {
	t.Helper()

	events := make(chan error, 1)
	instance.Once(types.PaymentEventError, func(data types.EventData) {
		select {
		case events <- data.Err:
		default:
		}
	})

	_, waitErr := instance.Wait(ctx)

	select {
	case eventErr := <-events:
		if eventErr == nil {
			t.Error("the error event must carry the failure")
		}
	case <-time.After(time.Second):
		t.Error("expected an error event on the instance")
	}
	return waitErr
}

// ── I17: the resolve variant returns a full record ────────────────────────────

func TestI17_ResolveReturnsFullTransaction(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)

	transaction, err := client.CollectPaymentAndResolve(ctx, collectInput(t, reference))
	// A payment that reached a terminal failure still carries its full record,
	// and that is what this test is about. A request the server refused
	// outright is a different thing and returns no record at all, so report
	// that error rather than the nil it left behind.
	if err != nil && categoryOf(t, err) != types.CategoryProvider {
		t.Fatalf("I17: resolve was rejected: %v", err)
	}
	if transaction == nil {
		t.Fatalf("I17: expected a transaction even for a failed payment, got nil (err: %v)", err)
	}

	if transaction.ID == "" {
		t.Error("I17: id must be populated")
	}
	if transaction.Amount != 1000 {
		t.Errorf("I17: amount = %d, want 1000", transaction.Amount)
	}
	if transaction.Reference != reference {
		t.Errorf("I17: reference = %q, want %q", transaction.Reference, reference)
	}
	if transaction.Metadata == nil {
		t.Error("I17: metadata must be present, never a partial stub")
	}
	if transaction.Status == types.TransactionStatusFailed && transaction.FailureReason == nil {
		t.Error("I17: a failed transaction must carry a failure reason")
	}
}

// ── I18: metadata round-trip ──────────────────────────────────────────────────

func TestI18_MetadataRoundTrip(t *testing.T) {
	client, ctx := newClient(t), testContext(t)
	reference := uniqueReference(t)

	input := collectInput(t, reference)
	input.Metadata = map[string]string{
		"orderId":  "order-12345",
		"campaign": "spring-sale",
	}

	instance, err := client.CollectPayment(ctx, input)
	if err != nil {
		t.Fatalf("I18: collect failed: %v", err)
	}
	defer instance.Close()
	requireNoImmediateError(t, instance)

	transaction, err := getTransactionEventually(t, client, ctx, reference)
	if err != nil {
		t.Fatalf("I18: lookup failed: %v", err)
	}

	for key, want := range input.Metadata {
		if got := transaction.Metadata[key]; got != want {
			t.Errorf("I18: metadata[%q] = %q, want %q (the SDK must not modify it)", key, got, want)
		}
	}
}

// ── I19: polling reaches a terminal state ─────────────────────────────────────

func TestI19_PollingReachesTerminal(t *testing.T) {
	client, ctx := newClient(t), testContext(t)

	instance, err := client.CollectPayment(ctx, collectInput(t, uniqueReference(t)))
	if err != nil {
		t.Fatalf("I19: collect failed: %v", err)
	}
	defer instance.Close()
	requireNoImmediateError(t, instance)

	// Wait must return rather than hang; whether the payment succeeded is the
	// sandbox provider's business, not this test's.
	transaction, waitErr := instance.Wait(ctx)
	if transaction == nil && waitErr == nil {
		t.Fatal("I19: Wait returned neither a transaction nor an error")
	}
	if !instance.Status().IsTerminal() {
		t.Errorf("I19: status = %s, want a terminal state", instance.Status())
	}
}
