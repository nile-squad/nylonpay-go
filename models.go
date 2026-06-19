package main

type TransactionStatus string
type TransactionType string
type PaymentMethod string
type PaymentEvent string
type Currency string

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
	UnitPrice float64
}

type BankDetails struct {
	AccountNumber string
	BankName      string
}

type CollectPaymentPayload struct {
	Amount      float64
	Currency    Currency
	Customer    Customer
	Description string
	Reference   *string
	Method      *PaymentMethod
	Bank        *BankDetails
	MetaData    *map[string]string
}

type MakePayoutPayload struct {
	Amount      float64
	Currency    Currency
	Customer    Customer
	Destination Destination
	Description string
	Reference   *string
	MetaData    *map[string]string
}

type CreateInvoicePayload struct {
	Amount      float64
	Currency    Currency
	Description string
	Items       *[]InvoiceItem
	RedirectURL *string
	Reference   *string
	MetaData    *map[string]string
}
