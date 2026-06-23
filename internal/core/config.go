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
}

type PaymentInstanceConfig struct {
	Reference        string
	InitialStatus    string
	FetchStatus      func(ctx context.Context, ref string) (string, error)
	FetchTransaction func(ctx context.Context, ref string) (*types.Transaction, error)
	PollInterval     time.Duration
	MaxPollDuration  time.Duration
	MaxPollAttempts  int
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
