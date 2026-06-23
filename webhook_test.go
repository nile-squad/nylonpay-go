package nylonpay_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	nylonpay "github.com/nile-squad/nylonpay-go"
)

func webhookClient(t *testing.T) *nylonpay.NylonPayClient {
	t.Helper()
	c, err := nylonpay.NewClient(nylonpay.Config{
		APIKey:    "npk_testkey",
		APISecret: "nps_testsecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func freshPayload() []byte {
	ts := time.Now().Unix()
	return fmt.Appendf(nil, `{"event":"success","reference":"ref001","timestamp":%d}`, ts)
}

// ── VerifyWebhookSignature ────────────────────────────────────────────────────

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	secret := "wh_secret123"
	payload := freshPayload()
	sig := signPayload(payload, secret)

	ok := webhookClient(t).VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:   string(payload),
		Signature: sig,
		Secret:    secret,
	})
	if !ok {
		t.Error("valid webhook must verify")
	}
}

func TestVerifyWebhookSignature_WrongSignature(t *testing.T) {
	secret := "wh_secret123"
	payload := freshPayload()

	ok := webhookClient(t).VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:   string(payload),
		Signature: "aabbccddeeff0011",
		Secret:    secret,
	})
	if ok {
		t.Error("wrong signature must not verify")
	}
}

func TestVerifyWebhookSignature_WrongSecret(t *testing.T) {
	payload := freshPayload()
	sig := signPayload(payload, "correct_secret")

	ok := webhookClient(t).VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:   string(payload),
		Signature: sig,
		Secret:    "wrong_secret",
	})
	if ok {
		t.Error("wrong secret must not verify")
	}
}

func TestVerifyWebhookSignature_ExpiredTimestamp(t *testing.T) {
	secret := "wh_secret123"
	past := time.Now().Add(-10 * time.Minute).Unix()
	payload := fmt.Appendf(nil, `{"event":"success","timestamp":%d}`, past)
	sig := signPayload(payload, secret)

	ok := webhookClient(t).VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:   string(payload),
		Signature: sig,
		Secret:    secret,
	})
	if ok {
		t.Error("expired webhook must not verify with default tolerance")
	}
}

func TestVerifyWebhookSignature_CustomTolerance(t *testing.T) {
	secret := "wh_secret123"
	// 2 minutes old — past default 300s but within a custom 600s window.
	past := time.Now().Add(-2 * time.Minute)
	payload := fmt.Appendf(nil, `{"event":"success","timestamp":%d}`, past.Unix())
	sig := signPayload(payload, secret)
	tol := 600 * time.Second

	ok := webhookClient(t).VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:          string(payload),
		Signature:        sig,
		Secret:           secret,
		ToleranceSeconds: &tol,
	})
	if !ok {
		t.Error("webhook within custom tolerance window must verify")
	}
}

func TestVerifyWebhookSignature_TamperedPayload(t *testing.T) {
	secret := "wh_secret123"
	payload := freshPayload()
	sig := signPayload(payload, secret)

	// Tamper with the payload after signing.
	tampered := fmt.Appendf(nil, `{"event":"success","reference":"EVIL","timestamp":%d}`, time.Now().Unix())

	ok := webhookClient(t).VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
		Payload:   string(tampered),
		Signature: sig,
		Secret:    secret,
	})
	if ok {
		t.Error("tampered payload must not verify against original signature")
	}
}
