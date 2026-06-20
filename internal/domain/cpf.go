package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrInvalidCPF is returned when a CPF string cannot be normalized or validated.
var ErrInvalidCPF = errors.New("invalid CPF: must contain exactly 11 digits")

// cleanDigits strips all non-digit characters from s and returns the resulting string.
func cleanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FormatCPF converts an 11-digit CPF string to the masked format "NNN.NNN.NNN-NN".
// Returns ErrInvalidCPF if the input does not have exactly 11 digits after stripping punctuation.
func FormatCPF(digits string) (string, error) {
	d := cleanDigits(digits)
	if len(d) != 11 {
		return "", ErrInvalidCPF
	}
	return fmt.Sprintf("%s.%s.%s-%s", d[0:3], d[3:6], d[6:9], d[9:11]), nil
}

// ParseCPF converts a masked CPF "NNN.NNN.NNN-NN" to its 11-digit form.
// Also accepts already-clean 11-digit strings.
// Returns ErrInvalidCPF if the result does not have exactly 11 digits.
func ParseCPF(masked string) (string, error) {
	d := cleanDigits(masked)
	if len(d) != 11 {
		return "", ErrInvalidCPF
	}
	return d, nil
}

// ValidateCPF reports whether the input, after stripping punctuation, contains exactly 11 digits.
// It does NOT validate the CPF check-digits (MVP scope).
func ValidateCPF(input string) bool {
	return len(cleanDigits(input)) == 11
}

// NormalizeCPF accepts either an 11-digit string or a masked "NNN.NNN.NNN-NN" string
// and returns the canonical 11-digit form. Used at the webhook boundary to correlate
// employeeNoString with members.federal_document (plan.md §Mapper de CPF).
func NormalizeCPF(input string) (string, error) {
	d := cleanDigits(input)
	if len(d) != 11 {
		return "", ErrInvalidCPF
	}
	return d, nil
}

// MaskCPFForLog returns a log-safe CPF representation "***.***.***-NN"
// where only the last 2 digits are visible (plan.md §S3, spec.md §FR-018).
func MaskCPFForLog(digits string) string {
	d := cleanDigits(digits)
	if len(d) < 2 {
		return "***.***.***-**"
	}
	return fmt.Sprintf("***.***.***-%s", d[len(d)-2:])
}
