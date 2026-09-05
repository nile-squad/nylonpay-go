package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultNonceLength is the nonce size in bytes; hex-encoded it becomes the
// 32-character value the backend expects.
const DefaultNonceLength = 16

// DefaultToleranceSeconds is the webhook replay window.
const DefaultToleranceSeconds = 300

// DisableFreshnessCheck turns off webhook replay protection entirely.
//
// It must be passed deliberately. A tolerance of 0 does NOT disable the check:
// it means a tolerance of zero seconds, the strictest possible setting, which
// in practice rejects everything. That is the safe reading of the two possible
// misunderstandings. Someone reaching for 0 is thinking about security, and
// under the old semantics they silently got no replay protection at all,
// permanently, with verification still returning true so nothing looked wrong.
const DisableFreshnessCheck = -1

type SignatureInput struct {
	Fingerprint string
	Nonce       string
	Timestamp   string
	Payload     any
	Secret      string
}

type VerifyWebhookInput struct {
	Payload          []byte
	Signature        string
	Secret           string
	ToleranceSeconds *int
}

// GenerateNonce returns 32 hex characters from a cryptographic source. The
// backend accepts a given (key, nonce) pair once and remembers it for ten
// minutes, so a nonce is never reused, including across retries of one call.
func GenerateNonce() (string, error) {
	nonceBytes := make([]byte, DefaultNonceLength)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("failed to generate secure random bytes: %w", err)
	}
	return hex.EncodeToString(nonceBytes), nil
}

// GenerateFingerprint derives a stable 64-character identifier for this process
// from OS and runtime metadata. The backend treats it as opaque and uses it for
// anomaly detection; it must stay constant for the life of the process and must
// match the _fingerprint sent in the request body.
func GenerateFingerprint() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	components := []string{
		"go_version:" + runtime.Version(),
		"os:" + runtime.GOOS,
		"arch:" + runtime.GOARCH,
		"hostname:" + hostname,
		"num_cpu:" + fmt.Sprintf("%d", runtime.NumCPU()),
	}

	hash := sha256.Sum256([]byte(strings.Join(components, "|")))
	return hex.EncodeToString(hash[:])
}

// isCanonicalHexSignature reports whether a signature is spelled the one way
// Nylon Pay emits: lowercase hex.
//
// Comparing decoded bytes alone is case-blind, so it would accept the same
// digest re-spelled in uppercase, a second valid form on the wire. The backend
// only ever emits lowercase, so nothing legitimate is rejected by insisting.
func isCanonicalHexSignature(signature string) bool {
	return signature != "" && signature == strings.ToLower(signature)
}

// constantTimeHexEqual compares a received hex signature against expected raw
// bytes without leaking timing information, and without panicking on malformed
// input. A signature that is not canonical, not hex, or the wrong length is
// rejected before the comparison.
func constantTimeHexEqual(signature string, expected []byte) bool {
	if !isCanonicalHexSignature(signature) {
		return false
	}

	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(provided, expected) == 1
}

// VerifyResponseSignature checks that a response body was produced by the
// holder of the API secret. The caller must have already removed the
// _responseSignature field; everything else, including _requestNonce, is
// covered by the signature.
func VerifyResponseSignature(data any, signature, secret string) (bool, error) {
	canonicalJSON, err := createCanonicalPayload(data)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize response data: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonicalJSON))

	return constantTimeHexEqual(signature, mac.Sum(nil)), nil
}

// VerifyWebhookSignature reports whether a webhook genuinely came from Nylon
// Pay and is not a replay. It never panics, on any input.
//
// Two checks, both must pass:
//
//  1. Authenticity: HMAC-SHA256 over the raw payload bytes matches the
//     signature. The bytes are hashed exactly as supplied, never decoded to a
//     string and re-encoded, which would rewrite any sequence that is not valid
//     UTF-8 and fail a payload that is genuinely authentic.
//  2. Freshness: the timestamp carried inside the signed body is within the
//     tolerance window. This is what stops a replay, a captured (body,
//     signature) pair stays cryptographically valid forever, but its embedded
//     timestamp goes stale. Every genuine delivery, including a retry hours
//     later, is re-stamped and re-signed, so this never rejects real traffic.
//
// The freshness anchor is read from the signed body, never from a transport
// header, so it cannot be refreshed without the secret. It is read only after
// the HMAC verifies, so it operates on trusted content.
//
// Note that the webhook secret is a separate credential from the API secret.
// Passing the API secret here makes every webhook fail verification.
func VerifyWebhookSignature(input VerifyWebhookInput) (valid bool) {
	// Belt and braces: this function is documented never to raise, so a
	// surprise from any input shape still returns false rather than escaping.
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()

	mac := hmac.New(sha256.New, []byte(input.Secret))
	mac.Write(input.Payload)

	if !constantTimeHexEqual(input.Signature, mac.Sum(nil)) {
		return false
	}

	toleranceSeconds := DefaultToleranceSeconds
	if input.ToleranceSeconds != nil {
		toleranceSeconds = *input.ToleranceSeconds
	}
	if toleranceSeconds == DisableFreshnessCheck {
		return true
	}
	if toleranceSeconds < 0 {
		return false
	}

	timestamp, err := extractSignedTimestamp(input.Payload)
	if err != nil {
		// Fail closed. A valid signature with no verifiable timestamp cannot be
		// proven fresh, so it cannot be told apart from a replay.
		return false
	}

	age := time.Since(timestamp)
	if age < 0 {
		age = -age
	}
	return age <= time.Duration(toleranceSeconds)*time.Second
}

// CreateTimeStamp returns the current time as epoch milliseconds. The backend
// rejects a timestamp more than five minutes from its own clock, so this must
// come from the system clock rather than a cached or monotonic-only source.
func CreateTimeStamp() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// CreateSignature computes the request signature over
// fingerprint.nonce.timestamp.canonicalPayload, keyed with the API secret as
// raw UTF-8 bytes.
//
// Payload must be the inner payload object (the operation input plus
// _fingerprint), NOT the full {intent, service, action, payload} envelope.
// Signing the envelope produces a well-formed request that fails auth every
// time, and is the most common first-implementation mistake.
func CreateSignature(input SignatureInput) (string, error) {
	canonicalPayload, err := createCanonicalPayload(input.Payload)
	if err != nil {
		return "", err
	}

	raw := fmt.Sprintf("%s.%s.%s.%s", input.Fingerprint, input.Nonce, input.Timestamp, canonicalPayload)
	mac := hmac.New(sha256.New, []byte(input.Secret))
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// extractSignedTimestamp reads the top-level "timestamp" from a webhook body.
//
// Nylon Pay emits ISO 8601 UTC with milliseconds, but epoch seconds and
// milliseconds are also accepted, in both numeric and string form, so a
// merchant's own tooling can re-stamp a body in tests and so every Nylon Pay
// SDK agrees on the same set of timestamp shapes.
func extractSignedTimestamp(payload []byte) (time.Time, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(payload, &doc); err != nil {
		return time.Time{}, err
	}

	raw, exists := doc["timestamp"]
	if !exists {
		return time.Time{}, fmt.Errorf("timestamp field missing")
	}

	var num json.Number
	if err := json.Unmarshal(raw, &num); err == nil {
		return epochToTime(num.String())
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		// Numeric strings are checked first: time.Parse would not read
		// "1718976000" as an instant, and the Python SDK accepts it.
		if trimmed := strings.TrimSpace(str); trimmed != "" {
			if parsed, err := epochToTime(trimmed); err == nil {
				return parsed, nil
			}
		}
		if t, err := time.Parse(time.RFC3339, str); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unknown or unsupported timestamp format")
}

// epochToTime interprets a numeric literal as epoch seconds or milliseconds.
// Values below 1e12 are seconds; anything larger is milliseconds.
func epochToTime(literal string) (time.Time, error) {
	value, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		return time.Time{}, err
	}
	if value < 1e12 {
		return time.Unix(int64(value), 0), nil
	}
	return time.UnixMilli(int64(value)), nil
}
