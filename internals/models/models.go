package models

import (
	"encoding/json"
	"fmt"
	"net/http"
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

type SDKError struct {
	Category  string `json:"category"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *SDKError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Category, e.Message)
}

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
