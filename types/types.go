package types

import "time"

// TransactionStatus is the lifecycle state of a transaction. Merchants drive
// fulfillment off these: complete the order on "successful", notify the
// customer on "failed", release inventory on "cancelled".
type TransactionStatus string

// TransactionType distinguishes what kind of money movement a record describes.
type TransactionType string

// PaymentMethod is how a collection is taken from the customer.
type PaymentMethod string

// PaymentEvent is a lifecycle event emitted by a PaymentInstance. Each fires at
// most once per instance.
type PaymentEvent string

// Currency is an ISO 4217 code from the supported set.
type Currency string

// TransactionMode records whether a transaction ran against test or live
// providers. It follows from the API key, not from any SDK setting.
type TransactionMode string

// WebhookEventType is the event name on an inbound webhook delivery.
type WebhookEventType string

// OnDelayedBehavior selects what happens when a payment is flagged delayed.
type OnDelayedBehavior string

const (
	// Non-terminal statuses. A payment in any of these is still in flight.
	TransactionStatusPending    TransactionStatus = "pending"
	TransactionStatusProcessing TransactionStatus = "processing"
	// TransactionStatusOnHold is a payout parked for review. Polling continues
	// through it; see Transaction.StatusText for a human explanation.
	TransactionStatusOnHold TransactionStatus = "on_hold"

	// Terminal statuses. Polling stops here.
	TransactionStatusSuccessful TransactionStatus = "successful"
	TransactionStatusFailed     TransactionStatus = "failed"
	TransactionStatusCancelled  TransactionStatus = "cancelled"

	TransactionTypePayout     TransactionType = "payout"
	TransactionTypeCollection TransactionType = "collection"
	TransactionTypeTransfer   TransactionType = "transfer"
	TransactionTypeEscrow     TransactionType = "escrow"
	TransactionTypeRefund     TransactionType = "refund"
	TransactionTypeReversal   TransactionType = "reversal"
	TransactionTypeCharge     TransactionType = "charge"
	TransactionTypeChargeBack TransactionType = "chargeback"

	PaymentMethodBank        PaymentMethod = "bank"
	PaymentMethodMobileMoney PaymentMethod = "mobileMoney"

	PaymentEventSuccess    PaymentEvent = "success"
	PaymentEventFailed     PaymentEvent = "failed"
	PaymentEventCancelled  PaymentEvent = "cancelled"
	PaymentEventProcessing PaymentEvent = "processing"
	PaymentEventError      PaymentEvent = "error"

	USD Currency = "USD"
	EUR Currency = "EUR"
	GBP Currency = "GBP"
	KES Currency = "KES"
	UGX Currency = "UGX"
	TZS Currency = "TZS"
	RWF Currency = "RWF"

	TransactionModeLive TransactionMode = "live"
	TransactionModeTest TransactionMode = "test"

	WebhookEventSuccessful WebhookEventType = "transaction.successful"
	WebhookEventFailed     WebhookEventType = "transaction.failed"
	WebhookEventProcessing WebhookEventType = "transaction.processing"
	WebhookEventCancelled  WebhookEventType = "transaction.cancelled"

	// OnDelayedWait keeps polling a delayed payment until it reaches a terminal
	// state. This is the default.
	OnDelayedWait OnDelayedBehavior = "wait"
	// OnDelayedReturn resolves with the still-pending payment as soon as it is
	// flagged delayed, leaving the outcome to arrive by webhook.
	OnDelayedReturn OnDelayedBehavior = "return"
)

// SupportedCurrencies is the set a payment may be denominated in.
var SupportedCurrencies = map[Currency]bool{
	USD: true, EUR: true, GBP: true, KES: true, UGX: true, TZS: true, RWF: true,
}

// IsTerminal reports whether a status ends the transaction's lifecycle.
// "on_hold" is not terminal: a payout under review is still in flight.
func (s TransactionStatus) IsTerminal() bool {
	switch s {
	case TransactionStatusSuccessful, TransactionStatusFailed, TransactionStatusCancelled:
		return true
	default:
		return false
	}
}

// Customer identifies who a payment is collected from or paid out to.
type Customer struct {
	Name string `json:"name"`
	// PhoneNumber accepts local (0768499027), international with or without a
	// leading plus, and any of those with spaces. The SDK normalizes it to
	// 256XXXXXXXXX before the request is signed.
	PhoneNumber string  `json:"phoneNumber"`
	Email       *string `json:"email,omitempty"`
}

// Destination is where a payout lands.
type Destination struct {
	AccountHolderName string  `json:"accountHolderName"`
	AccountNumber     string  `json:"accountNumber"`
	BankName          *string `json:"bankName,omitempty"`
	Phone             *string `json:"phone,omitempty"`
}

// InvoiceItem is one line on a hosted invoice.
type InvoiceItem struct {
	Name     string `json:"name"`
	Quantity int64  `json:"quantity"`
	// UnitPrice is the price per unit in the smallest currency unit.
	UnitPrice int64 `json:"unitPrice"`
}

// BankDetails is required when collecting with PaymentMethodBank.
type BankDetails struct {
	AccountNumber string `json:"accountNumber"`
	BankName      string `json:"bankName"`
}

// CollectPaymentInput initiates a collection from a customer.
type CollectPaymentInput struct {
	// Amount is a positive integer in the smallest currency unit. Minimum 500 UGX.
	Amount   int64    `json:"amount"`
	Currency Currency `json:"currency,omitempty"`
	Customer Customer `json:"customer"`
	// Description is the narration the customer sees.
	Description string `json:"description"`
	// Reference is the transaction identity and the only idempotency mechanism.
	// It must be a UUID; one is generated when omitted. Reusing a
	// reference replays the existing transaction rather than charging again.
	Reference string         `json:"reference,omitempty"`
	Method    *PaymentMethod `json:"method,omitempty"`
	Bank      *BankDetails   `json:"bank,omitempty"`
	// Tags are up to 10 short labels for filtering and reporting.
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MakePayoutInput initiates a disbursement to a destination account.
type MakePayoutInput struct {
	// Amount is a positive integer in the smallest currency unit. Minimum 5000 UGX.
	Amount      int64       `json:"amount"`
	Currency    Currency    `json:"currency,omitempty"`
	Customer    Customer    `json:"customer"`
	Destination Destination `json:"destination"`
	Description string      `json:"description"`
	// Reference must be a UUID; one is generated when omitted.
	Reference string            `json:"reference,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// CreateInvoiceInput generates a hosted invoice and emails it to the customer.
//
// Invoices are live-mode only: calling this with a sandbox key returns a
// validation error from the server.
type CreateInvoiceInput struct {
	// Amount is a positive integer in the smallest currency unit. Minimum 500 UGX.
	Amount   int64    `json:"amount"`
	Currency Currency `json:"currency,omitempty"`
	// CustomerEmail is required; the invoice is sent to this address.
	CustomerEmail string  `json:"customerEmail"`
	CustomerName  *string `json:"customerName,omitempty"`
	// CustomerPhone pre-fills the phone field on the payment page.
	CustomerPhone *string `json:"customerPhone,omitempty"`
	Description   *string `json:"description,omitempty"`
	// DueDate is an ISO 8601 date string, for example "2025-12-31".
	DueDate *string       `json:"dueDate,omitempty"`
	Items   []InvoiceItem `json:"items,omitempty"`
	// MerchantReference is stored on the transaction for reconciliation. When
	// supplied it must be a UUID.
	MerchantReference string            `json:"merchantReference,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// GetStatusInput selects a transaction for a one-shot status check.
type GetStatusInput struct {
	Reference string `json:"reference"`
}

// GetTransactionInput selects a transaction by id or reference. At least one is
// required.
type GetTransactionInput struct {
	ID        string `json:"id,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// VerifyPhoneInput pre-validates a phone number with the provider.
type VerifyPhoneInput struct {
	PhoneNumber string `json:"phoneNumber"`
	// Purpose is "collection" or "payout"; the provider may route differently.
	Purpose *string `json:"purpose,omitempty"`
}

// ListTransactionsInput filters a transaction listing. Every field is optional.
type ListTransactionsInput struct {
	// Tags uses AND semantics: only transactions carrying every listed tag match.
	Tags   []string          `json:"tags,omitempty"`
	Status TransactionStatus `json:"status,omitempty"`
	// Type is "collection", "payout" or "invoice".
	Type string `json:"type,omitempty"`
	// Limit is results per page, 1-100. Defaults to 20 server-side.
	Limit int `json:"limit,omitempty"`
	// Offset is the zero-based pagination offset.
	Offset int `json:"offset,omitempty"`
	// CreatedAfter and CreatedBefore are inclusive ISO 8601 datetimes.
	CreatedAfter  string `json:"createdAfter,omitempty"`
	CreatedBefore string `json:"createdBefore,omitempty"`
}

// TransactionSummary is the abbreviated record returned in a listing.
type TransactionSummary struct {
	ID        string            `json:"id"`
	Reference string            `json:"reference"`
	Amount    int64             `json:"amount"`
	Currency  Currency          `json:"currency"`
	Status    TransactionStatus `json:"status"`
	Type      TransactionType   `json:"type"`
	Method    *string           `json:"method"`
	Mode      TransactionMode   `json:"mode"`
	Tags      []string          `json:"tags"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// ListTransactionsResponse is a page of transactions plus the filters applied.
type ListTransactionsResponse struct {
	Transactions []TransactionSummary `json:"transactions"`
	// Count is the total number of matching transactions, for pagination.
	Count  int `json:"count"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	// Tags echoes the filter tags that were applied.
	Tags []string `json:"tags"`
}

// Transaction is the full record returned by GetTransaction, by the resolve
// variants, and on terminal lifecycle events.
type Transaction struct {
	ID          string            `json:"id"`
	Reference   string            `json:"reference"`
	Amount      int64             `json:"amount"`
	Currency    Currency          `json:"currency"`
	Status      TransactionStatus `json:"status"`
	Type        TransactionType   `json:"type"`
	Method      PaymentMethod     `json:"method"`
	Description string            `json:"description"`
	// Duplicate is true only when this response replayed an existing
	// transaction for a reused reference. No new payment was initiated.
	Duplicate *bool `json:"duplicate,omitempty"`
	// OperatorTid is the telco's or bank's own transaction id, what the paying
	// customer sees on their receipt. Null until the operator reports it,
	// typically at terminal status.
	OperatorTid *string `json:"operatorTid,omitempty"`
	// Phone is the normalized international form, 256XXXXXXXXX.
	Phone         string  `json:"phone"`
	Email         *string `json:"email,omitempty"`
	FailureReason *string `json:"failureReason,omitempty"`
	// StatusText is a plain-language description of the current status, for
	// example why a payout is on hold. Populated by the backend when available.
	StatusText *string           `json:"statusText,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Mode       TransactionMode   `json:"mode"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
	// Delayed is true when the payment has been non-terminal for longer than
	// about three minutes. It is a flag, not a status value.
	Delayed *bool `json:"delayed,omitempty"`
}

// StatusResponse is the result of a one-shot status check.
type StatusResponse struct {
	Reference string            `json:"reference"`
	Status    TransactionStatus `json:"status"`
	Amount    int64             `json:"amount"`
	Currency  Currency          `json:"currency"`
	// StatusText is a plain-language description of the current status.
	StatusText *string `json:"statusText,omitempty"`
	UpdatedAt  string  `json:"updatedAt"`
	// Delayed is true when the payment has been non-terminal for longer than
	// about three minutes. It is a flag, not a status value.
	Delayed *bool `json:"delayed,omitempty"`
}

// PhoneVerification is the provider's answer for a phone number.
type PhoneVerification struct {
	PhoneNumber  string `json:"phoneNumber"`
	CustomerName string `json:"customerName"`
	Verified     bool   `json:"verified"`
}

// InvoiceResponse describes a created hosted invoice. The customer receives an
// email containing PaymentLink.
type InvoiceResponse struct {
	ID            string `json:"id"`
	InvoiceNumber string `json:"invoiceNumber"`
	PaymentLink   string `json:"paymentLink"`
	// Amount is a decimal string here, matching the backend's wire JSON.
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
}

// VerifyWebhookInput is the raw material for webhook verification.
type VerifyWebhookInput struct {
	// Payload is the raw request body, hashed exactly as supplied. Do not parse
	// and re-serialize it first: that changes key order and whitespace, so a
	// genuine delivery would fail its own signature.
	Payload []byte
	// Signature is the x-nylon-signature header value.
	Signature string
	// Secret is the merchant's webhook secret, which is NOT the API secret.
	// Using the API secret here makes every webhook fail verification.
	Secret string
	// ToleranceSeconds is the replay window, defaulting to 300. Zero means zero
	// seconds of tolerance, the strictest setting, and does NOT disable the
	// check; pass DisableFreshnessCheck for that.
	ToleranceSeconds *int
}

// WebhookTransactionSnapshot is the transaction record inside a webhook body.
//
// Field names match the wire JSON exactly, because merchants unmarshal directly
// into this. Every key is always present: the backend sends an explicit null
// rather than omitting one, so the shape never changes. There is no StatusText
// here, the statuses it describes emit no webhook at all.
type WebhookTransactionSnapshot struct {
	TransactionID string `json:"transactionId"`
	Reference     string `json:"reference"`
	// Amount is a decimal string. Null only when the backend could not read the
	// transaction record while dispatching.
	Amount         *string           `json:"amount"`
	Currency       *string           `json:"currency"`
	Status         TransactionStatus `json:"status"`
	PreviousStatus TransactionStatus `json:"previousStatus"`
	Type           *TransactionType  `json:"type"`
	Method         *PaymentMethod    `json:"method"`
	Mode           *TransactionMode  `json:"mode"`
	FailureReason  *string           `json:"failureReason"`
	OperatorTid    *string           `json:"operatorTid"`
}

// WebhookPayload is the body of an inbound webhook delivery. Verify the raw
// bytes with VerifyWebhookSignature before unmarshalling into this.
type WebhookPayload struct {
	// DeliveryID is snake_case on the wire. Use it, or Reference, to dedupe:
	// delivery is at-least-once.
	DeliveryID string                     `json:"delivery_id"`
	Event      WebhookEventType           `json:"event"`
	Payload    WebhookTransactionSnapshot `json:"payload"`
	Timestamp  string                     `json:"timestamp"`
}

// EventData is handed to every PaymentInstance event handler.
type EventData struct {
	Event PaymentEvent
	// Reference is present on every event. Use it as the identifier on
	// "processing", where Transaction may not be populated yet.
	Reference string
	// Transaction is guaranteed only on the terminal events.
	Transaction *Transaction
	// Err, Category and Retryable are populated on the "error" event.
	Err       error
	Category  ErrorCategory
	Retryable bool
	// At is when the event was emitted.
	At time.Time
}

// PaymentEventHandler receives PaymentInstance lifecycle events. A panic inside a
// handler is contained: it stops neither the other handlers nor polling.
type PaymentEventHandler func(EventData)
