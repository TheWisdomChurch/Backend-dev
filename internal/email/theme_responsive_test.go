package email

import (
	"strings"
	"testing"
)

func TestResponsiveEmailShellHasFluidViewportAndMobileRules(t *testing.T) {
	html := EnsureResponsiveDocument(Branding{AppName: "The Wisdom Church"}, "<h1>Welcome</h1><p>Message</p>")
	for _, required := range []string{
		`name="viewport"`,
		`class="wc-frame"`,
		`width="100%"`,
		`max-width:640px`,
		`@media only screen and (max-width:480px)`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("responsive email is missing %q", required)
		}
	}
	if strings.Contains(html, `width="600"`) {
		t.Fatal("fixed-width email table must not be reintroduced")
	}
}

func TestResponsiveEmailShellDoesNotDoubleWrapNativeTemplate(t *testing.T) {
	first := renderEmailShell(Branding{AppName: "The Wisdom Church"}, colorAccent, renderBodyOpen()+"Hello"+renderBodyClose())
	second := EnsureResponsiveDocument(Branding{AppName: "The Wisdom Church"}, first)
	if first != second {
		t.Fatal("native responsive template was wrapped twice")
	}
}

func TestResponsiveEmailShellDoesNotInjectGenericDocumentLabels(t *testing.T) {
	html := EnsureResponsiveDocument(Branding{
		AppName:    "The Wisdom Church",
		AppTagline: "Official communication",
	}, "<p>Message</p>")

	for _, forbidden := range []string{"Official communication", "Official document", "Secure recipient communication"} {
		if strings.Contains(strings.ToLower(html), strings.ToLower(forbidden)) {
			t.Fatalf("global email shell must not inject %q", forbidden)
		}
	}
}
