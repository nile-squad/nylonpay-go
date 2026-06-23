package nylonpay

import (
	"context"
	"strings"
	"time"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// Client is the interface satisfied by NylonPayClient.
type Client interface {
	CollectPayment(ctx context.Context, input types.CollectPaymentPayload) (*core.PaymentInstance, error)
	CollectPaymentAndResolve(ctx context.Context, input types.CollectPaymentPayload) (*types.Transaction, error)
	MakePayout(ctx context.Context, input types.MakePayoutPayload) (*core.PaymentInstance, error)
	MakePayoutAndResolve(ctx context.Context, input types.MakePayoutPayload) (*types.Transaction, error)
	GetStatus(ctx context.Context, reference string) (*types.StatusResponse, error)
	GetTransaction(ctx context.Context, input types.GetTransactionInput) (*types.Transaction, error)
	VerifyPhone(ctx context.Context, phoneNumber string) (*types.PhoneVerification, error)
	CreateInvoice(ctx context.Context, input types.CreateInvoicePayload) (*types.InvoiceResponse, error)
	VerifyWebhookSignature(input types.VerifyWebhookInput) bool
}

type Config struct {
	APIKey          string
	APISecret       string
	BaseURL         string
	Timeout         time.Duration
	MaxRetries      int
	MaxPollInterval time.Duration
	MaxPollDuration time.Duration
	MaxPollAttempts int
	Hooks           *Hooks
}

type Hooks struct {
	BeforeCollect func(*types.CollectPaymentPayload) *types.CollectPaymentPayload
	AfterCollect  func(*types.CollectPaymentPayload, string, string, error)
	BeforePayout  func(*types.MakePayoutPayload) *types.MakePayoutPayload
	AfterPayout   func(*types.MakePayoutPayload, string, string, error)
	// OnError is called whenever a hook panics, receiving the hook name and
	// recovered value as an error. Optional but strongly recommended.
	OnError func(hook string, err error)
}

type NylonPayClient struct {
	cfg       Config
	transport *core.Transport
}

var _ Client = (*NylonPayClient)(nil)

// NewClient validates cfg and constructs a client ready to use.
func NewClient(cfg Config) (*NylonPayClient, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = core.MAX_RETRIES
	}
	if cfg.APIKey == "" {
		return nil, &core.SDKError{Category: "validation", Message: "apiKey is required"}
	}
	if !strings.HasPrefix(cfg.APIKey, "npk_") {
		return nil, &core.SDKError{Category: "validation", Message: `apiKey must start with "npk_"`}
	}
	if cfg.APISecret == "" {
		return nil, &core.SDKError{Category: "validation", Message: "apiSecret is required"}
	}
	if !strings.HasPrefix(cfg.APISecret, "nps_") {
		return nil, &core.SDKError{Category: "validation", Message: `apiSecret must start with "nps_"`}
	}

	transport := core.NewTransport(core.TransportConfig{
		APIKey:     cfg.APIKey,
		APISecret:  cfg.APISecret,
		BaseURL:    cfg.BaseURL,
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
	})

	return &NylonPayClient{cfg: cfg, transport: transport}, nil
}
