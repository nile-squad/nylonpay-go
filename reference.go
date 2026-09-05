package nylonpay

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"

	"github.com/nile-squad/nylonpay-go/types"
)

// referencePattern matches a reference, which the backend requires to be a
// UUID. Any version is accepted; generated references are v4.
//
// The anchors are \A and \z, not ^ and $, so the match is against the whole
// string and nothing else. Every Nylon Pay SDK must accept exactly the same set
// of reference strings, and an end-anchor that also matched before a trailing
// newline would let "<uuid>\n" through in one language and not another.
var referencePattern = regexp.MustCompile(
	`\A[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\z`,
)

// validateReferenceFormat enforces the UUID shape synchronously, so a malformed
// reference never costs a network round-trip.
//
// The usual way to trip this is passing an order id in your own format. Derive
// a UUID from it, or omit the reference and keep the generated one alongside
// your order.
func validateReferenceFormat(reference string) error {
	if !referencePattern.MatchString(reference) {
		return validationErr("reference must be a valid UUID")
	}
	return nil
}

// resolveReference returns the reference to use for a create operation,
// generating one when the merchant supplied none.
//
// The reference is the transaction identity and the only idempotency
// mechanism: reusing one replays the existing transaction instead of charging
// again, and a retry after a network failure must reuse it for that reason.
func resolveReference(reference string) (string, error) {
	if reference == "" {
		return generateReference()
	}
	if err := validateReferenceFormat(reference); err != nil {
		return "", err
	}
	return reference, nil
}

// generateReference produces a v4 UUID from a cryptographic source, so
// references are unique across rapid sequential calls.
func generateReference() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Never silently fall back to a predictable value: a fixed reference
		// would collide with every other call and replay someone else's
		// transaction.
		return "", &types.SDKError{
			Category: types.CategoryInternal,
			Message:  "Could not generate a transaction reference",
		}
	}

	buf[6] = buf[6]&0x0f | 0x40 // version 4
	buf[8] = buf[8]&0x3f | 0x80 // RFC 4122 variant

	encoded := hex.EncodeToString(buf)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32], nil
}
