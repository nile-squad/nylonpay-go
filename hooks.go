package nylonpay

import (
	"fmt"

	"github.com/nile-squad/nylonpay-go/types"
)

// hooks returns the configured hooks, or an empty set, so call sites do not
// each have to nil-check.
func (c *NylonPayClient) hooks() *Hooks {
	if c.cfg.Hooks == nil {
		return &Hooks{}
	}
	return c.cfg.Hooks
}

// reportHookError hands a recovered panic to the hook's own error handler.
//
// The handler is itself wrapped: a merchant whose OnError panics gets silence
// rather than a crashed payment. There is nowhere better to send that second
// failure, and losing it is strictly preferable to losing the payment.
func reportHookError(onError func(error), hookName string, recovered any) {
	if onError == nil {
		return
	}
	defer func() { _ = recover() }()
	onError(fmt.Errorf("%s hook failed: %v", hookName, recovered))
}

// runBeforeCollect applies the beforeCollect hook, if any.
//
// A nil return leaves the input untouched, and so does a panic. Only a non-nil
// return replaces it. The caller re-validates whatever comes back.
func (c *NylonPayClient) runBeforeCollect(input types.CollectPaymentInput) types.CollectPaymentInput {
	hook := c.hooks().BeforeCollect
	if !hook.isEnabled() || hook.Fn == nil {
		return input
	}

	var mutated *types.CollectPaymentInput
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				mutated = nil
				reportHookError(hook.OnError, "beforeCollect", recovered)
			}
		}()
		mutated = hook.Fn(input)
	}()

	if mutated == nil {
		return input
	}
	return *mutated
}

// runAfterCollect fires the afterCollect hook, on success and on failure alike.
func (c *NylonPayClient) runAfterCollect(result HookResult, sent, raw types.CollectPaymentInput) {
	hook := c.hooks().AfterCollect
	if !hook.isEnabled() || hook.Fn == nil {
		return
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			reportHookError(hook.OnError, "afterCollect", recovered)
		}
	}()
	hook.Fn(result, AfterHookInput[types.CollectPaymentInput]{Input: sent, Raw: raw})
}

// runBeforePayout applies the beforePayout hook, if any. Same contract as
// runBeforeCollect.
func (c *NylonPayClient) runBeforePayout(input types.MakePayoutInput) types.MakePayoutInput {
	hook := c.hooks().BeforePayout
	if !hook.isEnabled() || hook.Fn == nil {
		return input
	}

	var mutated *types.MakePayoutInput
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				mutated = nil
				reportHookError(hook.OnError, "beforePayout", recovered)
			}
		}()
		mutated = hook.Fn(input)
	}()

	if mutated == nil {
		return input
	}
	return *mutated
}

// runAfterPayout fires the afterPayout hook, on success and on failure alike.
func (c *NylonPayClient) runAfterPayout(result HookResult, sent, raw types.MakePayoutInput) {
	hook := c.hooks().AfterPayout
	if !hook.isEnabled() || hook.Fn == nil {
		return
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			reportHookError(hook.OnError, "afterPayout", recovered)
		}
	}()
	hook.Fn(result, AfterHookInput[types.MakePayoutInput]{Input: sent, Raw: raw})
}
