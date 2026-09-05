package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

func structToMap(in any) (map[string]any, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	var out map[string]any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func stripResponseSignature(raw json.RawMessage) (any, string, error) {
	if len(raw) == 0 {
		return nil, "", nil
	}

	var m map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		return nil, "", err
	}

	sigAny, exists := m["_responseSignature"]
	if !exists || sigAny == nil {
		return m, "", nil
	}

	sig, ok := sigAny.(string)
	if !ok {
		return m, "", nil
	}

	delete(m, "_responseSignature")
	return m, sig, nil
}

func calculateBackoff(attempt int) time.Duration {
	base := (1 << attempt) * 1000
	jitter := rand.Float64() * 500
	return time.Duration(float64(base)+jitter) * time.Millisecond
}

func buildHttpError(body []byte, statusCode int) *SDKError {
	var bodyParsed struct {
		Message string `json:"message"`
	}

	msg := fmt.Sprintf("HTTP %d", statusCode)
	if err := json.Unmarshal(body, &bodyParsed); err == nil && bodyParsed.Message != "" {
		msg = bodyParsed.Message
	}

	parsedErr := ParseError(msg)
	cat := parsedErr.Category
	if cat == "internal" {
		if c, ok := StatusCategory[statusCode]; ok {
			cat = c
		} else if statusCode >= 500 {
			cat = "internal"
		} else {
			cat = "validation"
		}
	}

	return &SDKError{
		Category:  cat,
		Message:   parsedErr.Message,
		Retryable: RetryableStatusCodes[statusCode],
	}
}

func ParseError(msg string) *SDKError {
	var parsed struct {
		Category types.ErrorCategory `json:"category"`
		Message  string              `json:"message"`
	}
	if err := json.Unmarshal([]byte(msg), &parsed); err == nil && parsed.Category != "" {
		return &SDKError{Category: parsed.Category, Message: parsed.Message}
	}

	matches := errorTypeSuffixRegex.FindStringSubmatch(msg)
	if len(matches) == 3 {
		cat := types.ErrorCategory(matches[2])
		if KnownCategories[cat] {
			return &SDKError{Category: cat, Message: matches[1]}
		}
	}

	return &SDKError{Category: "internal", Message: msg}
}

// stripRequestNonce removes the backend's echoed request nonce from a verified
// payload and returns it alongside the remaining data.
//
// The nonce is covered by the response signature, so once the HMAC verifies it
// is as trustworthy as the signature itself. Comparing it against the nonce
// just sent is what makes a response answer *this* request. It is stripped
// before anything reaches the merchant, since it is transport bookkeeping.
//
// The third return value reports whether a usable nonce was present at all. A
// missing field is a rejection, not a pass: a lenient "check it only when
// present" would accept exactly the pre-binding responses an attacker is most
// likely to be holding.
func stripRequestNonce(data any) (any, string, bool) {
	payload, ok := data.(map[string]any)
	if !ok {
		return data, "", false
	}

	raw, exists := payload["_requestNonce"]
	if !exists {
		return data, "", false
	}

	nonce, ok := raw.(string)
	if !ok {
		return data, "", false
	}

	delete(payload, "_requestNonce")
	return payload, nonce, true
}
