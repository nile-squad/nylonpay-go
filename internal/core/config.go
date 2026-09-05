package core

import (
	"context"
	"net/http"
	"time"

	"github.com/nile-squad/nylonpay-go/types"
)

type TransportConfig struct {
	APIKey     string
	APISecret  string
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
	HTTPClient *http.Client
	// MaxResponseBytes caps the response body size. Zero means MaxResponseBytes
	// (10 MB).
	MaxResponseBytes int64
}

// PaymentInstanceConfig wires a PaymentInstance to the operations it polls
// with. It is internal: merchants receive a fully constructed instance.
type PaymentInstanceConfig struct {
	Reference     string
	InitialStatus types.TransactionStatus
	// FetchStatus performs a one-shot status check.
	FetchStatus func(ctx context.Context, reference string) (*types.StatusResponse, error)
	// FetchTransaction retrieves the full record once a terminal state is seen.
	FetchTransaction func(ctx context.Context, reference string) (*types.Transaction, error)
	// PollInterval is the base gap between polls; zero means DefaultPollInterval.
	PollInterval time.Duration
	// MaxPollDuration and MaxPollAttempts are optional caps. Zero means poll
	// until the transaction reaches a terminal state.
	MaxPollDuration time.Duration
	MaxPollAttempts int
	OnDelayed       types.OnDelayedBehavior
	// InitialError, when set, means the backend rejected initiation. There is
	// nothing to poll, so the instance emits it as an "error" event instead.
	InitialError error
	// Context bounds the polling lifetime. Nil means context.Background.
	Context context.Context
}

type Transport struct {
	config TransportConfig
}

func NewTransport(cfg TransportConfig) *Transport {
	if cfg.BaseURL == "" {
		cfg.BaseURL = BASE_URL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = TIMEOUT
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: cfg.Timeout * 2,
		}
	}
	return &Transport{config: cfg}
}
