package core

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

const (
	BASE_URL            = "https://api.nylonpay.nilesquad.com/api/services"
	TIMEOUT             = 30 * time.Second
	MAX_RETRIES         = 3
	SDKService          = "sdk"
	DefaultPollInterval = 2 * time.Second
	PollJitter          = 250 * time.Millisecond

	// MaxResponseBytes bounds how much of a response body is read before the
	// transport gives up. Response bodies are signed in full for verification,
	// so without a bound an oversized reply could exhaust memory during the
	// read, before verification ever runs.
	MaxResponseBytes int64 = 10 * 1024 * 1024
)

// SDKError is the structured error every operation returns. It is an alias for
// the public types.SDKError so that consumers, who cannot import this internal
// package, can still recover it with errors.As.
type SDKError = types.SDKError

// KnownCategories is re-exported for convenience within the module.
var KnownCategories = types.KnownCategories

// StatusCategory maps the few HTTP statuses that carry meaning on their own.
// It is a fallback only: the backend tags the category onto the message, and
// that tag always wins. The SDK never classifies a tagged error by status.
var StatusCategory = map[int]types.ErrorCategory{
	http.StatusRequestTimeout:  types.CategoryTimeout,
	http.StatusTooManyRequests: types.CategoryRateLimit,
}

var RetryableStatusCodes = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// terminalStates are the statuses that stop polling. "on_hold" is deliberately
// absent: a payout parked for review is still in flight.
var terminalStates = map[string]bool{
	"successful": true,
	"failed":     true,
	"cancelled":  true,
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

// Backend action names. Every operation POSTs to the same endpoint; the action
// in the envelope is what selects it.
const (
	ActionCollectPayment           = "sdk-collect-payment"
	ActionCollectPaymentAndResolve = "sdk-collect-payment-and-resolve"
	ActionMakePayout               = "sdk-make-payout"
	ActionMakePayoutAndResolve     = "sdk-make-payout-and-resolve"
	ActionGetStatus                = "sdk-get-status"
	ActionGetTransaction           = "sdk-get-transaction"
	ActionListTransactions         = "sdk-list-transactions"
	ActionVerifyPhone              = "sdk-verify-phone"
	ActionCreateInvoice            = "sdk-create-invoice"
)
