package core

import (
	"encoding/json"
	"errors"
	"testing"
)

// ── ParseError ────────────────────────────────────────────────────────────────

func TestParseError_JSONFormat(t *testing.T) {
	msg := `{"category":"auth","message":"invalid api key"}`
	err := ParseError(msg)
	if err.Category != "auth" {
		t.Errorf("category = %q, want %q", err.Category, "auth")
	}
	if err.Message != "invalid api key" {
		t.Errorf("message = %q, want %q", err.Message, "invalid api key")
	}
}

func TestParseError_SuffixFormat(t *testing.T) {
	msg := "phone number not registered -- error-type: not_found"
	err := ParseError(msg)
	if err.Category != "not_found" {
		t.Errorf("category = %q, want %q", err.Category, "not_found")
	}
	if err.Message != "phone number not registered" {
		t.Errorf("message = %q, want %q", err.Message, "phone number not registered")
	}
}

func TestParseError_UnknownSuffix_FallsBackToInternal(t *testing.T) {
	msg := "something bad -- error-type: unknown_category"
	err := ParseError(msg)
	if err.Category != "internal" {
		t.Errorf("unknown suffix category should fall back to internal, got %q", err.Category)
	}
}

func TestParseError_PlainString_FallsBackToInternal(t *testing.T) {
	msg := "something went wrong"
	err := ParseError(msg)
	if err.Category != "internal" {
		t.Errorf("category = %q, want %q", err.Category, "internal")
	}
	if err.Message != msg {
		t.Errorf("message = %q, want %q", err.Message, msg)
	}
}

func TestParseError_AllKnownCategories(t *testing.T) {
	for cat := range KnownCategories {
		t.Run(cat, func(t *testing.T) {
			msg := "some message -- error-type: " + cat
			err := ParseError(msg)
			if err.Category != cat {
				t.Errorf("category = %q, want %q", err.Category, cat)
			}
		})
	}
}

func TestParseError_JSONMissingCategory_FallsBack(t *testing.T) {
	// JSON with only message, no category — must not match JSON path.
	msg := `{"message":"some error"}`
	err := ParseError(msg)
	if err.Category != "internal" {
		t.Errorf("JSON without category should fall back to internal, got %q", err.Category)
	}
}

// ── SDKError ──────────────────────────────────────────────────────────────────

func TestSDKError_ErrorString(t *testing.T) {
	err := &SDKError{Category: "validation", Message: "amount is required"}
	want := "[validation] amount is required"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestSDKError_ImplementsError(t *testing.T) {
	var err error = &SDKError{Category: "auth", Message: "bad key"}
	var sdkErr *SDKError
	if !errors.As(err, &sdkErr) {
		t.Error("*SDKError must satisfy the error interface via errors.As")
	}
}

// ── buildHttpError ────────────────────────────────────────────────────────────

func TestBuildHttpError_ParsesBodyMessage(t *testing.T) {
	body := []byte(`{"message":"account is suspended"}`)
	err := buildHttpError(body, 400)
	if err.Message != "account is suspended" {
		t.Errorf("message = %q, want %q", err.Message, "account is suspended")
	}
}

func TestBuildHttpError_FallsBackToHTTPCode_OnEmptyBody(t *testing.T) {
	err := buildHttpError([]byte(`{}`), 400)
	if err.Message == "" {
		t.Error("message must not be empty when body has no message field")
	}
}

func TestBuildHttpError_4xxIsValidation(t *testing.T) {
	err := buildHttpError([]byte(`{"message":"bad request"}`), 422)
	if err.Category != "validation" {
		t.Errorf("category = %q, want validation", err.Category)
	}
	if err.Retryable {
		t.Error("4xx errors must not be retryable")
	}
}

func TestBuildHttpError_5xxIsInternal(t *testing.T) {
	err := buildHttpError([]byte(`{}`), 500)
	if err.Category != "internal" {
		t.Errorf("category = %q, want internal", err.Category)
	}
	if !err.Retryable {
		t.Error("5xx errors must be retryable")
	}
}

func TestBuildHttpError_408IsTimeout(t *testing.T) {
	err := buildHttpError([]byte(`{}`), 408)
	if err.Category != "timeout" {
		t.Errorf("category = %q, want timeout", err.Category)
	}
	if !err.Retryable {
		t.Error("408 must be retryable")
	}
}

func TestBuildHttpError_429IsRateLimit(t *testing.T) {
	err := buildHttpError([]byte(`{}`), 429)
	if err.Category != "rate_limit" {
		t.Errorf("category = %q, want rate_limit", err.Category)
	}
	if !err.Retryable {
		t.Error("429 must be retryable")
	}
}

// ── structToMap ───────────────────────────────────────────────────────────────

func TestStructToMap_PreservesFields(t *testing.T) {
	type payload struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	m, err := structToMap(payload{Amount: 5000, Currency: "UGX"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	num, ok := m["amount"].(json.Number)
	if !ok {
		t.Fatalf("amount field type = %T, want json.Number", m["amount"])
	}
	if num.String() != "5000" {
		t.Errorf("amount = %q, want %q", num.String(), "5000")
	}
	if m["currency"] != "UGX" {
		t.Errorf("currency = %v, want UGX", m["currency"])
	}
}

func TestStructToMap_AllowsFingerprintInjection(t *testing.T) {
	m, err := structToMap(map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m["_fingerprint"] = "fp123"
	if m["_fingerprint"] != "fp123" {
		t.Error("fingerprint injection failed")
	}
}

func TestStructToMap_RejectsUnmarshalable(t *testing.T) {
	// Channels cannot be JSON-marshalled.
	_, err := structToMap(make(chan int))
	if err == nil {
		t.Error("expected error for unmarshallable type")
	}
}

// ── stripResponseSignature ────────────────────────────────────────────────────

func TestStripResponseSignature_RemovesField(t *testing.T) {
	raw := json.RawMessage(`{"status":"pending","_responseSignature":"abc123"}`)
	m, sig, err := stripResponseSignature(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != "abc123" {
		t.Errorf("sig = %q, want %q", sig, "abc123")
	}
	// The stripped map must not contain the signature field.
	asMap, ok := m.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", m)
	}
	if _, exists := asMap["_responseSignature"]; exists {
		t.Error("_responseSignature must be removed from the stripped map")
	}
	if asMap["status"] != "pending" {
		t.Errorf("status = %v, want pending", asMap["status"])
	}
}

func TestStripResponseSignature_NoSignatureField(t *testing.T) {
	raw := json.RawMessage(`{"status":"pending"}`)
	_, sig, err := stripResponseSignature(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != "" {
		t.Errorf("sig = %q, want empty string", sig)
	}
}

func TestStripResponseSignature_EmptyRaw(t *testing.T) {
	m, sig, err := stripResponseSignature(json.RawMessage{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != "" || m != nil {
		t.Error("empty raw message must return nil map and empty sig")
	}
}

func TestStripResponseSignature_NullSignatureField(t *testing.T) {
	raw := json.RawMessage(`{"_responseSignature":null,"data":"x"}`)
	_, sig, err := stripResponseSignature(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != "" {
		t.Errorf("null _responseSignature must yield empty sig, got %q", sig)
	}
}
