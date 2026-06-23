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

	InstanceEventProcessing InstanceEventType = "processing"
	InstanceEventSuccess    InstanceEventType = "success"
	InstanceEventFailed     InstanceEventType = "failed"
	InstanceEventCancelled  InstanceEventType = "cancelled"
	InstanceEventError      InstanceEventType = "error"
)

type Customer struct {
	Name        string  `json:"name"`
	PhoneNumber string  `json:"phoneNumber"`
	Email       *string `json:"email,omitempty"`
}

type Destination struct {
	AccountHolderName string  `json:"accountHolderName"`
	AccountNumber     string  `json:"accountNumber"`
	BankName          *string `json:"bankName,omitempty"`
	Phone             *string `json:"phone,omitempty"`
}

type InvoiceItem struct {
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice int64  `json:"unitPrice"`
}

type BankDetails struct {
	AccountNumber string `json:"accountNumber"`
	BankName      string `json:"bankName"`
}

type CollectPaymentPayload struct {
	Amount      int64              `json:"amount"`
	Currency    Currency           `json:"currency,omitempty"`
	Customer    Customer           `json:"customer"`
	Description string             `json:"description"`
	Reference   string             `json:"reference,omitempty"`
	Method      *PaymentMethod     `json:"method,omitempty"`
	Bank        *BankDetails       `json:"bank,omitempty"`
	MetaData    *map[string]string `json:"metadata,omitempty"`
}

type MakePayoutPayload struct {
	Amount      int64              `json:"amount"`
	Currency    Currency           `json:"currency,omitempty"`
	Customer    Customer           `json:"customer"`
	Destination Destination        `json:"destination"`
	Description string             `json:"description"`
	Reference   string             `json:"reference,omitempty"`
	MetaData    *map[string]string `json:"metadata,omitempty"`
}

type CreateInvoicePayload struct {
	Amount      int64              `json:"amount"`
	Currency    Currency           `json:"currency,omitempty"`
	Description string             `json:"description"`
	Items       *[]InvoiceItem     `json:"items,omitempty"`
	RedirectURL *string            `json:"redirectUrl,omitempty"`
	Reference   string             `json:"reference,omitempty"`
	MetaData    *map[string]string `json:"metadata,omitempty"`
}

type Transaction struct {
	ID            string             `json:"id"`
	Reference     string             `json:"reference"`
	Amount        int64              `json:"amount"`
	Currency      Currency           `json:"currency"`
	Status        TransactionStatus  `json:"status"`
	Type          TransactionType    `json:"type"`
	Method        PaymentMethod      `json:"method"`
	Description   string             `json:"description"`
	Duplicate     *bool              `json:"duplicate,omitempty"`
	OperatorTid   *string            `json:"operatorTid,omitempty"`
	Phone         string             `json:"phone"`
	Email         *string            `json:"email,omitempty"`
	FailureReason *string            `json:"failureReason,omitempty"`
	Metadata      *map[string]string `json:"metadata,omitempty"`
	Mode          TransactionMode    `json:"mode"`
	CreatedAt     string             `json:"createdAt"`
	UpdatedAt     string             `json:"updatedAt"`
}

type StatusResponse struct {
	Reference string            `json:"reference"`
	Status    TransactionStatus `json:"status"`
	Amount    int64             `json:"amount"`
	Currency  Currency          `json:"currency"`
	UpdatedAt string            `json:"updatedAt"`
}

type GetTransactionInput struct {
	ID        string `json:"id,omitempty"`
	Reference string `json:"reference,omitempty"`
}

type PhoneVerification struct {
	PhoneNumber  string `json:"phoneNumber"`
	CustomerName string `json:"customerName"`
	Verified     bool   `json:"verified"`
}

type InvoiceResponse struct {
	ID        string `json:"id"`
	Url       string `json:"url"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
	Status    string `json:"status"`
}

type VerifyWebhookInput struct {
	Payload          string
	Signature        string
	Secret           string
	ToleranceSeconds *time.Duration
}

type InstanceEventData struct {
	Event       InstanceEventType
	Reference   string
	Transaction *Transaction
	Err         error
	At          time.Time
}

type InstanceHandler func(InstanceEventData)
