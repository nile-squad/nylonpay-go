package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/nile-squad/nylonpay-go/internal/crypto"
)

var errorTypeSuffixRegex = regexp.MustCompile(`^(?s)(.*?)\s*--\s*error-type:\s*([a-z_]+)\s*$`)

var cachedFingerprint = crypto.GenerateFingerprint()

// Send serialises req into a signed envelope, executes it with retries, and
// unmarshals the verified response into out.
func (t *Transport) Send(ctx context.Context, req TransportRequest, out any) error {
	payloadMap, err := structToMap(req.Payload)
	if err != nil {
		return &SDKError{Category: "internal", Message: "Failed to parse payload"}
	}

	payloadMap["_fingerprint"] = cachedFingerprint

	env := Envelope{
		Intent:  "execute",
		Service: SDKService,
		Action:  req.Action,
		Payload: payloadMap,
	}

	bodyBytes, err := json.Marshal(env)
	if err != nil {
		return &SDKError{Category: "internal", Message: "Failed to encode envelope"}
	}

	nonce, _ := crypto.GenerateNonce()
	timestamp := crypto.CreateTimeStamp()

	signature, err := crypto.CreateSignature(crypto.SignatureInput{
		Fingerprint: cachedFingerprint,
		Nonce:       nonce,
		Timestamp:   timestamp,
		Payload:     payloadMap,
		Secret:      t.config.APISecret,
	})
	if err != nil {
		return &SDKError{Category: "internal", Message: "Failed to sign request"}
	}

	headers := http.Header{
		"Content-Type":      {"application/json"},
		"X-Nylon-Key":       {t.config.APIKey},
		"X-Nylon-Nonce":     {nonce},
		"X-Nylon-Timestamp": {timestamp},
		"X-Nylon-Signature": {signature},
	}

	// attempt 0 is the initial request; 1..MaxRetries are retries.
	for attempt := 0; attempt <= t.config.MaxRetries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, t.config.Timeout)

		httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, t.config.BaseURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			cancel()
			return &SDKError{Category: "internal", Message: "Failed to create request"}
		}
		httpReq.Header = headers

		resp, err := t.config.HTTPClient.Do(httpReq)
		if err != nil {
			cancel()
			isTimeout := attemptCtx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled
			category := "network"
			msg := err.Error()

			if isTimeout {
				category = "timeout"
				msg = fmt.Sprintf("Request timed out after %v", t.config.Timeout)
			}

			if attempt < t.config.MaxRetries {
				time.Sleep(calculateBackoff(attempt))
				continue
			}
			return &SDKError{Category: category, Message: msg, Retryable: true}
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()

		if err != nil {
			return &SDKError{Category: "internal", Message: "Failed to read response body"}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			retryable := RetryableStatusCodes[resp.StatusCode]
			if retryable && attempt < t.config.MaxRetries {
				time.Sleep(calculateBackoff(attempt))
				continue
			}
			return buildHttpError(respBody, resp.StatusCode)
		}

		var envResp BackendResponse
		if err := json.Unmarshal(respBody, &envResp); err != nil {
			return &SDKError{Category: "internal", Message: "Response missing status field"}
		}

		if !envResp.Status {
			return ParseError(envResp.Message)
		}

		strippedMap, sig, err := stripResponseSignature(envResp.Data)
		if err != nil {
			return &SDKError{Category: "internal", Message: "Failed to process response data"}
		}

		if sig == "" {
			return &SDKError{Category: "internal", Message: "Response signature missing"}
		}

		isValid, err := crypto.VerifyResponseSignature(strippedMap, sig, t.config.APISecret)
		if err != nil || !isValid {
			return &SDKError{Category: "internal", Message: "Response signature verification failed"}
		}

		strippedBytes, _ := json.Marshal(strippedMap)
		if err := json.Unmarshal(strippedBytes, out); err != nil {
			return &SDKError{Category: "internal", Message: "Failed to map response to struct"}
		}

		return nil
	}

	return &SDKError{Category: "timeout", Message: "Max retries exceeded"}
}
