package email

import (
	"strings"
	"testing"
)

func TestHeroImageIsFullBleedNoInsetNoCapNoBorder(t *testing.T) {
	html := renderHeroImageBlock("https://cdn.example.org/hero.png", "Hero")
	if strings.Contains(html, "padding:0 40px") {
		t.Fatal("hero image must not be inset with side padding")
	}
	if strings.Contains(html, "max-width:520px") {
		t.Fatal("hero image must not be capped at a fixed max-width")
	}
	if strings.Contains(html, "border:1px solid") {
		t.Fatal("hero image must not carry a border")
	}
	if !strings.Contains(html, "width:100%") {
		t.Fatal("hero image must be full width")
	}
	if !strings.Contains(html, `class="wc-hero"`) {
		t.Fatal("hero image should carry the wc-hero class for the responsive CSS hook")
	}
}

func TestRenderAnniversaryEmailGreetsCouple(t *testing.T) {
	html := RenderAnniversaryEmail(AnniversaryTemplateData{
		Branding:        Branding{AppName: "The Wisdom Church"},
		RecipientName:   "David Ogba",
		SpouseName:      "Sarah",
		AnniversaryDate: "14/06",
	})
	if !strings.Contains(html, "David Ogba &amp; Sarah") {
		t.Fatalf("expected a couple greeting, got:\n%s", html)
	}
}

func TestRenderAnniversaryEmailDoesNotDoubleComposeWhenAlreadyCouple(t *testing.T) {
	html := RenderAnniversaryEmail(AnniversaryTemplateData{
		Branding:      Branding{AppName: "The Wisdom Church"},
		RecipientName: "David Ogba & Sarah",
		SpouseName:    "Sarah",
	})
	if strings.Contains(html, "Sarah & Sarah") {
		t.Fatalf("spouse name must not be appended twice, got:\n%s", html)
	}
}
