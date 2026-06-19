package utils

import (
	"strings"
	"unicode"
)

// NormalizePhone converts a phone number to international format without a
// leading "+". Strips all whitespace, removes a leading "+", and expands
// Uganda local numbers (leading "0", 10 digits) to the 256 country code.
func NormalizePhone(phone string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, phone)

	normalized = strings.TrimPrefix(normalized, "+")

	if strings.HasPrefix(normalized, "0") && len(normalized) == 10 {
		normalized = "256" + normalized[1:]
	}

	return normalized
}
