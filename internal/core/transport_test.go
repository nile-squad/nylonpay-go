package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signResponseData computes the _responseSignature the mock server must embed.
func signResponseData(data map[string]any, secret string) string {
	b, _ := json.Marshal(data)
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	dec.Decode(&m)
	canonical, _ := json.Marshal(m)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil))
}

// successBody wraps data in a signed BackendResponse envelope.
func successBody(data map[string]any, secret string) []byte {
	sig := signResponseData(data, secret)
	data["_responseSignature"] = sig
	dataBytes, _ := json.Marshal(data)
	body, _ := json.Marshal(map[string]any{
		"status":  true,
		"message": "ok",
		"data":    json.RawMessage(dataBytes),
	})
	return body
}

// errorBody returns a failed BackendResponse with the given message.
func errorBody(msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"status":  false,
		"message": msg,
		"data":    nil,
	})
	return b
}

func testTransport(server *httptest.Server, secret string) *Transport {
	return NewTransport(TransportConfig{
		APIKey:     "npk_test",
		APISecret:  secret,
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})
}

// ── Happy path ────────────────────────────────────────────────────────────────

func TestSend_HappyPath(t *testing.T) {
	const secret = "nps_secret"
	want := map[string]any{"reference": "ref123", "status": "pending"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(successBody(map[string]any{"reference": "ref123", "status": "pending"}, secret))
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "get_status", Payload: map[string]string{"reference": "ref123"}},
		&out,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["reference"] != want["reference"] {
		t.Errorf("reference = %v, want %v", out["reference"], want["reference"])
	}
}

// ── Request headers ───────────────────────────────────────────────────────────

func TestSend_SetsRequiredHeaders(t *testing.T) {
	const secret = "nps_secret"
	var gotHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Write(successBody(map[string]any{}, secret))
	}))
	defer srv.Close()

	var out map[string]any
	testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "test", Payload: map[string]string{}},
		&out,
	)

	for _, h := range []string{"X-Nylon-Key", "X-Nylon-Nonce", "X-Nylon-Timestamp", "X-Nylon-Signature"} {
		if gotHeaders.Get(h) == "" {
			t.Errorf("missing required header: %s", h)
		}
	}
	if gotHeaders.Get("Content-Type") != "application/json" {
		t.Error("Content-Type must be application/json")
	}
}

func TestSend_EnvelopeHasCorrectShape(t *testing.T) {
	const secret = "nps_secret"
	var gotEnvelope Envelope

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotEnvelope)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successBody(map[string]any{}, secret))
	}))
	defer srv.Close()

	var out map[string]any
	testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "collect_payment", Payload: map[string]string{"key": "val"}},
		&out,
	)

	if gotEnvelope.Intent != "execute" {
		t.Errorf("intent = %q, want execute", gotEnvelope.Intent)
	}
	if gotEnvelope.Service != SDKService {
		t.Errorf("service = %q, want %q", gotEnvelope.Service, SDKService)
	}
	if gotEnvelope.Action != "collect_payment" {
		t.Errorf("action = %q, want collect_payment", gotEnvelope.Action)
	}
}

// ── Error handling ────────────────────────────────────────────────────────────

func TestSend_BackendErrorReturnsSDKError(t *testing.T) {
	const secret = "nps_secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(errorBody(`{"category":"validation","message":"amount is required"}`))
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "collect_payment", Payload: map[string]string{}},
		&out,
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	sdkErr, ok := err.(*SDKError)
	if !ok {
		t.Fatalf("expected *SDKError, got %T", err)
	}
	if sdkErr.Category != "validation" {
		t.Errorf("category = %q, want validation", sdkErr.Category)
	}
}

func TestSend_RejectsInvalidResponseSignature(t *testing.T) {
	const secret = "nps_secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Correct structure but wrong signature.
		body, _ := json.Marshal(map[string]any{
			"status":  true,
			"message": "ok",
			"data":    json.RawMessage(`{"reference":"ref1","_responseSignature":"badhex"}`),
		})
		w.Write(body)
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "get_status", Payload: map[string]string{}},
		&out,
	)
	if err == nil {
		t.Fatal("expected error on bad response signature, got nil")
	}
}

func TestSend_RejectsMissingResponseSignature(t *testing.T) {
	const secret = "nps_secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]any{
			"status":  true,
			"message": "ok",
			"data":    json.RawMessage(`{"reference":"ref1"}`), // no _responseSignature
		})
		w.Write(body)
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "get_status", Payload: map[string]string{}},
		&out,
	)
	if err == nil {
		t.Fatal("expected error when _responseSignature is absent, got nil")
	}
}

// ── Retry behaviour ───────────────────────────────────────────────────────────

func TestSend_RetriesOnTransientError(t *testing.T) {
	const secret = "nps_secret"
	attempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":false,"message":"unavailable","data":null}`))
			return
		}
		w.Write(successBody(map[string]any{"ok": true}, secret))
	}))
	defer srv.Close()

	tr := NewTransport(TransportConfig{
		APIKey:     "npk_test",
		APISecret:  secret,
		BaseURL:    srv.URL,
		MaxRetries: 3,
	})

	var out map[string]any
	err := tr.Send(context.Background(),
		TransportRequest{Action: "test", Payload: map[string]string{}},
		&out,
	)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestSend_DoesNotRetryOn4xx(t *testing.T) {
	const secret = "nps_secret"
	attempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"invalid input"}`))
	}))
	defer srv.Close()

	tr := NewTransport(TransportConfig{
		APIKey:     "npk_test",
		APISecret:  secret,
		BaseURL:    srv.URL,
		MaxRetries: 3,
	})

	var out map[string]any
	tr.Send(context.Background(),
		TransportRequest{Action: "test", Payload: map[string]string{}},
		&out,
	)
	if attempts != 1 {
		t.Errorf("4xx must not be retried; attempts = %d, want 1", attempts)
	}
}

func TestSend_ExceedsMaxRetries(t *testing.T) {
	const secret = "nps_secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tr := NewTransport(TransportConfig{
		APIKey:     "npk_test",
		APISecret:  secret,
		BaseURL:    srv.URL,
		MaxRetries: 2,
	})

	var out map[string]any
	err := tr.Send(context.Background(),
		TransportRequest{Action: "test", Payload: map[string]string{}},
		&out,
	)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

// ── Envelope fingerprint ──────────────────────────────────────────────────────

func TestSend_EmbedsFingerprintInPayload(t *testing.T) {
	const secret = "nps_secret"
	var gotPayload map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Envelope
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &env)
		if payloadMap, ok := env.Payload.(map[string]any); ok {
			gotPayload = payloadMap
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(successBody(map[string]any{}, secret))
	}))
	defer srv.Close()

	var out map[string]any
	testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: "test", Payload: map[string]string{"foo": "bar"}},
		&out,
	)

	if _, ok := gotPayload["_fingerprint"]; !ok {
		t.Error("_fingerprint must be present in every request payload")
	}
	if fp, _ := gotPayload["_fingerprint"].(string); !strings.HasPrefix(fp, "") || len(fp) != 64 {
		t.Errorf("_fingerprint = %q; expected a 64-char hex string (SHA-256)", fp)
	}
}
