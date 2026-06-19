package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/nile-squad/nylonpay-go/internals/models"
)

// Converts a struct payload into a map so we can easily inject _fingerprint safely
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

// Removes `_responseSignature` from the JSON payload
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

func buildHttpError(body []byte, statusCode int) *models.SDKError {
	var bodyParsed struct {
		Message string `json:"message"`
	}

	msg := fmt.Sprintf("HTTP %d", statusCode)
	if err := json.Unmarshal(body, &bodyParsed); err != nil && bodyParsed.Message == "" {
		msg = bodyParsed.Message
	}

	parsedErr := ParseError(msg)
	cat := parsedErr.Category
	if cat == "internal" {
		if c, ok := models.StatusCategory[statusCode]; ok {
			cat = c
		} else if statusCode >= 500 {
			cat = "internal"
		} else {
			cat = "validation"
		}
	}

	return &models.SDKError{
		Category:  cat,
		Message:   parsedErr.Message,
		Retryable: models.RetryableStatusCodes[statusCode],
	}
}

func ParseError(msg string) *models.SDKError {

	// Try parsing JSON first
	var parsed struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	}

	if err := json.Unmarshal([]byte(msg), &parsed); err == nil && parsed.Category != "" {
		return &models.SDKError{
			Category: parsed.Category,
			Message:  parsed.Message,
		}
	}

	// Fallback to regex suffix extraction
	matches := errorTypeSuffixRegex.FindStringSubmatch(msg)
	if len(matches) == 3 {
		cat := matches[2]
		if models.KnownCategories[cat] {
			return &models.SDKError{
				Category: cat,
				Message:  matches[1],
			}
		}
	}
	return &models.SDKError{
		Category: "internal",
		Message:  msg,
	}
}
