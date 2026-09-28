package kernel

import (
	"errors"
	"strings"
)

// ErrInvalidCNPJ covers malformed, wrong-length and bad-check-digit
// identifiers. Importers quarantine on this error; letters are never
// coerced to digits (A06).
var ErrInvalidCNPJ = errors.New("kernel: invalid CNPJ")

// CNPJ is a validated 14-character ASCII identifier (digits 0-9 and, since
// the alphanumeric program, uppercase A-Z). It is always handled as text:
// leading zeroes are preserved because the value is never numeric.
type CNPJ struct {
	normalized string
}

// ParseCNPJ strips formatting (dots, slash, dash, spaces), uppercases and
// validates length, alphabet and both check digits.
func ParseCNPJ(text string) (CNPJ, error) {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		case r == '.' || r == '/' || r == '-' || r == ' ':
		default:
			return CNPJ{}, ErrInvalidCNPJ
		}
	}
	s := b.String()
	if len(s) != 14 {
		return CNPJ{}, ErrInvalidCNPJ
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z') {
			return CNPJ{}, ErrInvalidCNPJ
		}
	}
	if !checkCNPJ(s) {
		return CNPJ{}, ErrInvalidCNPJ
	}
	return CNPJ{normalized: s}, nil
}

// Normalized returns the 14-character ASCII form.
func (c CNPJ) Normalized() string { return c.normalized }

// Alphanumeric reports whether the identifier uses the letter range.
func (c CNPJ) Alphanumeric() bool {
	for _, r := range c.normalized {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

// digitValue maps '0'-'9' to 0-9 and 'A'-'Z' to 17-42 (ASCII minus 48),
// per the Receita Federal alphanumeric program.
func digitValue(r rune) int {
	if r <= '9' {
		return int(r - '0')
	}
	return int(r - 'A' + 17)
}

func checkCNPJ(s string) bool {
	w1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += digitValue(rune(s[i])) * w1[i]
	}
	d1 := 0
	if r := sum % 11; r >= 2 {
		d1 = 11 - r
	}
	w2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 12; i++ {
		sum += digitValue(rune(s[i])) * w2[i]
	}
	sum += d1 * w2[12]
	d2 := 0
	if r := sum % 11; r >= 2 {
		d2 = 11 - r
	}
	return s[12]-'0' == byte(d1) && s[13]-'0' == byte(d2)
}
