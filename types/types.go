package types

import "time"

type TransactionStatus string
type TransactionType string
type PaymentMethod string
type PaymentEvent string
type Currency string
type TransactionMode string

// InstanceEventType identifies a lifecycle event emitted by a PaymentInstance.
type InstanceEventType string

const (
	TransactionStatusPending    TransactionStatus = "pending"
	TransactionStatusSuccessful TransactionStatus = "successful"
	TransactionStatusFailed     TransactionStatus = "failed"
	TransactionStatusCancelled  TransactionStatus = "cancelled"
	TransactionStatusProcessing TransactionStatus = "processing"

	TransactionTypePayout     TransactionType = "payout"
	TransactionTypeCollection TransactionType = "collection"
	TransactionTypeTransfer   TransactionType = "transfer"
	TransactionTypeEscrow     TransactionType = "escrow"
	TransactionTypeRefund     TransactionType = "refund"
	TransactionTypeReversal   TransactionType = "reversal"
	TransactionTypeCharge     TransactionType = "charge"
	TransactionTypeChargeBack TransactionType = "chargeback"

	PaymentMethodBank        PaymentMethod = "bank"
	PaymentMethodMobileMoney PaymentMethod = "mobile_money"

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

	// PaymentInstance lifecycle events.
	InstanceEventProcessing InstanceEventType = "processing"
	InstanceEventSuccess    InstanceEventType = "success"
	InstanceEventFailed     InstanceEventType = "failed"
	InstanceEventCancelled  InstanceEventType = "cancelled"
	InstanceEventError      InstanceEventType = "error"
)

type Customer struct {
	Name        string
	PhoneNumber string
	Email       *string
}

type Destination struct {
	AccountHolderName string
	AccountNumber     string
	BankName          *string
	Phone             *string
}

type InvoiceItem struct {
	Name      string
	Quantity  int
	UnitPrice int64
}

type BankDetails struct {
	AccountNumber string
	BankName      string
}

type CollectPaymentPayload struct {
	Amount      int64
	Currency    Currency
	Customer    Customer
	Description string
	Reference   string
	Method      *PaymentMethod
	Bank        *BankDetails
	MetaData    *map[string]string
}

type MakePayoutPayload struct {
	Amount      int64
	Currency    Currency
	Customer    Customer
	Destination Destination
	Description string
	Reference   string
	MetaData    *map[string]string
}

type CreateInvoicePayload struct {
	Amount      int64
	Currency    Currency
	Description string
	Items       *[]InvoiceItem
	RedirectURL *string
	Reference   string
	MetaData    *map[string]string
}

type Transaction struct {
	ID            string
	Reference     string
	Amount        int64
	Currency      Currency
	Status        TransactionStatus
	Type          TransactionType
	Method        PaymentMethod
	Description   string
	Duplicate     *bool
	OperatorTid   *string
	Phone         string
	Email         *string
	FailureReason *string
	Metadata      *map[string]string
	Mode          TransactionMode
	CreatedAt     string
	UpdatedAt     string
}

type StatusResponse struct {
	Reference string
	Status    TransactionStatus
	Amount    int64
	Currency  Currency
	UpdatedAt string
}

type GetTransactionInput struct {
	ID        string
	Reference string
}

type PhoneVerification struct {
	PhoneNumber  string
	CustomerName string
	Verified     bool
}

type InvoiceResponse struct {
	ID        string
	Url       string
	Token     string
	ExpiresAt string
	Status    string
}

type VerifyWebhookInput struct {
	Payload          string
	Signature        string
	Secret           string
	ToleranceSeconds *time.Duration
}

// InstanceEventData is delivered to every handler registered via On or Once.
type InstanceEventData struct {
	Event       InstanceEventType
	Reference   string
	Transaction *Transaction // non-nil on terminal (success/failed/cancelled) events
	Err         error        // non-nil on failed and error events
	At          time.Time    // UTC timestamp; format with time.RFC3339 for ISO 8601
}

// InstanceHandler is the callback type for PaymentInstance event listeners.
type InstanceHandler func(InstanceEventData)
