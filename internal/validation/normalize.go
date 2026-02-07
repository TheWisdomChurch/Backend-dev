package validation

import "strings"

// NormalizeEmail trims and lowercases an email string.
func NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// NormalizeString trims surrounding whitespace.
func NormalizeString(value string) string {
	return strings.TrimSpace(value)
}
