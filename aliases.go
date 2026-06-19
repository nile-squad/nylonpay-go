package nylonpay

// Type aliases and constant re-exports from the types subpackage.
// Consumers only need to import this root package — no separate types import required.

import "github.com/nile-squad/nylonpay-go/types"

type (
	Customer              = types.Customer
	Destination           = types.Destination
	InvoiceItem           = types.InvoiceItem
	BankDetails           = types.BankDetails
	CollectPaymentPayload = types.CollectPaymentPayload
	MakePayoutPayload     = types.MakePayoutPayload
	CreateInvoicePayload  = types.CreateInvoicePayload
	Transaction           = types.Transaction
	StatusResponse        = types.StatusResponse
	GetTransactionInput   = types.GetTransactionInput
	PhoneVerification     = types.PhoneVerification
	InvoiceResponse       = types.InvoiceResponse
	VerifyWebhookInput    = types.VerifyWebhookInput
	TransactionStatus     = types.TransactionStatus
	TransactionType       = types.TransactionType
	PaymentMethod         = types.PaymentMethod
	PaymentEvent          = types.PaymentEvent
	Currency              = types.Currency
	TransactionMode       = types.TransactionMode
)

const (
	TransactionStatusPending    = types.TransactionStatusPending
	TransactionStatusSuccessful = types.TransactionStatusSuccessful
	TransactionStatusFailed     = types.TransactionStatusFailed
	TransactionStatusCancelled  = types.TransactionStatusCancelled
	TransactionStatusProcessing = types.TransactionStatusProcessing

	TransactionTypePayout     = types.TransactionTypePayout
	TransactionTypeCollection = types.TransactionTypeCollection
	TransactionTypeTransfer   = types.TransactionTypeTransfer
	TransactionTypeEscrow     = types.TransactionTypeEscrow
	TransactionTypeRefund     = types.TransactionTypeRefund
	TransactionTypeReversal   = types.TransactionTypeReversal
	TransactionTypeCharge     = types.TransactionTypeCharge
	TransactionTypeChargeBack = types.TransactionTypeChargeBack

	PaymentMethodBank        = types.PaymentMethodBank
	PaymentMethodMobileMoney = types.PaymentMethodMobileMoney

	PaymentEventSuccess    = types.PaymentEventSuccess
	PaymentEventFailed     = types.PaymentEventFailed
	PaymentEventCancelled  = types.PaymentEventCancelled
	PaymentEventProcessing = types.PaymentEventProcessing
	PaymentEventError      = types.PaymentEventError

	USD = types.USD
	EUR = types.EUR
	GBP = types.GBP
	KES = types.KES
	UGX = types.UGX
	TZS = types.TZS
	RWF = types.RWF

	TransactionModeLive = types.TransactionModeLive
	TransactionModeTest = types.TransactionModeTest
)
