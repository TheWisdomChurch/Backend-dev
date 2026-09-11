package repository

import "testing"

func TestHumanizeSlug(t *testing.T) {
	cases := map[string]string{
		"wisdom-house-choir-wave-city-music": "Wisdom House Choir Wave City Music",
		"media_tech":                         "Media Tech",
		"ushering":                           "ushering",     // no separator — left alone
		"Media & Tech":                       "Media & Tech", // already has a space
		"  ":                                 "",
		"":                                   "",
		"choir-":                             "Choir",
	}
	for in, want := range cases {
		if got := humanizeSlug(in); got != want {
			t.Errorf("humanizeSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
