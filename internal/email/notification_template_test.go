package email

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func TestNotificationEmailUsesPNGLogo(t *testing.T) {
	html := RenderNotificationEmail(NotificationTemplateData{
		Branding: Branding{AppName: "The Wisdom Church", PublicURL: "https://api.example.org"},
		Title:    "New leadership application",
		Message:  "Peter Ogba applied for leadership (Deaconess).",
	})
	if !strings.Contains(html, "/assets/logo.png") {
		t.Fatalf("expected header to reference /assets/logo.png, got:\n%s", html)
	}
	if strings.Contains(html, "logo.webp") {
		t.Fatal("logo.webp must not be referenced — Gmail/Outlook do not render WebP")
	}
}

func TestNotificationEmailRendersActionButtonNotBareURL(t *testing.T) {
	portal := "https://admin.wisdomchurchhq.org"
	html := RenderNotificationEmail(NotificationTemplateData{
		Branding:    Branding{AppName: "The Wisdom Church"},
		Title:       "New leadership application",
		Message:     "Peter Ogba applied for leadership (Deaconess).",
		ActionURL:   portal,
		ActionLabel: "Open admin portal",
	})
	if !strings.Contains(html, `<a href="`+portal+`"`) {
		t.Fatalf("expected an anchor button to %s, got:\n%s", portal, html)
	}
	if strings.Contains(html, "Open the admin portal: "+portal) {
		t.Fatal("portal link must not be dumped as bare text in the body")
	}
	if !strings.Contains(html, "wc-button") {
		t.Fatal("expected the shared button treatment (wc-button)")
	}
}

func TestNotificationEmailInternalFooterOmitsSubscriptionCopy(t *testing.T) {
	html := RenderNotificationEmail(NotificationTemplateData{
		Branding:       Branding{AppName: "The Wisdom Church"},
		Title:          "New leadership application",
		Message:        "Peter Ogba applied for leadership (Deaconess).",
		Internal:       true,
		UnsubscribeURL: "https://example.org/unsub",
	})
	for _, forbidden := range []string{"subscribed to", "Unsubscribe", "unsubscribe"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("internal notification must not contain %q", forbidden)
		}
	}
	if !strings.Contains(html, "Internal notification for The Wisdom Church administrators.") {
		t.Fatalf("expected the internal-notice footer line, got:\n%s", html)
	}
}

func TestNotificationEmailSubscriberFooterUnchanged(t *testing.T) {
	html := RenderNotificationEmail(NotificationTemplateData{
		Branding:       Branding{AppName: "The Wisdom Church"},
		Title:          "Sunday service",
		Message:        "Join us this Sunday.",
		UnsubscribeURL: "https://example.org/unsub",
	})
	if !strings.Contains(html, "subscribed to") || !strings.Contains(html, "Unsubscribe") {
		t.Fatal("non-internal notification should keep the subscription footer + unsubscribe link")
	}
}

func TestNotificationTextMirrorsActionAndInternalFooter(t *testing.T) {
	text := RenderNotificationText(NotificationTemplateData{
		Branding:      Branding{AppName: "The Wisdom Church"},
		Title:         "New leadership application",
		Message:       "Peter Ogba applied for leadership (Deaconess).",
		RecipientName: ptr("Peter Chima"),
		ActionURL:     "https://admin.wisdomchurchhq.org",
		ActionLabel:   "Open admin portal",
		Internal:      true,
	})
	if !strings.Contains(text, "Open admin portal: https://admin.wisdomchurchhq.org") {
		t.Fatalf("text body missing action line:\n%s", text)
	}
	if strings.Contains(text, "subscribed to updates") {
		t.Fatal("internal text body must not mention subscription")
	}
}

func TestEmailShellHasDarkModeAndColorScheme(t *testing.T) {
	html := renderEmailShell(Branding{AppName: "The Wisdom Church"}, "", renderBodyOpen()+"Hi"+renderBodyClose())
	for _, required := range []string{
		`name="color-scheme"`,
		`@media (prefers-color-scheme:dark)`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("email shell missing %q", required)
		}
	}
}

func TestEmailHeaderRendersSingleCleanWordmark(t *testing.T) {
	html := renderEmailShell(Branding{AppName: "The Wisdom Church"}, "", renderBodyOpen()+"Hi"+renderBodyClose())
	if strings.Count(html, "The Wisdom Church") < 1 {
		t.Fatal("expected the canonical brand name in the header")
	}
	// The stacked tiny "The" eyebrow above the name is gone.
	if strings.Contains(html, `line-height:1.3;\">The</div>`) {
		t.Fatal("stacked standalone \"The\" eyebrow should be removed")
	}
}

func TestNotificationEmailHasPreheader(t *testing.T) {
	html := RenderNotificationEmail(NotificationTemplateData{
		Branding: Branding{AppName: "The Wisdom Church"},
		Title:    "New leadership application",
		Message:  "Peter Ogba applied for leadership (Deaconess).",
	})
	if !strings.Contains(html, "Peter Ogba applied for leadership") || !strings.Contains(html, "display:none;overflow:hidden") {
		t.Fatalf("expected a hidden preheader carrying the message summary:\n%s", html)
	}
}
