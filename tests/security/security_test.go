// Package security_test is the canonical Security Test suite (S1-S21) every
// Nylon Pay SDK ships. The IDs are traceable from the spec's Implementation
// Requirements to the test names below.
//
// Everything here runs against mocked transport. No network, no credentials.
package security_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	nylonpay "github.com/nile-squad/nylonpay-go"
	"github.com/nile-squad/nylonpay-go/internal/crypto"
	"github.com/nile-squad/nylonpay-go/types"
)

const (
	testSecret    = "nps_test_secret"
	testKey       = "npk_test_key"
	webhookSecret = "wh_test_secret"
)

func signaturePayload(payload any, secret string) string {
	signature, err := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: strings.Repeat("a", 64),
		Nonce:       "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
		Timestamp:   "1718976000000",
		Payload:     payload,
		Secret:      secret,
	})
	if err != nil {
		panic(err)
	}
	return signature
}

// canonicalizeASCII mirrors what the backend does when signing a response.
//
// Go's json.Marshal sorts map keys by UTF-8 byte, which is identical to the
// spec's UTF-16 code-unit order for the ASCII keys used here. The general case
// is covered by the conformance vectors, not by this helper.
func canonicalizeASCII(t *testing.T, data map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return string(encoded)
}

func hmacHex(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// signedBackendResponse builds the envelope a healthy backend returns: data
// carrying the echoed request nonce, signed, wrapped in the status envelope.
func signedBackendResponse(t *testing.T, data map[string]any, requestNonce, secret string) []byte {
	t.Helper()
	data["_requestNonce"] = requestNonce
	data["_responseSignature"] = hmacHex(canonicalizeASCII(t, data), secret)

	body, err := json.Marshal(map[string]any{"status": true, "message": "ok", "data": data})
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	return body
}

func clientFor(t *testing.T, server *httptest.Server) *nylonpay.NylonPayClient {
	t.Helper()
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:     testKey,
		APISecret:  testSecret,
		BaseURL:    server.URL,
		Force:      true,
		MaxRetries: -1,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func getStatus(t *testing.T, server *httptest.Server) (*types.StatusResponse, error) {
	t.Helper()
	return clientFor(t, server).GetStatus(t.Context(), types.GetStatusInput{Reference: "ref_0123456789"})
}

// ── S1: request signature integrity ───────────────────────────────────────────

func TestS1_SignatureIsDeterministicAndSensitive(t *testing.T) {
	payload := map[string]any{"amount": 5000, "reference": "ORDER-2026-001"}
	base := signaturePayload(payload, testSecret)

	if base != signaturePayload(payload, testSecret) {
		t.Error("identical inputs must produce identical signatures")
	}
	if !regexpMatch(`^[a-f0-9]{64}$`, base) {
		t.Errorf("signature %q is not a 64-char lowercase hex digest", base)
	}

	if signaturePayload(map[string]any{"amount": 5001, "reference": "ORDER-2026-001"}, testSecret) == base {
		t.Error("changing the payload must change the signature")
	}
	if signaturePayload(payload, "nps_other_secret") == base {
		t.Error("changing the secret must change the signature")
	}

	withOtherNonce, _ := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: strings.Repeat("a", 64),
		Nonce:       "ffffffffffffffffffffffffffffffff",
		Timestamp:   "1718976000000",
		Payload:     payload,
		Secret:      testSecret,
	})
	if withOtherNonce == base {
		t.Error("changing the nonce must change the signature")
	}

	withOtherTimestamp, _ := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: strings.Repeat("a", 64),
		Nonce:       "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
		Timestamp:   "1718976000001",
		Payload:     payload,
		Secret:      testSecret,
	})
	if withOtherTimestamp == base {
		t.Error("changing the timestamp must change the signature")
	}
}

// ── S2: canonical payload ordering ────────────────────────────────────────────

func TestS2_KeyOrderIrrelevantArrayOrderSignificant(t *testing.T) {
	// Two maps differing only in insertion order. Go maps are unordered, so
	// this is asserted through nested structures whose serialization would
	// differ if ordering were not normalized.
	first := map[string]any{
		"b": map[string]any{"y": 2, "x": 1},
		"a": 1,
	}
	second := map[string]any{
		"a": 1,
		"b": map[string]any{"x": 1, "y": 2},
	}
	if signaturePayload(first, testSecret) != signaturePayload(second, testSecret) {
		t.Error("object key order, at any depth, must not affect the signature")
	}

	forward := map[string]any{"items": []any{"a", "b", "c"}}
	reversed := map[string]any{"items": []any{"c", "b", "a"}}
	if signaturePayload(forward, testSecret) == signaturePayload(reversed, testSecret) {
		t.Error("array order is significant and must affect the signature")
	}
}

// ── S3: nonce quality ─────────────────────────────────────────────────────────

func TestS3_NoncesAreRandomAndUnique(t *testing.T) {
	const iterations = 10000
	seen := make(map[string]struct{}, iterations)

	for i := 0; i < iterations; i++ {
		nonce, err := crypto.GenerateNonce()
		if err != nil {
			t.Fatalf("nonce generation failed: %v", err)
		}
		if !regexpMatch(`^[a-f0-9]{32}$`, nonce) {
			t.Fatalf("nonce %q is not 32 lowercase hex characters", nonce)
		}
		if _, duplicate := seen[nonce]; duplicate {
			t.Fatalf("nonce collision after %d generations", i)
		}
		seen[nonce] = struct{}{}
	}
}

// ── S4: fingerprint stability ─────────────────────────────────────────────────

func TestS4_FingerprintIsStableHex(t *testing.T) {
	first := crypto.GenerateFingerprint()
	if !regexpMatch(`^[a-f0-9]{64}$`, first) {
		t.Errorf("fingerprint %q is not a 64-char lowercase hex value", first)
	}
	if first != crypto.GenerateFingerprint() {
		t.Error("the fingerprint must be stable within a process")
	}
}

// ── S5-S7: response signature verification ────────────────────────────────────

func TestS5_ResponseVerificationAcceptsValidSignature(t *testing.T) {
	data := map[string]any{"reference": "ref_0123456789", "status": "pending"}
	valid, err := crypto.VerifyResponseSignature(data, hmacHex(canonicalizeASCII(t, data), testSecret), testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Error("a correctly signed response must verify")
	}
}

func TestS6_ResponseVerificationRejectsTamperingAndWrongSecret(t *testing.T) {
	data := map[string]any{"reference": "ref_0123456789", "status": "pending"}
	signature := hmacHex(canonicalizeASCII(t, data), testSecret)

	tampered := map[string]any{"reference": "ref_0123456789", "status": "successful"}
	if valid, _ := crypto.VerifyResponseSignature(tampered, signature, testSecret); valid {
		t.Error("a tampered payload must not verify")
	}

	fromOtherSecret := hmacHex(canonicalizeASCII(t, data), "nps_other_secret")
	if valid, _ := crypto.VerifyResponseSignature(data, fromOtherSecret, testSecret); valid {
		t.Error("a signature produced with a different secret must not verify")
	}
}

func TestS7_ResponseVerificationRejectsMalformedWithoutPanicking(t *testing.T) {
	data := map[string]any{"reference": "ref_0123456789"}
	valid := hmacHex(canonicalizeASCII(t, data), testSecret)

	// One flipped hex character.
	flipped := []byte(valid)
	if flipped[0] == 'a' {
		flipped[0] = 'b'
	} else {
		flipped[0] = 'a'
	}

	for name, signature := range map[string]string{
		"empty":        "",
		"short":        "abc",
		"non-hex":      strings.Repeat("z", 64),
		"odd length":   "abc123",
		"too long":     strings.Repeat("a", 128),
		"byte-flipped": string(flipped),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("verification panicked on %s signature: %v", name, recovered)
				}
			}()
			if ok, _ := crypto.VerifyResponseSignature(data, signature, testSecret); ok {
				t.Errorf("%s signature must not verify", name)
			}
		})
	}
}

// ── S9: constant-time comparison ──────────────────────────────────────────────

// The comparison primitive is exercised throughout S5-S8 and S16. This test
// pins the property those depend on: verification is length-guarded, so a
// wrong-length digest is rejected rather than compared, and never raises.
func TestS9_ComparisonIsLengthGuarded(t *testing.T) {
	data := map[string]any{"reference": "ref_0123456789"}
	for _, length := range []int{0, 1, 31, 63, 65, 127} {
		signature := strings.Repeat("a", length)
		if ok, _ := crypto.VerifyResponseSignature(data, signature, testSecret); ok {
			t.Errorf("a %d-character signature must not verify", length)
		}
	}
}

// ── S10, S11: transport is fail-closed on response signatures ─────────────────

func TestS10_TransportRejectsMissingResponseSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A signed payload with the signature field stripped, exactly what a
		// man-in-the-middle would produce.
		data := map[string]any{
			"reference":     "ref_0123456789",
			"status":        "successful",
			"_requestNonce": r.Header.Get("X-Nylon-Nonce"),
		}
		body, _ := json.Marshal(map[string]any{"status": true, "message": "ok", "data": data})
		w.Write(body)
	}))
	defer server.Close()

	status, err := getStatus(t, server)
	if status != nil {
		t.Error("unverified data must never be returned")
	}
	assertCategory(t, err, types.CategoryInternal)
}

func TestS11_TransportRejectsInvalidSignatureAcceptsValid(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data := map[string]any{
				"reference":          "ref_0123456789",
				"status":             "successful",
				"_requestNonce":      r.Header.Get("X-Nylon-Nonce"),
				"_responseSignature": strings.Repeat("a", 64),
			}
			body, _ := json.Marshal(map[string]any{"status": true, "message": "ok", "data": data})
			w.Write(body)
		}))
		defer server.Close()

		status, err := getStatus(t, server)
		if status != nil {
			t.Error("data behind an invalid signature must never be returned")
		}
		assertCategory(t, err, types.CategoryInternal)
	})

	t.Run("valid", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(signedBackendResponse(t, map[string]any{
				"reference": "ref_0123456789",
				"status":    "successful",
			}, r.Header.Get("X-Nylon-Nonce"), testSecret))
		}))
		defer server.Close()

		status, err := getStatus(t, server)
		if err != nil {
			t.Fatalf("a correctly signed response must be accepted, got: %v", err)
		}
		if status.Reference != "ref_0123456789" {
			t.Errorf("reference = %q, want ref_0123456789", status.Reference)
		}
	})
}

// ── S12: credential prefixes ──────────────────────────────────────────────────

func TestS12_ConstructionRejectsMalformedCredentials(t *testing.T) {
	cases := map[string]nylonpay.Config{
		"missing key":       {APISecret: "nps_secret"},
		"bad key prefix":    {APIKey: "pk_live_wrong", APISecret: "nps_secret"},
		"missing secret":    {APIKey: "npk_key"},
		"bad secret prefix": {APIKey: "npk_key", APISecret: "sk_wrong"},
	}

	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			cfg.Force = true
			client, err := nylonpay.NewClient(cfg)
			if err == nil {
				t.Fatal("expected construction to fail")
			}
			if client != nil {
				t.Error("no client may be constructed from invalid credentials")
			}
			assertCategory(t, err, types.CategoryValidation)
		})
	}
}

// ── S13: the secret stays off the public surface ──────────────────────────────

func TestS13_SecretIsNotExposedAndCacheIsSecretAware(t *testing.T) {
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey: testKey, APISecret: testSecret, Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Nothing reachable through the exported surface may carry the secret.
	if serialized, err := json.Marshal(client); err == nil && strings.Contains(string(serialized), testSecret) {
		t.Error("the API secret must not appear in the serialized client")
	}
	if value := reflect.ValueOf(client).Elem(); value.NumField() > 0 {
		if strings.Contains(fmt.Sprintf("%+v", client), testSecret) {
			t.Log("secret appears in the unexported struct state, which is expected; it must simply never be exported")
		}
	}

	// Rotating the secret must not hand back a client still signing with the old one.
	first, _ := nylonpay.NewClient(nylonpay.Config{APIKey: testKey, APISecret: testSecret})
	same, _ := nylonpay.NewClient(nylonpay.Config{APIKey: testKey, APISecret: testSecret})
	rotated, _ := nylonpay.NewClient(nylonpay.Config{APIKey: testKey, APISecret: "nps_rotated_secret"})

	if first != same {
		t.Error("the same credentials must return the cached client")
	}
	if first == rotated {
		t.Error("rotating the secret must yield a different client")
	}
}

// ── S15: response replay binding ──────────────────────────────────────────────

func TestS15_ResponseReplayIsRejected(t *testing.T) {
	t.Run("nonce mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Correctly signed, but answering a different request.
			w.Write(signedBackendResponse(t, map[string]any{
				"reference": "ref_0123456789",
				"status":    "successful",
			}, "00000000000000000000000000000000", testSecret))
		}))
		defer server.Close()

		status, err := getStatus(t, server)
		if status != nil {
			t.Error("a response bound to another request must not be returned")
		}
		assertCategory(t, err, types.CategoryInternal)
	})

	t.Run("nonce omitted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			data := map[string]any{"reference": "ref_0123456789", "status": "successful"}
			data["_responseSignature"] = hmacHex(canonicalizeASCII(t, data), testSecret)
			body, _ := json.Marshal(map[string]any{"status": true, "message": "ok", "data": data})
			w.Write(body)
		}))
		defer server.Close()

		_, err := getStatus(t, server)
		assertCategory(t, err, types.CategoryInternal)
	})

	// A response captured from a real earlier call must not satisfy a later one.
	t.Run("captured response replayed", func(t *testing.T) {
		var captured []byte
		requests := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if requests == 1 {
				captured = signedBackendResponse(t, map[string]any{
					"reference": "ref_0123456789",
					"status":    "successful",
				}, r.Header.Get("X-Nylon-Nonce"), testSecret)
				w.Write(captured)
				return
			}
			w.Write(captured) // replayed verbatim
		}))
		defer server.Close()

		client := clientFor(t, server)
		if _, err := client.GetStatus(t.Context(), types.GetStatusInput{Reference: "ref_0123456789"}); err != nil {
			t.Fatalf("the first, genuine response must be accepted: %v", err)
		}
		if _, err := client.GetStatus(t.Context(), types.GetStatusInput{Reference: "ref_0123456789"}); err == nil {
			t.Error("replaying a captured response onto a later request must be rejected")
		}
	})
}

// ── S17: response size cap, enforced during the read ──────────────────────────

func TestS17_OversizedResponseIsRejected(t *testing.T) {
	const cap = 4096
	oversized := strings.Repeat("x", cap*4)

	t.Run("declared content-length", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(len(oversized)))
			w.Write([]byte(oversized))
		}))
		defer server.Close()

		_, err := capClient(t, server, cap).GetStatus(t.Context(), types.GetStatusInput{Reference: "ref_0123456789"})
		assertCategory(t, err, types.CategoryInternal)
	})

	// The case a Content-Length check cannot catch: chunked, no length header.
	t.Run("chunked with no length", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Skip("test server does not support streaming")
			}
			for i := 0; i < 8; i++ {
				w.Write([]byte(strings.Repeat("y", cap)))
				flusher.Flush()
			}
		}))
		defer server.Close()

		_, err := capClient(t, server, cap).GetStatus(t.Context(), types.GetStatusInput{Reference: "ref_0123456789"})
		assertCategory(t, err, types.CategoryInternal)
	})

	// A body at or under the cap still works, so the guard is not simply
	// rejecting everything.
	t.Run("within the cap succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(signedBackendResponse(t, map[string]any{
				"reference": "ref_0123456789",
				"status":    "pending",
			}, r.Header.Get("X-Nylon-Nonce"), testSecret))
		}))
		defer server.Close()

		if _, err := capClient(t, server, cap).GetStatus(t.Context(), types.GetStatusInput{Reference: "ref_0123456789"}); err != nil {
			t.Fatalf("a small response must still be accepted: %v", err)
		}
	})
}

func capClient(t *testing.T, server *httptest.Server, maxBytes int64) *nylonpay.NylonPayClient {
	t.Helper()
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:           testKey,
		APISecret:        testSecret,
		BaseURL:          server.URL,
		Force:            true,
		MaxRetries:       -1,
		MaxResponseBytes: maxBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// ── S20: non-integer amounts cannot reach the wire ────────────────────────────

// Amounts, quantities and unit prices are int64 throughout the public API, so a
// non-integer value is unrepresentable rather than merely rejected: the spec's
// integer-only wire rule holds at compile time. What remains testable is that
// invalid amounts fail before any signing or network call.
func TestS20_InvalidAmountsFailBeforeAnyRequest(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	client := clientFor(t, server)

	_, err := client.CollectPayment(t.Context(), types.CollectPaymentInput{
		Amount:      0,
		Customer:    types.Customer{Name: "Jane", PhoneNumber: "0771234567"},
		Description: "test",
	})
	assertCategory(t, err, types.CategoryValidation)

	_, err = client.CreateInvoice(t.Context(), types.CreateInvoiceInput{
		Amount:        5000,
		CustomerEmail: "jane@example.com",
		Items:         []types.InvoiceItem{{Name: "item", Quantity: 0, UnitPrice: 100}},
	})
	assertCategory(t, err, types.CategoryValidation)

	if requests != 0 {
		t.Errorf("validation must fail before any request; server saw %d", requests)
	}
}

// ── S21: what is signed, and what the body carries ────────────────────────────

func TestS21_SignedFingerprintMatchesBodyAndCoversInnerPayloadOnly(t *testing.T) {
	type captured struct {
		envelope  map[string]any
		signature string
		nonce     string
		timestamp string
	}
	var got captured

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		got = captured{
			envelope:  envelope,
			signature: r.Header.Get("X-Nylon-Signature"),
			nonce:     r.Header.Get("X-Nylon-Nonce"),
			timestamp: r.Header.Get("X-Nylon-Timestamp"),
		}
		w.Write(signedBackendResponse(t, map[string]any{
			"reference": "ref_0123456789",
			"status":    "pending",
		}, r.Header.Get("X-Nylon-Nonce"), testSecret))
	}))
	defer server.Close()

	if _, err := getStatus(t, server); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload, ok := got.envelope["payload"].(map[string]any)
	if !ok {
		t.Fatal("request envelope has no payload object")
	}
	bodyFingerprint, ok := payload["_fingerprint"].(string)
	if !ok {
		t.Fatal("_fingerprint is missing from the request body")
	}
	if !regexpMatch(`^[a-f0-9]{64}$`, bodyFingerprint) {
		t.Errorf("_fingerprint %q is not 64 lowercase hex characters", bodyFingerprint)
	}

	// The signed fingerprint must be byte-identical to the one in the body: the
	// server reads it from the body and feeds that value into its own input.
	expected, err := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: bodyFingerprint,
		Nonce:       got.nonce,
		Timestamp:   got.timestamp,
		Payload:     payload,
		Secret:      testSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.signature != expected {
		t.Errorf("signature does not match one computed over the inner payload with the body's fingerprint\n  sent: %s\n  want: %s", got.signature, expected)
	}

	// Signing the whole envelope is the classic first-implementation mistake;
	// it must not be what happened here.
	overEnvelope, err := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: bodyFingerprint,
		Nonce:       got.nonce,
		Timestamp:   got.timestamp,
		Payload:     got.envelope,
		Secret:      testSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.signature == overEnvelope {
		t.Error("the signature must cover the inner payload only, not the full envelope")
	}
}

// ── S8, S14, S16, S18, S19 ────────────────────────────────────────────────────

// S8 (webhook verification never raises), S14 (webhook replay protection),
// S16 (canonical lowercase hex only) and S18 (0 is strict, not disabled) are
// covered against the public entry point in webhook_test.go at the repository
// root. S19 (conformance vectors V1-V7) lives in internal/crypto alongside the
// canonicalizer it pins. This test asserts the shared primitives those rely on
// are reachable and behave, so the suite fails loudly if they are ever moved.
func TestS8_S14_S16_S18_WebhookSurfaceIsPresent(t *testing.T) {
	payload := fmt.Appendf(nil, `{"event":"transaction.successful","timestamp":%d}`, time.Now().Unix())
	signature := hmacHex(string(payload), webhookSecret)

	if !nylonpay.VerifyWebhookSignature(types.VerifyWebhookInput{
		Payload: payload, Signature: signature, Secret: webhookSecret,
	}) {
		t.Fatal("a fresh, correctly signed webhook must verify")
	}
	if nylonpay.VerifyWebhookSignature(types.VerifyWebhookInput{
		Payload: payload, Signature: strings.ToUpper(signature), Secret: webhookSecret,
	}) {
		t.Error("S16: uppercase hex must be rejected")
	}
	if nylonpay.DisableFreshnessCheck != -1 {
		t.Errorf("S18: the disable sentinel must be -1, got %d", nylonpay.DisableFreshnessCheck)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertCategory(t *testing.T, err error, want types.ErrorCategory) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with category %s, got nil", want)
	}
	sdkErr, ok := err.(*types.SDKError)
	if !ok {
		t.Fatalf("error %v is %T, want *types.SDKError", err, err)
	}
	if sdkErr.Category != want {
		t.Errorf("category = %s, want %s (message: %s)", sdkErr.Category, want, sdkErr.Message)
	}
}

func regexpMatch(pattern, value string) bool {
	return regexp.MustCompile(pattern).MatchString(value)
}
