package core

import (
	"reflect"
	"sync"

	"github.com/nile-squad/nylonpay-go/types"
)

// registeredHandler is one subscription. The identity is the handler's code
// pointer, which is how Off finds it again: Go cannot compare function values.
type registeredHandler struct {
	handler types.PaymentEventHandler
	id      uintptr
	once    bool
}

// emitter is a goroutine-safe pubsub for lifecycle events.
//
// Handlers fire in registration order. A panic in one is contained, so it stops
// neither the remaining handlers nor the polling loop that emitted the event.
type emitter struct {
	mu       sync.Mutex
	handlers map[types.PaymentEvent][]*registeredHandler
}

func newEmitter() *emitter {
	return &emitter{handlers: make(map[types.PaymentEvent][]*registeredHandler)}
}

func handlerID(handler types.PaymentEventHandler) uintptr {
	return reflect.ValueOf(handler).Pointer()
}

func (e *emitter) add(event types.PaymentEvent, handler types.PaymentEventHandler, once bool) {
	if handler == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[event] = append(e.handlers[event], &registeredHandler{
		handler: handler,
		id:      handlerID(handler),
		once:    once,
	})
}

// remove drops every subscription matching handler for event. Removing a
// handler that was never registered is a no-op, not an error.
func (e *emitter) remove(event types.PaymentEvent, handler types.PaymentEventHandler) {
	if handler == nil {
		return
	}
	target := handlerID(handler)

	e.mu.Lock()
	defer e.mu.Unlock()

	registered := e.handlers[event]
	kept := make([]*registeredHandler, 0, len(registered))
	for _, candidate := range registered {
		if candidate.id != target {
			kept = append(kept, candidate)
		}
	}
	e.handlers[event] = kept
}

// take returns the handlers to fire for an event, in registration order, and
// unregisters the once-handlers among them.
//
// It deliberately does not invoke anything. The caller fires the returned
// handlers after releasing its own locks, because merchant code may run for
// arbitrarily long and must not be holding a lock the handler might itself need.
func (e *emitter) take(event types.PaymentEvent) []types.PaymentEventHandler {
	e.mu.Lock()
	defer e.mu.Unlock()

	registered := e.handlers[event]
	if len(registered) == 0 {
		return nil
	}

	firing := make([]types.PaymentEventHandler, 0, len(registered))
	kept := make([]*registeredHandler, 0, len(registered))
	for _, candidate := range registered {
		firing = append(firing, candidate.handler)
		if !candidate.once {
			kept = append(kept, candidate)
		}
	}
	e.handlers[event] = kept
	return firing
}

// invokeHandler runs one handler, absorbing any panic. A merchant's logging
// callback must never be able to abort a payment.
func invokeHandler(handler types.PaymentEventHandler, data types.EventData) {
	defer func() { _ = recover() }()
	handler(data)
}
