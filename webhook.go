package nylonpay

import (
	"github.com/nile-squad/nylonpay-go/internal/crypto"
	"github.com/nile-squad/nylonpay-go/types"
)

// VerifyWebhook validates the HMAC-SHA256 signature on an inbound webhook and
// checks the embedded timestamp is within the replay-prevention window.
func (c *NylonPayClient) VerifyWebhook(input types.VerifyWebhookInput) bool {
	var tol *int
	if input.ToleranceSeconds != nil {
		seconds := int(input.ToleranceSeconds.Seconds())
		tol = &seconds
	}
	return crypto.VerifyWebhookSignature(crypto.VerifyWebhookInput{
		Payload:          []byte(input.Payload),
		Signature:        input.Signature,
		Secret:           input.Secret,
		ToleranceSeconds: tol,
	})
}
