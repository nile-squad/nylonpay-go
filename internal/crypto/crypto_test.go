package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

// computeHMAC is the same operation VerifyResponseSignature performs,
// used in tests to construct valid signatures.
func computeHMAC(key, data []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func signData(data map[string]any, secret string) (string, error) {
	canonical, err := createCanonicalPayload(data)
	if err != nil {
		return "", err
	}
	return computeHMAC([]byte(secret), []byte(canonical)), nil
}

// ── Nonce ────────────────────────────────────────────────────────────────────

func TestGenerateNonce_Is32HexChars(t *testing.T) {
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nonce) != 32 {
		t.Errorf("nonce length = %d, want 32", len(nonce))
	}
	if _, err := hex.DecodeString(nonce); err != nil {
		t.Errorf("nonce %q is not valid hex: %v", nonce, err)
	}
}

func TestGenerateNonce_IsUnique(t *testing.T) {
	a, _ := GenerateNonce()
	b, _ := GenerateNonce()
	if a == b {
		t.Error("two consecutive nonces are identical — RNG may be broken")
	}
}

// ── Request signing ───────────────────────────────────────────────────────────

func baseSignatureInput() SignatureInput {
	return SignatureInput{
		Fingerprint: "aabbccdd",
		Nonce:       "nonce123",
		Timestamp:   "1700000000000",
		Payload:     map[string]any{"amount": 1000, "currency": "UGX"},
		Secret:      "nps_testsecret",
	}
}

func TestCreateSignature_Deterministic(t *testing.T) {
	in := baseSignatureInput()
	sig1, err1 := CreateSignature(in)
	sig2, err2 := CreateSignature(in)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if sig1 != sig2 {
		t.Error("identical inputs produced different signatures")
	}
}

func TestCreateSignature_ChangesWithSecret(t *testing.T) {
	in := baseSignatureInput()
	sig1, _ := CreateSignature(in)
	in.Secret = "nps_different"
	sig2, _ := CreateSignature(in)
	if sig1 == sig2 {
		t.Error("signatures must differ when secret changes")
	}
}

func TestCreateSignature_ChangesWithPayload(t *testing.T) {
	in := baseSignatureInput()
	sig1, _ := CreateSignature(in)
	in.Payload = map[string]any{"amount": 9999, "currency": "UGX"}
	sig2, _ := CreateSignature(in)
	if sig1 == sig2 {
		t.Error("signatures must differ when payload changes")
	}
}

func TestCreateSignature_ChangesWithNonce(t *testing.T) {
	in := baseSignatureInput()
	sig1, _ := CreateSignature(in)
	in.Nonce = "different_nonce"
	sig2, _ := CreateSignature(in)
	if sig1 == sig2 {
		t.Error("signatures must differ when nonce changes")
	}
}

func TestCreateSignature_ChangesWithTimestamp(t *testing.T) {
	in := baseSignatureInput()
	sig1, _ := CreateSignature(in)
	in.Timestamp = "9999999999999"
	sig2, _ := CreateSignature(in)
	if sig1 == sig2 {
		t.Error("signatures must differ when timestamp changes")
	}
}

// Object key order must NOT affect the signature (JCS / RFC 8785).
func TestCreateSignature_KeyOrderIndependence(t *testing.T) {
	in1 := baseSignatureInput()
	in1.Payload = map[string]any{"z": 1, "a": 2, "m": 3}

	in2 := baseSignatureInput()
	in2.Payload = map[string]any{"a": 2, "m": 3, "z": 1}

	sig1, _ := CreateSignature(in1)
	sig2, _ := CreateSignature(in2)
	if sig1 != sig2 {
		t.Error("key order must not affect the signature (JCS canonicalisation)")
	}
}

// Array element order MUST affect the signature.
func TestCreateSignature_ArrayOrderSignificance(t *testing.T) {
	in1 := baseSignatureInput()
	in1.Payload = map[string]any{"items": []int{1, 2, 3}}

	in2 := baseSignatureInput()
	in2.Payload = map[string]any{"items": []int{3, 2, 1}}

	sig1, _ := CreateSignature(in1)
	sig2, _ := CreateSignature(in2)
	if sig1 == sig2 {
		t.Error("array element order must affect the signature")
	}
}

// ── Response signature verification ──────────────────────────────────────────

func TestVerifyResponseSignature_Valid(t *testing.T) {
	secret := "nps_testsecret"
	data := map[string]any{"reference": "abc123", "status": "pending"}

	sig, err := signData(data, secret)
	if err != nil {
		t.Fatalf("signData: %v", err)
	}

	ok, err := VerifyResponseSignature(data, sig, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected valid signature to pass verification")
	}
}

func TestVerifyResponseSignature_WrongSecret(t *testing.T) {
	data := map[string]any{"reference": "abc123"}
	sig, _ := signData(data, "correct_secret")

	ok, _ := VerifyResponseSignature(data, sig, "wrong_secret")
	if ok {
		t.Error("verification must fail with wrong secret")
	}
}

func TestVerifyResponseSignature_EmptySignature(t *testing.T) {
	data := map[string]any{"reference": "abc123"}
	ok, _ := VerifyResponseSignature(data, "", "secret")
	if ok {
		t.Error("empty signature must not verify")
	}
}

func TestVerifyResponseSignature_NonHexSignature(t *testing.T) {
	data := map[string]any{"reference": "abc123"}
	ok, _ := VerifyResponseSignature(data, "not-valid-hex!!!", "secret")
	if ok {
		t.Error("non-hex signature must not verify")
	}
}

func TestVerifyResponseSignature_TruncatedSignature(t *testing.T) {
	secret := "nps_testsecret"
	data := map[string]any{"reference": "abc123"}
	full, _ := signData(data, secret)
	truncated := full[:len(full)-4]

	ok, _ := VerifyResponseSignature(data, truncated, secret)
	if ok {
		t.Error("truncated signature must not verify")
	}
}

func TestVerifyResponseSignature_ModifiedData(t *testing.T) {
	secret := "nps_testsecret"
	original := map[string]any{"amount": int64(1000)}
	sig, _ := signData(original, secret)

	tampered := map[string]any{"amount": int64(9999)}
	ok, _ := VerifyResponseSignature(tampered, sig, secret)
	if ok {
		t.Error("signature must not verify against tampered data")
	}
}

// ── Webhook signature verification ───────────────────────────────────────────

func freshWebhookPayload() []byte {
	ts := time.Now().Unix()
	return fmt.Appendf(nil, `{"event":"success","reference":"ref001","timestamp":%d}`, ts)
}

func signWebhook(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	secret := "wh_secret"
	payload := freshWebhookPayload()
	sig := signWebhook(payload, secret)

	ok := VerifyWebhookSignature(VerifyWebhookInput{
		Payload:   payload,
		Signature: sig,
		Secret:    secret,
	})
	if !ok {
		t.Error("valid webhook must verify")
	}
}

func TestVerifyWebhookSignature_InvalidSignature(t *testing.T) {
	secret := "wh_secret"
	payload := freshWebhookPayload()

	ok := VerifyWebhookSignature(VerifyWebhookInput{
		Payload:   payload,
		Signature: "aabbccdd00112233",
		Secret:    secret,
	})
	if ok {
		t.Error("invalid signature must not verify")
	}
}

func TestVerifyWebhookSignature_WrongSecret(t *testing.T) {
	payload := freshWebhookPayload()
	sig := signWebhook(payload, "correct_secret")

	ok := VerifyWebhookSignature(VerifyWebhookInput{
		Payload:   payload,
		Signature: sig,
		Secret:    "wrong_secret",
	})
	if ok {
		t.Error("wrong secret must not verify")
	}
}

func TestVerifyWebhookSignature_ExpiredTimestamp(t *testing.T) {
	secret := "wh_secret"
	past := time.Now().Add(-10 * time.Minute).Unix()
	payload := fmt.Appendf(nil, `{"event":"success","timestamp":%d}`, past)
	sig := signWebhook(payload, secret)

	ok := VerifyWebhookSignature(VerifyWebhookInput{
		Payload:   payload,
		Signature: sig,
		Secret:    secret,
	})
	if ok {
		t.Error("expired webhook (10 min old) must not verify with default 300s tolerance")
	}
}

func TestVerifyWebhookSignature_ZeroTolerance_SkipsTimestampCheck(t *testing.T) {
	secret := "wh_secret"
	// Very old timestamp — but tolerance = 0 disables freshness check.
	old := int64(1000000000)
	payload := fmt.Appendf(nil, `{"event":"success","timestamp":%d}`, old)
	sig := signWebhook(payload, secret)
	zero := 0

	ok := VerifyWebhookSignature(VerifyWebhookInput{
		Payload:          payload,
		Signature:        sig,
		Secret:           secret,
		ToleranceSeconds: &zero,
	})
	if !ok {
		t.Error("zero tolerance should skip the timestamp check")
	}
}

func TestVerifyWebhookSignature_FutureTimestamp_WithinTolerance(t *testing.T) {
	secret := "wh_secret"
	// 1 minute in the future — still within 300s tolerance.
	future := time.Now().Add(1 * time.Minute).Unix()
	payload := fmt.Appendf(nil, `{"event":"success","timestamp":%d}`, future)
	sig := signWebhook(payload, secret)

	ok := VerifyWebhookSignature(VerifyWebhookInput{
		Payload:   payload,
		Signature: sig,
		Secret:    secret,
	})
	if !ok {
		t.Error("slightly-future timestamp within tolerance should verify")
	}
}
