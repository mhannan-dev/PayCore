package models

import (
	"errors"
	"regexp"
	"strings"
)

var quantumDecimal = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// ParseQuantumDecimal uses only decimal string arithmetic and the int64 boundary.
func ParseQuantumDecimal(input string, decimals int) (Amount, error) {
	input = strings.TrimSpace(input)
	if decimals < 0 || decimals > 18 || !quantumDecimal.MatchString(input) {
		return 0, errors.New("invalid token amount")
	}
	whole, fraction, _ := strings.Cut(input, ".")
	if len(fraction) > decimals {
		return 0, errors.New("excess token precision")
	}
	units := strings.TrimLeft(whole+fraction+strings.Repeat("0", decimals-len(fraction)), "0")
	return ParseAmount(units)
}
func FormatQuantumAmount(amount Amount, decimals int) (string, error) {
	if amount < 0 || decimals < 0 || decimals > 18 {
		return "", errors.New("invalid token amount")
	}
	raw := amount.String()
	if decimals == 0 {
		return raw, nil
	}
	if len(raw) <= decimals {
		raw = strings.Repeat("0", decimals+1-len(raw)) + raw
	}
	at := len(raw) - decimals
	fraction := strings.TrimRight(raw[at:], "0")
	if fraction == "" {
		return raw[:at], nil
	}
	return raw[:at] + "." + fraction, nil
}
