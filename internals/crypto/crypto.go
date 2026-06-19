package crypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// 16 bytes translates to a 32-character hex-encoded string.
const DefaultNonceLength = 16

type SignatureInput struct {
	Fingerprint string
	Nonce       string
	Timestamp   string
	Payload     any
	Secret      string
}

func GenerateNonce() (string, error) {
	nonceBytes := make([]byte, DefaultNonceLength)
	_, err := rand.Read(nonceBytes)
	if err != nil {
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
	payload := strings.Join(components, "|")

	hash := sha256.Sum256([]byte(payload))

	return hex.EncodeToString(hash[:])
}

func CreateTimeStamp() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func CreateSignature(input SignatureInput) (string, error) {
	canonicalPayload, err := createCanonicalPayload(input.Payload)
	if err != nil {
		return "", err
	}

	payloadStr := fmt.Sprintf("%s.%s.%s.%s", input.Fingerprint, input.Nonce, input.Timestamp, canonicalPayload)

	mac := hmac.New(sha256.New, []byte(input.Secret))
	mac.Write([]byte(payloadStr))

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
