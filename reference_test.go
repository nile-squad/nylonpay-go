package nylonpay_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	nylonpay "github.com/nile-squad/nylonpay-go"
)

// referenceInput builds a valid collect input carrying the given reference, so
// each case below differs only in the reference it supplies.
func referenceInput(reference string) nylonpay.CollectPaymentInput {
	return nylonpay.CollectPaymentInput{
		Amount:      1000,
		Customer:    nylonpay.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "reference test",
		Reference:   reference,
	}
}

// assertReferenceAccepted checks that validation let the reference through. The
// call still reaches the network against an unreachable BaseURL, so a non-
// validation error is expected and fine; only a validation error is a failure.
func assertReferenceAccepted(t *testing.T, reference string) {
	t.Helper()
	_, err := testClient(t).CollectPayment(context.Background(), referenceInput(reference))
	if isValidationError(err) {
		t.Fatalf("reference %q should pass validation, got: %v", reference, err)
	}
}

func assertReferenceRejected(t *testing.T, reference string) {
	t.Helper()
	_, err := testClient(t).CollectPayment(context.Background(), referenceInput(reference))
	assertSDKError(t, err, "validation")
}

// ── Auto-generated references ─────────────────────────────────────────────────

func TestCollectPayment_AutoGeneratesReference(t *testing.T) {
	// An omitted reference must be filled in with a valid one, so validation
	// passes and only the (unreachable) network call fails.
	assertReferenceAccepted(t, "")
}

// ── Accepted references ───────────────────────────────────────────────────────

func TestCollectPayment_AcceptsUUID(t *testing.T) {
	assertReferenceAccepted(t, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
}

// Hex digits are case-insensitive, so an uppercase UUID names the same value.
func TestCollectPayment_AcceptsUppercaseUUID(t *testing.T) {
	assertReferenceAccepted(t, "7C9E6679-7425-40DE-944B-E07FC1F90AE7")
}

// Validation checks the shape, not the version: a merchant's v1 or v7 id is
// still a UUID the backend can store.
func TestCollectPayment_AcceptsNonV4UUID(t *testing.T) {
	assertReferenceAccepted(t, "550e8400-e29b-11d4-a716-446655440000")
}

// ── Rejected references ───────────────────────────────────────────────────────

func TestCollectPayment_RejectsNonUUIDReference(t *testing.T) {
	assertReferenceRejected(t, "ORDER-2026-001")
}

// The format the SDK itself used to generate. Guards against a regression that
// silently reintroduces references the backend rejects.
func TestCollectPayment_RejectsFifteenCharHexReference(t *testing.T) {
	assertReferenceRejected(t, "a1b2c3d4e5f6789")
}

// Whole-string matching: an end-anchor that also matched before a trailing
// newline would accept this in some languages and not others.
func TestCollectPayment_RejectsUUIDWithTrailingNewline(t *testing.T) {
	assertReferenceRejected(t, "7c9e6679-7425-40de-944b-e07fc1f90ae7\n")
}

func TestCollectPayment_RejectsUUIDWithoutHyphens(t *testing.T) {
	assertReferenceRejected(t, "7c9e6679742540de944be07fc1f90ae7")
}

func TestCollectPayment_RejectsUUIDWithSurroundingWhitespace(t *testing.T) {
	assertReferenceRejected(t, "  7c9e6679-7425-40de-944b-e07fc1f90ae7  ")
}

func TestCollectPayment_RejectsOverlongReference(t *testing.T) {
	assertReferenceRejected(t, strings.Repeat("a", 36))
}

// capturedAutoReference returns the reference the SDK generated for a call that
// supplied none. The beforeCollect hook sees the prepared input, which is the
// only place an auto-generated reference is observable from outside.
func capturedAutoReference(t *testing.T) string {
	t.Helper()

	var captured string
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:     "npk_testkey",
		APISecret:  "nps_testsecret",
		BaseURL:    "http://127.0.0.1:1",
		Force:      true,
		MaxRetries: -1,
		Hooks: &nylonpay.Hooks{
			BeforeCollect: &nylonpay.Hook[func(nylonpay.CollectPaymentInput) *nylonpay.CollectPaymentInput]{
				Fn: func(in nylonpay.CollectPaymentInput) *nylonpay.CollectPaymentInput {
					captured = in.Reference
					return nil
				},
				OnError: func(error) {},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _ = client.CollectPayment(context.Background(), referenceInput(""))
	if captured == "" {
		t.Fatal("beforeCollect never saw a generated reference")
	}
	return captured
}

// The generated reference must be a v4 UUID: version nibble 4, RFC 4122
// variant. A generator that drifted off that shape would still look plausible
// while producing values the backend refuses.
func TestGeneratedReferenceIsV4UUID(t *testing.T) {
	reference := capturedAutoReference(t)

	if !uuidPattern.MatchString(reference) {
		t.Fatalf("generated reference %q is not a UUID", reference)
	}
	if version := reference[14]; version != '4' {
		t.Errorf("generated reference %q has version %q, want 4", reference, version)
	}
	if variant := reference[19]; !strings.ContainsRune("89abAB", rune(variant)) {
		t.Errorf("generated reference %q has variant nibble %q, want one of 8/9/a/b",
			reference, variant)
	}
}

// Each call must produce a fresh reference: a repeat would replay the previous
// transaction instead of starting a new one.
func TestGeneratedReferencesAreUnique(t *testing.T) {
	first, second := capturedAutoReference(t), capturedAutoReference(t)
	if first == second {
		t.Errorf("two calls generated the same reference %q", first)
	}
}

// uuidPattern mirrors the SDK's own reference rule, written out independently
// so a broken pattern in the SDK cannot make these tests pass.
var uuidPattern = regexp.MustCompile(
	`\A[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\z`,
)

// isValidationError returns true if err is an SDKError with category "validation".
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "[validation]")
}
