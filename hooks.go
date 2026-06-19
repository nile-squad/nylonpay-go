package nylonpay

import "github.com/nile-squad/nylonpay-go/types"

func (c *NylonPayClient) runBeforeCollectHook(
	fn func(*types.CollectPaymentPayload) *types.CollectPaymentPayload,
	in *types.CollectPaymentPayload,
) (out *types.CollectPaymentPayload) {
	defer func() {
		if recover() != nil {
			out = in
		}
	}()
	return fn(in)
}

func (c *NylonPayClient) runAfterCollectHook(
	fn func(*types.CollectPaymentPayload, string, string, error),
	in *types.CollectPaymentPayload,
	ref, status string,
	err error,
) {
	defer func() { recover() }()
	fn(in, ref, status, err)
}

func (c *NylonPayClient) runBeforePayoutHook(
	fn func(*types.MakePayoutPayload) *types.MakePayoutPayload,
	in *types.MakePayoutPayload,
) (out *types.MakePayoutPayload) {
	defer func() {
		if recover() != nil {
			out = in
		}
	}()
	return fn(in)
}

func (c *NylonPayClient) runAfterPayoutHook(
	fn func(*types.MakePayoutPayload, string, string, error),
	in *types.MakePayoutPayload,
	ref, status string,
	err error,
) {
	defer func() { recover() }()
	fn(in, ref, status, err)
}
