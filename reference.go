package nylonpay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

const (
	referenceMinLength = 13
	referenceMaxLength = 15
)

func (c *NylonPayClient) resolveReference(ref string) (string, error) {
	if ref == "" {
		return generateReference(), nil
	}
	if len(ref) < referenceMinLength || len(ref) > referenceMaxLength {
		return "", &core.SDKError{
			Category: "validation",
			Message:  fmt.Sprintf("reference must be %d–%d characters", referenceMinLength, referenceMaxLength),
		}
	}
	return ref, nil
}

func generateReference() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:referenceMaxLength]
}

func (c *NylonPayClient) rawFetchStatus(ctx context.Context, ref string) (string, error) {
	resp, err := c.GetStatus(ctx, ref)
	if err != nil {
		return "", err
	}
	return string(resp.Status), nil
}

func (c *NylonPayClient) rawFetchTransaction(ctx context.Context, ref string) (*types.Transaction, error) {
	return c.GetTransaction(ctx, types.GetTransactionInput{Reference: ref})
}
