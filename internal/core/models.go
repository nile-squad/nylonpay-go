package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	BASE_URL            = "https://api.nylonpay.nilesquad.com/api/services"
	TIMEOUT             = 30 * time.Second
	MAX_RETRIES         = 3
	SDKService          = "sdk"
	DefaultPollInterval = 2 * time.Second
	DefaultPollDuration = 5 * time.Minute
	DefaultPollAttempts = 150
	PollJitter          = 500 * time.Millisecond
)

var KnownCategories = map[string]bool{
	"auth":       true,
	"validation": true,
	"limit":      true,
	"rate_limit": true,
	"account":    true,
	"provider":   true,
	"duplicate":  true,
	"not_found":  true,
	"internal":   true,
	"network":    true,
	"timeout":    true,
}

var StatusCategory = map[int]string{
	http.StatusRequestTimeout:  "timeout",
	http.StatusTooManyRequests: "rate_limit",
}

var RetryableStatusCodes = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

var terminalStates = map[string]bool{
	"successful": true,
	"failed":     true,
	"cancelled":  true,
}

// SDKError is the structured error type returned by all SDK operations.
type SDKError struct {
	Category  string `json:"category"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *SDKError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Category, e.Message)
}

// TransportRequest is the input envelope for a single SDK action.
type TransportRequest struct {
	Action  string
	Payload any
}

type Envelope struct {
	Intent  string `json:"intent"`
	Service string `json:"service"`
	Action  string `json:"action"`
	Payload any    `json:"payload"`
}

type BackendResponse struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}
