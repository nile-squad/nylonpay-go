package utils

import "testing"

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "local 10-digit prefixed with 0",
			input: "0771234567",
			want:  "256771234567",
		},
		{
			name:  "international with plus",
			input: "+256771234567",
			want:  "256771234567",
		},
		{
			name:  "international without plus",
			input: "256771234567",
			want:  "256771234567",
		},
		{
			name:  "strips leading spaces",
			input: "  0771234567",
			want:  "256771234567",
		},
		{
			name:  "strips trailing spaces",
			input: "0771234567  ",
			want:  "256771234567",
		},
		{
			name:  "strips internal spaces",
			input: "077 123 4567",
			want:  "256771234567",
		},
		{
			name:  "strips tabs and newlines",
			input: "077\t123\n4567",
			want:  "256771234567",
		},
		{
			name:  "with plus and spaces",
			input: "+256 771 234 567",
			want:  "256771234567",
		},
		// Numbers that start with 0 but aren't 10 digits should NOT get 256 prefix.
		{
			name:  "0-prefix but 9 digits — no country code added",
			input: "077123456",
			want:  "077123456",
		},
		{
			name:  "0-prefix but 11 digits — no country code added",
			input: "07712345678",
			want:  "07712345678",
		},
		// Non-zero starting numbers pass through as-is (after stripping + and spaces).
		{
			name:  "Kenya number",
			input: "+254712345678",
			want:  "254712345678",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePhone(tc.input)
			if got != tc.want {
				t.Errorf("NormalizePhone(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
