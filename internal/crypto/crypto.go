package crypto

import (
	"bytes"
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

const DefaultNonceLength = 16
const DefaultToleranceSeconds = 300

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

func GenerateNonce() (string, error) {
	nonceBytes := make([]byte, DefaultNonceLength)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("failed to generate secure random bytes: %w", err)
	}
	return hex.EncodeToString(nonceBytes), nil
}

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

func VerifyResponseSignature(data any, signature, secret string) (bool, error) {
	canonicalJSON, err := createCanonicalPayload(data)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize response data: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonicalJSON))
	expectedBytes := mac.Sum(nil)

	providedBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false, nil
	}
	if len(providedBytes) != len(expectedBytes) {
		return false, nil
	}

	return subtle.ConstantTimeCompare(providedBytes, expectedBytes) == 1, nil
}

func VerifyWebhookSignature(input VerifyWebhookInput) bool {
	mac := hmac.New(sha256.New, []byte(input.Secret))
	mac.Write(input.Payload)
	expectedBytes := mac.Sum(nil)

	providedBytes, err := hex.DecodeString(input.Signature)
	if err != nil {
		return false
	}
	if len(providedBytes) != len(expectedBytes) {
		return false
	}
	if subtle.ConstantTimeCompare(providedBytes, expectedBytes) != 1 {
		return false
	}

	toleranceSeconds := DefaultToleranceSeconds
	if input.ToleranceSeconds != nil {
		toleranceSeconds = *input.ToleranceSeconds
	}
	if toleranceSeconds <= 0 {
		return true
	}

	timestamp, err := extractSignedTimestamp(input.Payload)
	if err != nil {
		return false
	}

	age := time.Since(timestamp)
	if age < 0 {
		age = -age
	}
	return age <= time.Duration(toleranceSeconds)*time.Second
}

func CreateTimeStamp() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

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

func createCanonicalPayload(payload any) (string, error) {
	initialBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	var generic any
	decoder := json.NewDecoder(bytes.NewReader(initialBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return "", fmt.Errorf("failed to normalize payload structure: %w", err)
	}

	canonicalBytes, err := json.Marshal(generic)
	if err != nil {
		return "", fmt.Errorf("failed to generate canonical JSON: %w", err)
	}
	return string(canonicalBytes), nil
}

func extractSignedTimestamp(payload []byte) (time.Time, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(payload, &doc); err != nil {
		return time.Time{}, err
	}

	raw, exists := doc["timestamp"]
	if !exists {
		return time.Time{}, fmt.Errorf("timestamp field missing")
	}

	var num float64
	if err := json.Unmarshal(raw, &num); err == nil {
		if num < 1e12 {
			return time.Unix(int64(num), 0), nil
		}
		return time.UnixMilli(int64(num)), nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		if t, err := time.Parse(time.RFC3339, str); err == nil {
			return t, nil
		}
		if t, err := time.Parse("2006-01-02T15:04:05.999999999Z07:00", str); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unknown or unsupported timestamp format")
}
