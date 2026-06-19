package nylonpay

import (
	"fmt"

	"github.com/nile-squad/nylonpay-go/types"
)

func (c *NylonPayClient) runBeforeCollectHook(
	fn func(*types.CollectPaymentPayload) *types.CollectPaymentPayload,
	in *types.CollectPaymentPayload,
) (out *types.CollectPaymentPayload) {
	defer func() {
		if r := recover(); r != nil {
			out = in
			c.notifyHookError("beforeCollect", r)
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
	defer func() {
		if r := recover(); r != nil {
			c.notifyHookError("afterCollect", r)
		}
	}()
	fn(in, ref, status, err)
}

func (c *NylonPayClient) runBeforePayoutHook(
	fn func(*types.MakePayoutPayload) *types.MakePayoutPayload,
	in *types.MakePayoutPayload,
) (out *types.MakePayoutPayload) {
	defer func() {
		if r := recover(); r != nil {
			out = in
			c.notifyHookError("beforePayout", r)
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
	defer func() {
		if r := recover(); r != nil {
			c.notifyHookError("afterPayout", r)
		}
	}()
	fn(in, ref, status, err)
}

func (c *NylonPayClient) notifyHookError(hook string, r any) {
	if c.cfg.Hooks != nil && c.cfg.Hooks.OnError != nil {
		defer func() { recover() }() // guard against OnError itself panicking
		c.cfg.Hooks.OnError(hook, fmt.Errorf("hook panic: %v", r))
	}
}
