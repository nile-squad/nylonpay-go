package nylonpay

import (
	"github.com/nile-squad/nylonpay-go/internal/crypto"
	"github.com/nile-squad/nylonpay-go/types"
)

func (c *NylonPayClient) VerifyWebhookSignature(input types.VerifyWebhookInput) bool {
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
