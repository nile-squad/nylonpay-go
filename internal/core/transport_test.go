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
	"sync"
	"testing"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
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

// successBody wraps data in a signed BackendResponse envelope, echoing the
// request nonce inside the signed payload the way the real backend does. That
// echo is what binds the response to the request that asked for it; without it
// the transport rejects the response as unverifiable.
func successBody(data map[string]any, secret, requestNonce string) []byte {
	data["_requestNonce"] = requestNonce
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
		w.Write(successBody(map[string]any{"reference": "ref123", "status": "pending"}, secret, r.Header.Get("X-Nylon-Nonce")))
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
		w.Write(successBody(map[string]any{}, secret, r.Header.Get("X-Nylon-Nonce")))
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
		w.Write(successBody(map[string]any{}, secret, r.Header.Get("X-Nylon-Nonce")))
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
		w.Write(successBody(map[string]any{"ok": true}, secret, r.Header.Get("X-Nylon-Nonce")))
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
		w.Write(successBody(map[string]any{}, secret, r.Header.Get("X-Nylon-Nonce")))
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

// ── Malformed and unexpected responses ────────────────────────────────────────

func TestSend_NonJSONBodyIsRejected(t *testing.T) {
	const secret = "nps_secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: ActionGetStatus, Payload: map[string]string{"reference": "ref"}}, &out)

	assertSDKErrorCategory(t, err, types.CategoryInternal)
}

func TestSend_EmptyBodyIsRejected(t *testing.T) {
	const secret = "nps_secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: ActionGetStatus, Payload: map[string]string{"reference": "ref"}}, &out)

	assertSDKErrorCategory(t, err, types.CategoryInternal)
}

// The backend tags the category onto the message, which is the only channel
// that survives its 200/400-only responses. That tag always wins over status.
func TestSend_TaggedMessageWinsOverHTTPStatus(t *testing.T) {
	const secret = "nps_secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errorBody("API key was not found -- error-type: auth"))
	}))
	defer srv.Close()

	var out map[string]any
	err := testTransport(srv, secret).Send(context.Background(),
		TransportRequest{Action: ActionGetStatus, Payload: map[string]string{"reference": "ref"}}, &out)

	sdkErr := assertSDKErrorCategory(t, err, types.CategoryAuth)
	if sdkErr.Message != "API key was not found" {
		t.Errorf("message = %q, want the human portion without the tag", sdkErr.Message)
	}
}

func TestSend_UnauthorizedAndUnprocessableAreNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			const secret = "nps_secret"
			var mu sync.Mutex
			attempts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				attempts++
				mu.Unlock()
				w.WriteHeader(status)
				w.Write(errorBody("rejected"))
			}))
			defer srv.Close()

			transport := NewTransport(TransportConfig{
				APIKey: "npk_test", APISecret: secret, BaseURL: srv.URL,
				Timeout: 5 * time.Second, MaxRetries: 3,
			})

			var out map[string]any
			if err := transport.Send(context.Background(),
				TransportRequest{Action: ActionGetStatus, Payload: map[string]string{"reference": "ref"}}, &out); err == nil {
				t.Fatal("expected an error")
			}
			mu.Lock()
			defer mu.Unlock()
			if attempts != 1 {
				t.Errorf("attempts = %d, want 1: client errors are returned immediately", attempts)
			}
		})
	}
}

// Each attempt carries a fresh nonce, timestamp and signature over a body that
// never changes. Reusing them would be rejected by the backend as a replay.
func TestSend_EachRetryIsSignedFresh(t *testing.T) {
	const secret = "nps_secret"
	var mu sync.Mutex
	var nonces, signatures, bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		nonces = append(nonces, r.Header.Get("X-Nylon-Nonce"))
		signatures = append(signatures, r.Header.Get("X-Nylon-Signature"))
		bodies = append(bodies, string(body))
		seen := len(nonces)
		mu.Unlock()

		if seen < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write(successBody(map[string]any{"ok": true}, secret, r.Header.Get("X-Nylon-Nonce")))
	}))
	defer srv.Close()

	transport := NewTransport(TransportConfig{
		APIKey: "npk_test", APISecret: secret, BaseURL: srv.URL,
		Timeout: 5 * time.Second, MaxRetries: 3,
	})

	var out map[string]any
	if err := transport.Send(context.Background(),
		TransportRequest{Action: ActionGetStatus, Payload: map[string]string{"reference": "ref"}}, &out); err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(nonces) != 3 {
		t.Fatalf("attempts = %d, want 3", len(nonces))
	}
	for i := 1; i < len(nonces); i++ {
		if nonces[i] == nonces[0] {
			t.Error("every attempt must carry a distinct nonce")
		}
		if signatures[i] == signatures[0] {
			t.Error("every attempt must be signed afresh")
		}
		if bodies[i] != bodies[0] {
			t.Error("the body, and therefore the reference, must not change between attempts")
		}
	}
}

func TestSend_CancelledContextIsNotRetried(t *testing.T) {
	const secret = "nps_secret"
	var mu sync.Mutex
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	transport := NewTransport(TransportConfig{
		APIKey: "npk_test", APISecret: secret, BaseURL: srv.URL,
		Timeout: 5 * time.Second, MaxRetries: 3,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	var out map[string]any
	if err := transport.Send(ctx,
		TransportRequest{Action: ActionGetStatus, Payload: map[string]string{"reference": "ref"}}, &out); err == nil {
		t.Fatal("expected an error after cancellation")
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts > 1 {
		t.Errorf("attempts = %d: a cancelled request must not be retried", attempts)
	}
}

func assertSDKErrorCategory(t *testing.T, err error, want types.ErrorCategory) *types.SDKError {
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
	return sdkErr
}
