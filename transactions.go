package nylonpay

import (
	"context"

	"github.com/nile-squad/nylonpay-go/internal/core"
	"github.com/nile-squad/nylonpay-go/types"
)

// ListTransactions returns a page of transactions for the account, with
// optional filters.
//
// Tag filtering uses AND semantics: passing two tags returns only transactions
// carrying both, not either.
func (c *NylonPayClient) ListTransactions(ctx context.Context, input types.ListTransactionsInput) (*types.ListTransactionsResponse, error) {
	if input.Limit < 0 {
		return nil, validationErr("limit must not be negative")
	}
	if input.Limit > 100 {
		return nil, validationErr("limit must not exceed 100")
	}
	if input.Offset < 0 {
		return nil, validationErr("offset must not be negative")
	}

	var listing types.ListTransactionsResponse
	if err := c.transport.Send(ctx, core.TransportRequest{
		Action:  core.ActionListTransactions,
		Payload: input,
	}, &listing); err != nil {
		return nil, err
	}
	return &listing, nil
}

// GetTransactionsByTag lists transactions carrying a single tag. It is
// shorthand for ListTransactions with that one tag applied, and accepts the
// same options; any Tags already on input are replaced.
func (c *NylonPayClient) GetTransactionsByTag(ctx context.Context, tag string, input types.ListTransactionsInput) (*types.ListTransactionsResponse, error) {
	if err := requireNonEmpty(tag, "tag"); err != nil {
		return nil, err
	}

	input.Tags = []string{tag}
	return c.ListTransactions(ctx, input)
}
