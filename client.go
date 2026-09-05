package nylonpay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// Client is the full set of merchant operations. NylonPayClient implements it;
// take this interface in your own code if you want to substitute a fake.
type Client interface {
	CollectPayment(ctx context.Context, input types.CollectPaymentInput) (*core.PaymentInstance, error)
	CollectPaymentAndResolve(ctx context.Context, input types.CollectPaymentInput) (*types.Transaction, error)
	MakePayout(ctx context.Context, input types.MakePayoutInput) (*core.PaymentInstance, error)
	MakePayoutAndResolve(ctx context.Context, input types.MakePayoutInput) (*types.Transaction, error)
	GetStatus(ctx context.Context, input types.GetStatusInput) (*types.StatusResponse, error)
	GetTransaction(ctx context.Context, input types.GetTransactionInput) (*types.Transaction, error)
	ListTransactions(ctx context.Context, input types.ListTransactionsInput) (*types.ListTransactionsResponse, error)
	GetTransactionsByTag(ctx context.Context, tag string, input types.ListTransactionsInput) (*types.ListTransactionsResponse, error)
	VerifyPhone(ctx context.Context, input types.VerifyPhoneInput) (*types.PhoneVerification, error)
	CreateInvoice(ctx context.Context, input types.CreateInvoiceInput) (*types.InvoiceResponse, error)
	VerifyWebhookSignature(input types.VerifyWebhookInput) bool
}

// Config configures a client. Only APIKey and APISecret are required.
//
// Test versus live mode follows from the API key, not from anything here: a
// sandbox key routes through test providers and moves no real money. There is
// deliberately no environment option.
type Config struct {
	// APIKey must start with "npk_".
	APIKey string
	// APISecret must start with "nps_". It is used only to compute HMACs and is
	// never sent in a request body, a query parameter, or a log line.
	APISecret string
	// BaseURL overrides the endpoint. It is a complete URL including the path;
	// the SDK appends nothing to it. Defaults to the production endpoint.
	BaseURL string
	// Timeout bounds a single HTTP attempt. Defaults to 30s.
	Timeout time.Duration
	// MaxRetries is the number of retries after the first attempt. Defaults to
	// 3, so up to 4 attempts in total. Zero means "unset", so pass a negative
	// value to disable retries outright.
	MaxRetries int
	// MaxPollInterval is the base gap between status polls, defaulting to 2s.
	// Despite the name (kept for cross-language consistency) it is the starting
	// interval, not a ceiling: each poll adds jitter, and after two minutes the
	// interval doubles every two minutes up to a hard 15s cap.
	MaxPollInterval time.Duration
	// MaxPollDuration optionally caps how long a PaymentInstance polls. Zero
	// means poll until the transaction reaches a terminal state.
	MaxPollDuration time.Duration
	// MaxPollAttempts optionally caps how many status polls are made. Zero
	// means poll until terminal.
	MaxPollAttempts int
	// OnDelayed decides what happens once a payment is flagged delayed, after
	// roughly three minutes in flight. Defaults to OnDelayedWait.
	OnDelayed types.OnDelayedBehavior
	// HTTPClient substitutes the HTTP client, for tests or custom transports.
	HTTPClient *http.Client
	// MaxResponseBytes caps the response body size. Zero means 10 MB.
	MaxResponseBytes int64
	// Force returns a brand new client even when a cached one exists for this
	// key, secret and base URL.
	Force bool
	// Hooks observe or enrich payment calls. See Hooks.
	Hooks *Hooks
}

// Hook wraps a merchant callback so it can never crash a payment.
//
// Fn runs inside a recovery boundary. A panic is routed to OnError and the call
// proceeds; for a before* hook it proceeds with the original, unmutated input.
// OnError is itself contained, so a faulty error handler cannot bring anything
// down either.
type Hook[T any] struct {
	// Enabled toggles the hook without removing its configuration. Nil means
	// enabled.
	Enabled *bool
	// Fn is the handler.
	Fn T
	// OnError receives any panic from Fn. It is required: a hook failure must
	// be neither swallowed nor allowed to fail the payment.
	OnError func(error)
}

// isEnabled reports whether the hook should run.
func (h *Hook[T]) isEnabled() bool {
	return h != nil && (h.Enabled == nil || *h.Enabled)
}

// HookResult is the outcome handed to an after* hook. Err is nil on success.
type HookResult struct {
	Reference string
	Status    string
	Err       error
}

// AfterHookInput carries both views of the input to an after* hook.
type AfterHookInput[T any] struct {
	// Input is the final wire payload: reference resolved, phone normalized,
	// and any before* hook mutation applied.
	Input T
	// Raw is the untouched original merchant input.
	Raw T
}

// Hooks are registered once at construction. Each lifecycle point takes at most
// one hook.
//
// A before* hook may return a replacement input; returning nil leaves the input
// unchanged. Whatever it returns is re-run through the full validation and
// normalization suite before it is sent, so a hook can no more smuggle a bad
// reference or a sub-minimum amount past the checks than the original caller.
//
// An after* hook runs after the transport call whether it succeeded or failed.
// Neither kind ever receives secrets, provider payloads, or transport internals.
type Hooks struct {
	BeforeCollect *Hook[func(types.CollectPaymentInput) *types.CollectPaymentInput]
	AfterCollect  *Hook[func(HookResult, AfterHookInput[types.CollectPaymentInput])]
	BeforePayout  *Hook[func(types.MakePayoutInput) *types.MakePayoutInput]
	AfterPayout   *Hook[func(HookResult, AfterHookInput[types.MakePayoutInput])]
}

// NylonPayClient is a ready-to-use SDK client. It holds no connection state
// between calls and is safe for concurrent use.
type NylonPayClient struct {
	cfg       Config
	transport *core.Transport
}

var _ Client = (*NylonPayClient)(nil)

// instances caches one client per key + secret + base URL so repeated calls in
// a long-running process do not pile up. The mutex is required, not defensive:
// Go programs call this from multiple goroutines, and an unguarded map would
// both duplicate instances and corrupt itself.
var (
	instancesMu sync.Mutex
	instances   = make(map[string]*NylonPayClient)
)

// instanceKey identifies a cached client. The secret is hashed rather than
// stored, so it never sits in a map key, and including it means rotating the
// secret hands back a fresh client instead of one still signing with the old one.
func instanceKey(apiKey, apiSecret, baseURL string) string {
	sum := sha256.Sum256([]byte(apiSecret))
	return apiKey + ":" + baseURL + ":" + hex.EncodeToString(sum[:])[:16]
}

// NewClient validates cfg and returns a client ready to use.
//
// Configuration is validated eagerly: a client with missing or malformed
// credentials cannot be constructed, and the error is returned before any
// network call. Calling NewClient again with the same key, secret and base URL
// returns the same client unless Config.Force is set.
func NewClient(cfg Config) (*NylonPayClient, error) {
	if cfg.APIKey == "" {
		return nil, &types.SDKError{Category: types.CategoryValidation, Message: "apiKey is required"}
	}
	if !strings.HasPrefix(cfg.APIKey, "npk_") {
		return nil, &types.SDKError{Category: types.CategoryValidation, Message: `apiKey must start with "npk_"`}
	}
	if cfg.APISecret == "" {
		return nil, &types.SDKError{Category: types.CategoryValidation, Message: "apiSecret is required"}
	}
	if !strings.HasPrefix(cfg.APISecret, "nps_") {
		return nil, &types.SDKError{Category: types.CategoryValidation, Message: `apiSecret must start with "nps_"`}
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = core.TIMEOUT
	}
	switch {
	case cfg.MaxRetries == 0:
		cfg.MaxRetries = core.MAX_RETRIES
	case cfg.MaxRetries < 0:
		// Zero is indistinguishable from "unset" for an int field, so a
		// negative value is how a caller says "do not retry at all".
		cfg.MaxRetries = 0
	}
	if cfg.MaxPollInterval == 0 {
		cfg.MaxPollInterval = core.DefaultPollInterval
	}
	if cfg.OnDelayed == "" {
		cfg.OnDelayed = types.OnDelayedWait
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = core.BASE_URL
	}

	key := instanceKey(cfg.APIKey, cfg.APISecret, cfg.BaseURL)

	instancesMu.Lock()
	defer instancesMu.Unlock()

	if !cfg.Force {
		if existing, ok := instances[key]; ok {
			return existing, nil
		}
	}

	client := &NylonPayClient{
		cfg: cfg,
		transport: core.NewTransport(core.TransportConfig{
			APIKey:           cfg.APIKey,
			APISecret:        cfg.APISecret,
			BaseURL:          cfg.BaseURL,
			Timeout:          cfg.Timeout,
			MaxRetries:       cfg.MaxRetries,
			HTTPClient:       cfg.HTTPClient,
			MaxResponseBytes: cfg.MaxResponseBytes,
		}),
	}

	instances[key] = client
	return client, nil
}
