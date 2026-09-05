package types

import "fmt"

// ErrorCategory is the machine-readable classification carried by every
// SDKError. Branch on this, never on the message text and never on an HTTP
// status code: the backend answers 200 for success and 400 for every failure
// regardless of cause, so the status tells you nothing about what went wrong.
type ErrorCategory string

const (
	// CategoryAuth covers an invalid, missing, revoked or expired key, a bad
	// signature, a replayed request, or a scope violation.
	CategoryAuth ErrorCategory = "auth"
	// CategoryValidation means the input was rejected, either by this SDK
	// before the request left, or by the server.
	CategoryValidation ErrorCategory = "validation"
	// CategoryLimit means an account or KYC transaction limit was exceeded.
	CategoryLimit ErrorCategory = "limit"
	// CategoryRateLimit means too many requests were sent. Back off before
	// retrying rather than retrying immediately.
	CategoryRateLimit ErrorCategory = "rate_limit"
	// CategoryAccount means the merchant account is missing or not active.
	CategoryAccount ErrorCategory = "account"
	// CategoryProvider means the payment provider rejected the operation.
	CategoryProvider ErrorCategory = "provider"
	// CategoryDuplicate means the reference is already taken and cannot be
	// replayed to you because it belongs to another account. Retry with a new,
	// unique reference. Reusing a reference you own is not an error: the server
	// replays your existing transaction and flags it with Transaction.Duplicate.
	CategoryDuplicate ErrorCategory = "duplicate"
	// CategoryNotFound means the referenced transaction does not exist.
	CategoryNotFound ErrorCategory = "not_found"
	// CategoryInternal means an unexpected server-side failure, or a response
	// this SDK could not verify.
	CategoryInternal ErrorCategory = "internal"
	// CategoryNetwork means the request never reached the server (DNS, TLS,
	// connection).
	CategoryNetwork ErrorCategory = "network"
	// CategoryTimeout means the request exceeded the configured timeout.
	CategoryTimeout ErrorCategory = "timeout"
)

// KnownCategories is the fixed taxonomy. A category the backend sends that is
// not in this set is treated as CategoryInternal rather than trusted.
var KnownCategories = map[ErrorCategory]bool{
	CategoryAuth:       true,
	CategoryValidation: true,
	CategoryLimit:      true,
	CategoryRateLimit:  true,
	CategoryAccount:    true,
	CategoryProvider:   true,
	CategoryDuplicate:  true,
	CategoryNotFound:   true,
	CategoryInternal:   true,
	CategoryNetwork:    true,
	CategoryTimeout:    true,
}

// SDKError is the structured error returned by every SDK operation.
//
// Recover it with errors.As to branch on the category:
//
//	tx, err := client.CollectPaymentAndResolve(ctx, input)
//	if err != nil {
//	    var sdkErr *nylonpay.SDKError
//	    if errors.As(err, &sdkErr) {
//	        switch sdkErr.Category {
//	        case nylonpay.CategoryDuplicate:
//	            // reference is taken, retry with a new one
//	        case nylonpay.CategoryRateLimit:
//	            // back off before retrying
//	        }
//	    }
//	}
//
// Message is written for a person to read and may change between releases.
// Category and Retryable are the stable, machine-readable signals.
type SDKError struct {
	Category  ErrorCategory `json:"category"`
	Message   string        `json:"message"`
	Retryable bool          `json:"retryable"`
}

// Error implements the error interface.
func (e *SDKError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Category, e.Message)
}
