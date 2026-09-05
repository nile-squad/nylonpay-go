package nylonpay

import (
	"github.com/nile-squad/nylonpay-go/internal/crypto"
	"github.com/nile-squad/nylonpay-go/types"
)

// VerifyWebhookSignature reports whether a webhook genuinely came from Nylon
// Pay and is recent enough not to be a replay. It never panics, on any input,
// and returns false for every kind of failure.
//
// It needs no client and no API credentials, only the webhook secret, so it can
// be called directly from an HTTP handler:
//
//	body, _ := io.ReadAll(r.Body)
//	if !nylonpay.VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
//	    Payload:   body,
//	    Signature: r.Header.Get("x-nylon-signature"),
//	    Secret:    os.Getenv("NYLONPAY_WEBHOOK_SECRET"),
//	}) {
//	    http.Error(w, "invalid signature", http.StatusUnauthorized)
//	    return
//	}
//
// Pass the exact bytes received, before any JSON parse or re-serialization:
// re-encoding changes key order and whitespace, so a genuine delivery would
// fail its own signature.
//
// The secret is the webhook secret configured on your API key, not the API
// secret used for requests. They are separate credentials.
func VerifyWebhookSignature(input types.VerifyWebhookInput) bool {
	return crypto.VerifyWebhookSignature(crypto.VerifyWebhookInput{
		Payload:          input.Payload,
		Signature:        input.Signature,
		Secret:           input.Secret,
		ToleranceSeconds: input.ToleranceSeconds,
	})
}

// VerifyWebhookSignature is the method form, for code that holds a Client
// interface. It delegates to the package-level function, which is the more
// convenient entry point.
func (c *NylonPayClient) VerifyWebhookSignature(input types.VerifyWebhookInput) bool {
	return VerifyWebhookSignature(input)
}
