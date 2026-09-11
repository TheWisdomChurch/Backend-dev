package repository

import (
	"strings"
	"unicode"
)

// humanizeSlug turns a machine-generated slug ("wisdom-house-choir-wave-city-music",
// "media_tech") into a readable name ("Wisdom House Choir Wave City Music",
// "Media Tech"). Anything that already looks human-entered — it contains a
// space, or has no hyphen/underscore at all — is returned unchanged, so an
// admin-typed department name like "Media & Tech" is never altered.
func humanizeSlug(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || strings.Contains(trimmed, " ") || !strings.ContainsAny(trimmed, "-_") {
		return trimmed
	}
	words := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		runes := []rune(strings.ToLower(w))
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
