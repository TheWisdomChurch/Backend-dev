// Package sanitize provides HTML stripping utilities for user-supplied
// free-text fields before they are persisted. It must NOT be applied to
// fields that intentionally contain HTML (e.g. EmailTemplate.HTMLBody).
package sanitize

import "github.com/microcosm-cc/bluemonday"

var strict = bluemonday.StrictPolicy()

// Text strips all HTML tags from s and returns the plain-text result.
func Text(s string) string {
	return strict.Sanitize(s)
}

// TextPtr strips HTML from a pointer string. Returns nil when s is nil.
func TextPtr(s *string) *string {
	if s == nil {
		return nil
	}
	out := strict.Sanitize(*s)
	return &out
}

// Fields sanitizes every non-empty string in the provided map in place.
func Fields(fields map[string]string) map[string]string {
	for k, v := range fields {
		fields[k] = strict.Sanitize(v)
	}
	return fields
}
