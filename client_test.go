package nylonpay_test

import (
	"errors"
	"testing"
	"time"

	nylonpay "github.com/nile-squad/nylonpay-go"
	"github.com/nile-squad/nylonpay-go/internal/core"
)

func assertSDKError(t *testing.T, err error, wantCategory string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sdkErr *core.SDKError
	if !errors.As(err, &sdkErr) {
		t.Fatalf("expected *core.SDKError, got %T: %v", err, err)
	}
	if sdkErr.Category != wantCategory {
		t.Errorf("category = %q, want %q (message: %s)", sdkErr.Category, wantCategory, sdkErr.Message)
	}
}

// ── NewClient config validation ───────────────────────────────────────────────

func TestNewClient_MissingAPIKey(t *testing.T) {
	_, err := nylonpay.NewClient(nylonpay.Config{
		APISecret: "nps_secret",
	})
	assertSDKError(t, err, "validation")
}

func TestNewClient_WrongAPIKeyPrefix(t *testing.T) {
	_, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "wrong_prefix_key",
		APISecret: "nps_secret",
	})
	assertSDKError(t, err, "validation")
}

func TestNewClient_MissingAPISecret(t *testing.T) {
	_, err := nylonpay.NewClient(nylonpay.Config{
		APIKey: "npk_valid",
	})
	assertSDKError(t, err, "validation")
}

func TestNewClient_WrongAPISecretPrefix(t *testing.T) {
	_, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_valid",
		APISecret: "wrong_secret",
	})
	assertSDKError(t, err, "validation")
}

func TestNewClient_ValidConfig(t *testing.T) {
	c, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	// Supply zero — must default to 30s without error.
	_, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
		Timeout:   0,
	})
	if err != nil {
		t.Fatalf("zero Timeout must default gracefully, got: %v", err)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	_, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
		Timeout:   60 * time.Second,
	})
	if err != nil {
		t.Fatalf("custom timeout must not cause an error: %v", err)
	}
}

func TestNewClient_ImplementsClientInterface(t *testing.T) {
	c, _ := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
	})
	// Compile-time check is already in client.go; this confirms runtime.
	var _ nylonpay.Client = c
}
