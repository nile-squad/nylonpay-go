package nylonpay_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	nylonpay "github.com/nile-squad/nylonpay-go"
)

const webhookSecret = "wh_secret123"

func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func freshPayload() []byte {
	return fmt.Appendf(nil, `{"event":"success","reference":"ref001","timestamp":%d}`, time.Now().Unix())
}

func stalePayload() []byte {
	stale := time.Now().Add(-10 * time.Minute).Unix()
	return fmt.Appendf(nil, `{"event":"success","reference":"ref001","timestamp":%d}`, stale)
}

func intPtr(v int) *int { return &v }

// verify is the merchant-facing entry point: no client, no API credentials.
func verify(payload []byte, signature, secret string, tolerance *int) bool {
	return nylonpay.VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:          payload,
		Signature:        signature,
		Secret:           secret,
		ToleranceSeconds: tolerance,
	})
}

// ── Authenticity ──────────────────────────────────────────────────────────────

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	payload := freshPayload()
	if !verify(payload, signPayload(payload, webhookSecret), webhookSecret, nil) {
		t.Error("a correctly signed, fresh webhook must verify")
	}
}

func TestVerifyWebhookSignature_WrongSignature(t *testing.T) {
	payload := freshPayload()
	if verify(payload, strings.Repeat("a", 64), webhookSecret, nil) {
		t.Error("an incorrect signature must not verify")
	}
}

func TestVerifyWebhookSignature_WrongSecret(t *testing.T) {
	payload := freshPayload()
	if verify(payload, signPayload(payload, "different_secret"), webhookSecret, nil) {
		t.Error("a signature from a different secret must not verify")
	}
}

func TestVerifyWebhookSignature_TamperedPayload(t *testing.T) {
	payload := freshPayload()
	signature := signPayload(payload, webhookSecret)
	tampered := fmt.Appendf(nil, `{"event":"success","reference":"ref999","timestamp":%d}`, time.Now().Unix())

	if verify(tampered, signature, webhookSecret, nil) {
		t.Error("a tampered body must not verify against the original signature")
	}
}

// S16: one canonical form on the wire. Comparing decoded bytes alone would be
// case-blind and would accept this.
func TestVerifyWebhookSignature_UppercaseHexRejected(t *testing.T) {
	payload := freshPayload()
	signature := signPayload(payload, webhookSecret)

	if !verify(payload, signature, webhookSecret, nil) {
		t.Fatal("precondition: the lowercase signature must verify")
	}
	if verify(payload, strings.ToUpper(signature), webhookSecret, nil) {
		t.Error("the same digest re-spelled in uppercase hex must be rejected")
	}
}

// S8: never raises, on any input.
func TestVerifyWebhookSignature_NeverPanics(t *testing.T) {
	cases := []struct {
		name      string
		payload   []byte
		signature string
	}{
		{"empty payload", []byte{}, signPayload([]byte{}, webhookSecret)},
		{"nil payload", nil, ""},
		{"invalid utf8", []byte{0x7b, 0xff, 0x7d}, signPayload([]byte{0x7b, 0xff, 0x7d}, webhookSecret)},
		{"unparseable json", []byte("not json at all"), signPayload([]byte("not json at all"), webhookSecret)},
		{"missing timestamp", []byte(`{"event":"success"}`), signPayload([]byte(`{"event":"success"}`), webhookSecret)},
		{"malformed signature", freshPayload(), "zzzz"},
		{"odd-length signature", freshPayload(), "abc"},
		{"empty signature", freshPayload(), ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("verification panicked: %v", recovered)
				}
			}()
			// Each of these must fail closed rather than raise.
			if verify(tc.payload, tc.signature, webhookSecret, nil) {
				t.Error("expected verification to fail")
			}
		})
	}
}

// Invariant 29: the bytes are hashed exactly as supplied. Round-tripping them
// through a string would rewrite this payload and fail a genuine delivery.
func TestVerifyWebhookSignature_HashesRawBytesExactly(t *testing.T) {
	payload := []byte{0x7b, 0x22, 0x74, 0x22, 0x3a, 0xff, 0x7d} // {"t":<0xff>}
	if !verify(payload, signPayload(payload, webhookSecret), webhookSecret, intPtr(nylonpay.DisableFreshnessCheck)) {
		t.Error("a payload containing non-UTF-8 bytes must still verify")
	}
}

// ── Freshness (S14, S18) ──────────────────────────────────────────────────────

func TestVerifyWebhookSignature_StaleIsRejected(t *testing.T) {
	payload := stalePayload()
	if verify(payload, signPayload(payload, webhookSecret), webhookSecret, nil) {
		t.Error("a correctly signed but stale webhook must be rejected as a replay")
	}
}

func TestVerifyWebhookSignature_WidenedToleranceAcceptsStale(t *testing.T) {
	payload := stalePayload()
	if !verify(payload, signPayload(payload, webhookSecret), webhookSecret, intPtr(900)) {
		t.Error("a 900s window must accept a 10-minute-old webhook")
	}
}

// S18: 0 is the strictest setting, not an off switch.
func TestVerifyWebhookSignature_ZeroToleranceIsStrict(t *testing.T) {
	payload := stalePayload()
	if verify(payload, signPayload(payload, webhookSecret), webhookSecret, intPtr(0)) {
		t.Error("tolerance 0 must reject a stale webhook, not skip the check")
	}
}

func TestVerifyWebhookSignature_DisableSentinelAcceptsStale(t *testing.T) {
	payload := stalePayload()
	if !verify(payload, signPayload(payload, webhookSecret), webhookSecret, intPtr(nylonpay.DisableFreshnessCheck)) {
		t.Error("only the explicit sentinel opts out, and it must work")
	}
}

// A valid signature with nothing to date it cannot be told apart from a replay.
func TestVerifyWebhookSignature_MissingTimestampFailsClosed(t *testing.T) {
	payload := []byte(`{"event":"success","reference":"ref001"}`)
	if verify(payload, signPayload(payload, webhookSecret), webhookSecret, nil) {
		t.Error("a signed body carrying no timestamp must fail closed")
	}
}

// The timestamp is inside the signed body, so refreshing it breaks the HMAC.
func TestVerifyWebhookSignature_TimestampCannotBeRefreshed(t *testing.T) {
	captured := stalePayload()
	capturedSignature := signPayload(captured, webhookSecret)

	refreshed := freshPayload()
	if verify(refreshed, capturedSignature, webhookSecret, nil) {
		t.Error("swapping in a fresh timestamp under a captured signature must fail")
	}
}

func TestVerifyWebhookSignature_FutureTimestampBeyondToleranceRejected(t *testing.T) {
	future := time.Now().Add(10 * time.Minute).Unix()
	payload := fmt.Appendf(nil, `{"event":"success","timestamp":%d}`, future)

	if verify(payload, signPayload(payload, webhookSecret), webhookSecret, nil) {
		t.Error("a far-future timestamp must be rejected too")
	}
}

// ── Timestamp shapes ──────────────────────────────────────────────────────────

func TestVerifyWebhookSignature_AcceptedTimestampFormats(t *testing.T) {
	now := time.Now()
	cases := map[string][]byte{
		"epoch seconds":       fmt.Appendf(nil, `{"timestamp":%d}`, now.Unix()),
		"epoch milliseconds":  fmt.Appendf(nil, `{"timestamp":%d}`, now.UnixMilli()),
		"numeric string secs": fmt.Appendf(nil, `{"timestamp":"%d"}`, now.Unix()),
		"numeric string ms":   fmt.Appendf(nil, `{"timestamp":"%d"}`, now.UnixMilli()),
		"iso 8601 utc":        fmt.Appendf(nil, `{"timestamp":"%s"}`, now.UTC().Format(time.RFC3339Nano)),
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if !verify(payload, signPayload(payload, webhookSecret), webhookSecret, nil) {
				t.Errorf("%s should be an accepted timestamp shape", name)
			}
		})
	}
}

// ── The method form stays available for the Client interface ──────────────────

func TestVerifyWebhookSignature_MethodMatchesFunction(t *testing.T) {
	client, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
		Force:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := freshPayload()
	input := nylonpay.VerifyWebhookInput{
		Payload:   payload,
		Signature: signPayload(payload, webhookSecret),
		Secret:    webhookSecret,
	}
	if !client.VerifyWebhookSignature(input) {
		t.Error("the method form must behave identically to the function")
	}
}
