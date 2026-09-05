package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/nile-squad/nylonpay-go/internal/crypto"
	"github.com/nile-squad/nylonpay-go/types"
)

var errorTypeSuffixRegex = regexp.MustCompile(`^(?s)(.*?)\s*--\s*error-type:\s*([a-z_]+)\s*$`)

var cachedFingerprint = crypto.GenerateFingerprint()

// Messages surfaced to merchants. They state the outcome in plain language and
// name no internal mechanics: no nonce, no HMAC, no signature verification, no
// field names, no raw error dumps. The machine-readable signal is the category.
const (
	msgUnverifiedResponse = "Could not verify the server response"
	msgInvalidResponse    = "Received an invalid response from the server"
	msgTimedOut           = "The request timed out"
	msgCancelled          = "The request was cancelled before it completed"
	msgUnreachable        = "Could not reach the server, check your network connection and try again"
	msgRequestNotPrepared = "Could not prepare the request"
)

var errResponseTooLarge = errors.New("response body exceeds the maximum size")

// Send performs one SDK action, retrying transient failures.
//
// The body is built once and reused byte-for-byte across attempts, so the
// reference (which is the idempotency key) is constant and a retry replays the
// existing transaction rather than starting a second one. The auth headers,
// however, are regenerated per attempt: a fresh nonce, timestamp and signature
// every time. That is required, not cosmetic. The backend accepts a given
// (key, nonce) pair exactly once and remembers it for ten minutes, and it
// rejects any timestamp more than five minutes from its own clock, so a retry
// that reused the first attempt's identity would be refused as a replay, and
// would age out of the freshness window during backoff.
func (t *Transport) Send(ctx context.Context, req TransportRequest, out any) error {
	payloadMap, err := structToMap(req.Payload)
	if err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgRequestNotPrepared}
	}
	// A nil payload (for example a before* hook that returned nothing) decodes
	// to a nil map, which cannot be written to. Treat it as empty.
	if payloadMap == nil {
		payloadMap = map[string]any{}
	}
	payloadMap["_fingerprint"] = cachedFingerprint

	bodyBytes, err := json.Marshal(Envelope{
		Intent:  "execute",
		Service: SDKService,
		Action:  req.Action,
		Payload: payloadMap,
	})
	if err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgRequestNotPrepared}
	}

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if waitErr := sleepWithContext(ctx, calculateBackoff(attempt-1)); waitErr != nil {
				return waitErr
			}
		}

		sdkErr, retryable := t.sendOnce(ctx, bodyBytes, payloadMap, out)
		if sdkErr == nil {
			return nil
		}
		if !retryable || attempt >= t.config.MaxRetries {
			return sdkErr
		}
	}
}

// sendOnce performs a single signed attempt. It reports the failure and whether
// that failure is worth retrying.
func (t *Transport) sendOnce(ctx context.Context, bodyBytes []byte, signedPayload any, out any) (*SDKError, bool) {
	nonce, err := crypto.GenerateNonce()
	if err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgRequestNotPrepared}, false
	}
	timestamp := crypto.CreateTimeStamp()

	signature, err := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: cachedFingerprint,
		Nonce:       nonce,
		Timestamp:   timestamp,
		Payload:     signedPayload,
		Secret:      t.config.APISecret,
	})
	if err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgRequestNotPrepared}, false
	}

	attemptCtx, cancel := context.WithTimeout(ctx, t.config.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, t.config.BaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgRequestNotPrepared}, false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Nylon-Key", t.config.APIKey)
	httpReq.Header.Set("X-Nylon-Nonce", nonce)
	httpReq.Header.Set("X-Nylon-Timestamp", timestamp)
	httpReq.Header.Set("X-Nylon-Signature", signature)

	resp, err := t.config.HTTPClient.Do(httpReq)
	if err != nil {
		return classifyRequestError(ctx, attemptCtx)
	}
	defer resp.Body.Close()

	respBody, err := readCapped(resp.Body, t.maxResponseBytes())
	if err != nil {
		// An oversized or unreadable body is rejected before any parsing or
		// verification happens, and never fully buffered.
		return &SDKError{Category: types.CategoryInternal, Message: msgInvalidResponse}, false
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return buildHttpError(respBody, resp.StatusCode), RetryableStatusCodes[resp.StatusCode]
	}

	var envResp BackendResponse
	if err := json.Unmarshal(respBody, &envResp); err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgInvalidResponse}, false
	}

	if !envResp.Status {
		return ParseError(envResp.Message), false
	}

	return t.decodeVerifiedData(envResp.Data, nonce, out), false
}

// decodeVerifiedData verifies a success payload and unmarshals it into out.
//
// Two checks, both fail-closed. The signature proves the backend produced this
// response; the echoed nonce proves it answers the request just sent. Without
// the second, any response the backend ever legitimately produced stays validly
// signed forever and can be replayed onto a later call for the same reference.
// Both failures surface as the same message on purpose, telling an attacker
// which check failed buys them information and buys the merchant nothing.
func (t *Transport) decodeVerifiedData(data json.RawMessage, sentNonce string, out any) *SDKError {
	unverified := &SDKError{Category: types.CategoryInternal, Message: msgUnverifiedResponse}

	stripped, signature, err := stripResponseSignature(data)
	if err != nil || signature == "" {
		return unverified
	}

	// Verified over the payload that still carries _requestNonce; only
	// _responseSignature is removed before hashing.
	valid, err := crypto.VerifyResponseSignature(stripped, signature, t.config.APISecret)
	if err != nil || !valid {
		return unverified
	}

	unbound, echoedNonce, found := stripRequestNonce(stripped)
	if !found || echoedNonce != sentNonce {
		return unverified
	}

	verifiedBytes, err := json.Marshal(unbound)
	if err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgInvalidResponse}
	}
	if err := json.Unmarshal(verifiedBytes, out); err != nil {
		return &SDKError{Category: types.CategoryInternal, Message: msgInvalidResponse}
	}
	return nil
}

// classifyRequestError separates the three ways a request can fail to complete.
// Caller cancellation is not a timeout and is never retried: the caller asked
// us to stop, so trying again would ignore them.
func classifyRequestError(callerCtx, attemptCtx context.Context) (*SDKError, bool) {
	if errors.Is(callerCtx.Err(), context.Canceled) {
		return &SDKError{Category: types.CategoryTimeout, Message: msgCancelled}, false
	}
	if callerCtx.Err() != nil || errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return &SDKError{Category: types.CategoryTimeout, Message: msgTimedOut, Retryable: true}, true
	}
	return &SDKError{Category: types.CategoryNetwork, Message: msgUnreachable, Retryable: true}, true
}

// readCapped reads at most limit bytes, treating anything longer as a failure.
//
// The cap is applied during the read, against a running byte count, rather than
// by inspecting Content-Length: a header check cannot bound peak memory once
// the body is already buffered, and is a no-op entirely when the server sends
// no length at all (chunked transfer). Reading one byte past the limit is what
// distinguishes "exactly at the cap" from "over it".
func readCapped(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errResponseTooLarge
	}
	return data, nil
}

func (t *Transport) maxResponseBytes() int64 {
	if t.config.MaxResponseBytes > 0 {
		return t.config.MaxResponseBytes
	}
	return MaxResponseBytes
}

// sleepWithContext waits out a backoff delay, giving up early if the caller
// cancels. A bare time.Sleep would keep a cancelled call alive for the whole
// backoff.
func sleepWithContext(ctx context.Context, delay time.Duration) *SDKError {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.Canceled) {
			return &SDKError{Category: types.CategoryTimeout, Message: msgCancelled}
		}
		return &SDKError{Category: types.CategoryTimeout, Message: msgTimedOut, Retryable: true}
	case <-timer.C:
		return nil
	}
}

// msgPollTimeout is what a merchant sees when a status wait hits their
// configured cap. It names the outcome, not the mechanism.
const msgPollTimeout = "Timed out waiting for the transaction status to update"
