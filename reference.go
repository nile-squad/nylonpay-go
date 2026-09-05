package nylonpay

import (
	"crypto/rand"
	"encoding/hex"
	"unicode/utf8"

	"github.com/nile-squad/nylonpay-go/types"
)

// A reference is echoed verbatim to the provider as its merchantTransactionId,
// which is bounded to this range. Anything outside it is rejected.
const (
	referenceMinLength = 13
	referenceMaxLength = 15
)

// validateReferenceLength enforces the 13-15 character bound synchronously, so
// an out-of-range reference never costs a network round-trip.
//
// Length is counted in characters, not bytes, so every Nylon Pay SDK accepts
// exactly the same set of reference strings regardless of encoding.
//
// The usual way to trip this is passing a 36-character UUID order id. Hash or
// truncate it to 15 characters or fewer first.
func validateReferenceLength(reference string) error {
	length := utf8.RuneCountInString(reference)
	if length < referenceMinLength || length > referenceMaxLength {
		return validationErr("reference must be %d-%d characters", referenceMinLength, referenceMaxLength)
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
	if err := validateReferenceLength(reference); err != nil {
		return "", err
	}
	return reference, nil
}

// generateReference produces a 15-character reference from a cryptographic
// source, so references are unique across rapid sequential calls.
func generateReference() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// Never silently fall back to a predictable value: a fixed reference
		// would collide with every other call and replay someone else's
		// transaction.
		return "", &types.SDKError{
			Category: types.CategoryInternal,
			Message:  "Could not generate a transaction reference",
		}
	}
	return hex.EncodeToString(buf)[:referenceMaxLength], nil
}
