package crypto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf16"
)

// createCanonicalPayload returns the RFC 8785 (JCS) canonical serialization of
// payload. This exact string is what both the request signature and the
// response signature are computed over, so it must be byte-identical to what
// the backend produces for the same data.
//
// Three rules, each of which has a conformance vector guarding it:
//
//   - Object keys are sorted by UTF-16 code unit, recursively (V5, V7).
//   - Arrays keep their order; objects nested inside them are still sorted (V3).
//   - Strings carry minimal JSON escaping only. "/", "<", ">", "&" and every
//     non-ASCII character are emitted literally (V4).
//
// encoding/json cannot be used for the final emit: it HTML-escapes "<", ">"
// and "&", and it sorts map keys by UTF-8 byte order, which is code *point*
// order and diverges from code-unit order above U+FFFF. The serializer below
// is written out by hand for that reason.
func createCanonicalPayload(payload any) (string, error) {
	normalized, err := normalizeForCanonical(payload)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := writeCanonicalValue(&buf, normalized); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// normalizeForCanonical flattens an arbitrary Go value into the generic JSON
// shapes (map, slice, string, bool, json.Number, nil) so the emitter has a
// single set of types to walk.
//
// The marshal/unmarshal round trip is lossless for our purposes: json.Marshal
// escapes "<" to "<", and the decoder turns it straight back into "<", so
// the escaping the emitter cares about is decided only at emit time.
// UseNumber keeps numeric literals as their original text, so an integer is
// never widened through float64.
func normalizeForCanonical(payload any) (any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("canonical payload: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("canonical payload: %w", err)
	}
	return normalized, nil
}

func writeCanonicalValue(buf *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if typed {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		// Written verbatim. Every number the SDK puts on the wire is an
		// integer (invariant 33), so the ECMAScript float formatting that JCS
		// mandates is never reachable.
		buf.WriteString(typed.String())
	case string:
		writeCanonicalString(buf, typed)
	case []any:
		buf.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		buf.WriteByte('{')
		for i, key := range sortedKeysByCodeUnit(typed) {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonicalString(buf, key)
			buf.WriteByte(':')
			if err := writeCanonicalValue(buf, typed[key]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonical payload: unsupported value type %T", value)
	}
	return nil
}

// writeCanonicalString emits a JSON string with the minimal escaping JCS
// requires: the two mandatory escapes, the five short forms for the control
// characters that have them, and \u00xx for the rest of C0. Everything else,
// including "/" and all non-ASCII, is written through untouched.
func writeCanonicalString(buf *bytes.Buffer, value string) {
	const hexDigits = "0123456789abcdef"

	buf.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(`\u00`)
				buf.WriteByte(hexDigits[(r>>4)&0xF])
				buf.WriteByte(hexDigits[r&0xF])
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// sortedKeysByCodeUnit orders object keys by UTF-16 code unit, as RFC 8785
// requires.
//
// This is deliberately not sort.Strings: Go compares strings by UTF-8 byte,
// which is Unicode code-point order. The two agree across the whole BMP and
// diverge above U+FFFF, where surrogates (U+D800-U+DFFF) sort below
// U+E000-U+FFFF under code units but above them under code points. Merchant
// metadata keys are arbitrary strings, so an emoji key reaches this path.
func sortedKeysByCodeUnit(object map[string]any) []string {
	type encodedKey struct {
		key   string
		units []uint16
	}

	encoded := make([]encodedKey, 0, len(object))
	for key := range object {
		encoded = append(encoded, encodedKey{key: key, units: utf16.Encode([]rune(key))})
	}

	sort.Slice(encoded, func(i, j int) bool {
		return lessByCodeUnit(encoded[i].units, encoded[j].units)
	})

	keys := make([]string, len(encoded))
	for i, item := range encoded {
		keys[i] = item.key
	}
	return keys
}

func lessByCodeUnit(first, second []uint16) bool {
	shortest := len(first)
	if len(second) < shortest {
		shortest = len(second)
	}
	for i := 0; i < shortest; i++ {
		if first[i] != second[i] {
			return first[i] < second[i]
		}
	}
	return len(first) < len(second)
}
