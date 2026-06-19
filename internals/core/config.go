package core

import (
	"net/http"
	"time"
)

const (
	BASE_URL    = "https://api.nylonpay.com"
	TIMEOUT     = 10 * time.Second
	MAX_RETRIES = 3
	SDKService  = "nylon_sdk"
)

type TransportConfig struct {
	APIKey     string
	APISecret  string
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
	HTTPClient *http.Client
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
	return &Transport{
		config: cfg,
	}
}
