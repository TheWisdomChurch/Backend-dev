package service

import "testing"

func TestTitleCaseName(t *testing.T) {
	cases := map[string]string{
		"peter chima":       "Peter Chima",
		"PETER CHIMA":       "Peter Chima",
		"  peter  ":         "Peter",
		"mary-jane o'brien": "Mary-Jane O'Brien",
		"":                  "",
	}
	for in, want := range cases {
		if got := titleCaseName(in); got != want {
			t.Errorf("titleCaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPersonDisplayName(t *testing.T) {
	if got := personDisplayName("peter", "ogba"); got != "Peter Ogba" {
		t.Errorf("got %q", got)
	}
	if got := personDisplayName("peter", ""); got != "Peter" {
		t.Errorf("empty last name: got %q", got)
	}
}
