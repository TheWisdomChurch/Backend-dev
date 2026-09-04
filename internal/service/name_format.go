package service

import "strings"

// titleCaseName normalises a human name for display in emails and
// notifications. Names arrive from public forms in whatever case the person
// typed ("peter chima", "PETER CHIMA"), which looks unprofessional in
// outbound mail. This lower-cases each word then capitalises the first letter,
// preserving separators inside a word so "mary-jane o'brien" becomes
// "Mary-Jane O'Brien".
//
// It is deliberately simple ASCII-first casing: it does not attempt
// "McDonald" / "van der Berg" style exceptions.
func titleCaseName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	capNext := true
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if r == ' ' || r == '-' || r == '\'' || r == '.' {
			capNext = true
			b.WriteRune(r)
			continue
		}
		if capNext {
			b.WriteString(strings.ToUpper(string(r)))
			capNext = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// personDisplayName joins first/last name parts and title-cases the result,
// collapsing the extra whitespace left by an empty part.
func personDisplayName(first, last string) string {
	joined := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(first), strings.TrimSpace(last)}, " "))
	return titleCaseName(joined)
}
