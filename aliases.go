package nylonpay

import (
	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/internal/crypto"
	"github.com/nile-squad/nylonpay-go/types"
)

// This file re-exports the public surface so merchants import one package.

// PaymentInstance tracks one transaction to a terminal state. See its methods
// for the event and waiting API.
type PaymentInstance = core.PaymentInstance

// SDKError is the structured error returned by every operation. Recover it with
// errors.As and branch on Category; never on the message text or an HTTP status.
type SDKError = types.SDKError

// ErrorCategory is the fixed taxonomy every SDKError is drawn from.
type ErrorCategory = types.ErrorCategory

// Error categories. These are the stable, machine-readable failure signals.
const (
	CategoryAuth       = types.CategoryAuth
	CategoryValidation = types.CategoryValidation
	CategoryLimit      = types.CategoryLimit
	CategoryRateLimit  = types.CategoryRateLimit
	CategoryAccount    = types.CategoryAccount
	CategoryProvider   = types.CategoryProvider
	CategoryDuplicate  = types.CategoryDuplicate
	CategoryNotFound   = types.CategoryNotFound
	CategoryInternal   = types.CategoryInternal
	CategoryNetwork    = types.CategoryNetwork
	CategoryTimeout    = types.CategoryTimeout
)

// DisableFreshnessCheck opts out of webhook replay protection entirely.
//
// Pass it deliberately. A ToleranceSeconds of 0 does NOT disable the check: it
// means zero seconds of tolerance, the strictest possible setting.
const DisableFreshnessCheck = crypto.DisableFreshnessCheck

// Operation inputs and results.
type (
	Customer                   = types.Customer
	Destination                = types.Destination
	InvoiceItem                = types.InvoiceItem
	BankDetails                = types.BankDetails
	CollectPaymentInput        = types.CollectPaymentInput
	MakePayoutInput            = types.MakePayoutInput
	CreateInvoiceInput         = types.CreateInvoiceInput
	GetStatusInput             = types.GetStatusInput
	GetTransactionInput        = types.GetTransactionInput
	VerifyPhoneInput           = types.VerifyPhoneInput
	ListTransactionsInput      = types.ListTransactionsInput
	ListTransactionsResponse   = types.ListTransactionsResponse
	TransactionSummary         = types.TransactionSummary
	Transaction                = types.Transaction
	StatusResponse             = types.StatusResponse
	PhoneVerification          = types.PhoneVerification
	InvoiceResponse            = types.InvoiceResponse
	VerifyWebhookInput         = types.VerifyWebhookInput
	WebhookPayload             = types.WebhookPayload
	WebhookTransactionSnapshot = types.WebhookTransactionSnapshot
)

// Domain enumerations.
type (
	TransactionStatus = types.TransactionStatus
	TransactionType   = types.TransactionType
	PaymentMethod     = types.PaymentMethod
	PaymentEvent      = types.PaymentEvent
	Currency          = types.Currency
	TransactionMode   = types.TransactionMode
	WebhookEventType  = types.WebhookEventType
	OnDelayedBehavior = types.OnDelayedBehavior
)

// PaymentInstance event payloads.
type (
	EventData           = types.EventData
	PaymentEventHandler = types.PaymentEventHandler
)

const (
	// Non-terminal statuses.
	TransactionStatusPending    = types.TransactionStatusPending
	TransactionStatusProcessing = types.TransactionStatusProcessing
	TransactionStatusOnHold     = types.TransactionStatusOnHold
	// Terminal statuses.
	TransactionStatusSuccessful = types.TransactionStatusSuccessful
	TransactionStatusFailed     = types.TransactionStatusFailed
	TransactionStatusCancelled  = types.TransactionStatusCancelled

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

	// PaymentInstance lifecycle events. Each fires at most once per instance.
	PaymentEventProcessing = types.PaymentEventProcessing
	PaymentEventSuccess    = types.PaymentEventSuccess
	PaymentEventFailed     = types.PaymentEventFailed
	PaymentEventCancelled  = types.PaymentEventCancelled
	PaymentEventError      = types.PaymentEventError

	// Webhook event types.
	WebhookEventSuccessful = types.WebhookEventSuccessful
	WebhookEventFailed     = types.WebhookEventFailed
	WebhookEventProcessing = types.WebhookEventProcessing
	WebhookEventCancelled  = types.WebhookEventCancelled

	// OnDelayedWait keeps polling a delayed payment; OnDelayedReturn hands back
	// the still-pending record.
	OnDelayedWait   = types.OnDelayedWait
	OnDelayedReturn = types.OnDelayedReturn

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
