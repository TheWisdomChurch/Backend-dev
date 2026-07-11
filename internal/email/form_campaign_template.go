package email

import (
	"bytes"
	"html/template"
	"strings"
)

type FormCampaignEmailCTA struct {
	Label string
	URL   string
}

type FormCampaignEmailHighlight struct {
	Label string
	Value string
}

type FormCampaignEmailResource struct {
	Label       string
	URL         string
	Description string
	Kind        string
}

type FormCampaignTemplateData struct {
	Branding          Branding
	Subject           string
	PreviewText       string
	HeroEyebrow       string
	HeroTitle         string
	HeroSubtitle      string
	RecipientName     string
	FormTitle         string
	IntroHTML         template.HTML
	BodyHTML          template.HTML
	ClosingHTML       template.HTML
	HeroImageURL      string
	FlyerImageURLs    []string
	Highlights        []FormCampaignEmailHighlight
	ResourceLinks     []FormCampaignEmailResource
	PrimaryCTA        *FormCampaignEmailCTA
	SecondaryCTA      *FormCampaignEmailCTA
	EventTitle        string
	EventDate         string
	EventTime         string
	EventLocation     string
	RegistrationCode  string
	CalendarOptInURL  string
	GoogleCalendarURL string
	CalendarICSURL    string
	FooterNote        string
}

// formCampaignBodyTemplate renders only the body content (no page/card/header/
// footer chrome — that's applied uniformly by renderEmailShell, same as every
// other template in this package) for the campaign/marketing email.
var formCampaignBodyTemplate = template.Must(template.New("form-campaign-body").Funcs(template.FuncMap{
	"hero": func(url, alt string) template.HTML {
		return template.HTML(renderHeroImageBlock(url, alt))
	},
	"button": func(label, url string) template.HTML {
		return template.HTML(renderButton(label, url, "", ""))
	},
	"outlineButton": func(label, url string) template.HTML {
		return template.HTML(renderOutlineButton(label, url))
	},
	"textLink": func(label, url string) template.HTML {
		return template.HTML(renderTextLink(label, url))
	},
	"code": func(label, value string) template.HTML {
		return template.HTML(renderCodeBlock(label, value))
	},
	"hasText": func(s string) bool {
		return strings.TrimSpace(s) != ""
	},
	"resourceKindLabel": func(kind string) string {
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "flyer":
			return "Flyer"
		case "document":
			return "Document"
		case "guide":
			return "Guide"
		case "schedule":
			return "Schedule"
		default:
			return "Resource"
		}
	},
	"resourceActionLabel": func(kind string) string {
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "flyer":
			return "Download flyer"
		case "document":
			return "Download document"
		case "guide":
			return "Open guide"
		case "schedule":
			return "Open schedule"
		default:
			return "Open resource"
		}
	},
}).Parse(`
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">{{.PreviewText}}</div>

<tr><td style="padding:36px 40px 8px;font-family:` + fontStack + `;">
  <div style="font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:` + colorAccent + `;margin-bottom:12px;">{{.HeroEyebrow}}</div>
  <h1 style="margin:0 0 16px;font-size:24px;line-height:1.3;font-weight:700;letter-spacing:-.01em;color:` + colorInk + `;">{{.HeroTitle}}</h1>
  <p style="margin:0 0 16px;font-size:15px;line-height:1.65;color:` + colorBody + `;">Hello {{.RecipientName}}, {{.HeroSubtitle}}</p>
</td></tr>

{{hero .HeroImageURL .HeroTitle}}

{{if .IntroHTML}}
<tr><td style="padding:0 40px 8px;font-family:` + fontStack + `;">
  <div style="font-size:15px;line-height:1.65;color:` + colorBody + `;">{{.IntroHTML}}</div>
</td></tr>
{{end}}

{{if or (hasText .EventTitle) (hasText .EventDate) (hasText .EventTime) (hasText .EventLocation)}}
<tr><td style="padding:16px 40px 0;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorLine + `;"><tr><td style="padding:20px 20px;font-family:` + fontStack + `;">
    <div style="font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:` + colorMuted + `;margin-bottom:10px;">Event details</div>
    {{if hasText .EventTitle}}<div style="margin:0 0 10px;font-size:18px;font-weight:700;letter-spacing:-.01em;color:` + colorInk + `;">{{.EventTitle}}</div>{{end}}
    {{if hasText .EventDate}}<div style="margin:0 0 6px;font-size:14px;color:` + colorBody + `;"><strong style="color:` + colorInk + `;">Date:</strong> {{.EventDate}}</div>{{end}}
    {{if hasText .EventTime}}<div style="margin:0 0 6px;font-size:14px;color:` + colorBody + `;"><strong style="color:` + colorInk + `;">Time:</strong> {{.EventTime}}</div>{{end}}
    {{if hasText .EventLocation}}<div style="margin:0;font-size:14px;color:` + colorBody + `;"><strong style="color:` + colorInk + `;">Venue:</strong> {{.EventLocation}}</div>{{end}}
  </td></tr></table>
</td></tr>
{{end}}

{{if hasText .RegistrationCode}}
<tr><td style="padding:16px 40px 0;">{{code "Registration code" .RegistrationCode}}</td></tr>
{{end}}

{{if .Highlights}}
<tr><td style="padding:16px 40px 0;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
  {{range .Highlights}}
    <tr><td style="padding:0 0 10px;">
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorLine + `;"><tr><td style="padding:14px 16px;font-family:` + fontStack + `;">
        <div style="font-size:10px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:` + colorFaint + `;margin-bottom:4px;">{{.Label}}</div>
        <div style="font-size:15px;line-height:1.5;color:` + colorInk + `;font-weight:600;">{{.Value}}</div>
      </td></tr></table>
    </td></tr>
  {{end}}
  </table>
</td></tr>
{{end}}

{{if .BodyHTML}}
<tr><td style="padding:16px 40px 0;font-family:` + fontStack + `;">
  <div style="font-size:15px;line-height:1.75;color:` + colorBody + `;">{{.BodyHTML}}</div>
</td></tr>
{{end}}

{{if or (hasText .CalendarOptInURL) (hasText .GoogleCalendarURL) (hasText .CalendarICSURL)}}
<tr><td style="padding:20px 40px 0;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:` + colorInk + `;"><tr><td style="padding:22px 22px 24px;font-family:` + fontStack + `;">
    <div style="font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:` + colorAccent + `;margin-bottom:8px;">Calendar action</div>
    <div style="font-size:18px;font-weight:700;letter-spacing:-.01em;color:` + colorPaper + `;margin-bottom:14px;">Open your calendar now</div>
    {{if hasText .CalendarOptInURL}}{{button "Add to calendar" .CalendarOptInURL}}{{else if hasText .GoogleCalendarURL}}{{button "Open Google Calendar" .GoogleCalendarURL}}{{end}}
    {{if and (hasText .CalendarOptInURL) (hasText .GoogleCalendarURL)}}<div style="height:10px;line-height:10px;font-size:0;">&nbsp;</div>{{button "Google Calendar" .GoogleCalendarURL}}{{end}}
    {{if hasText .CalendarICSURL}}<div style="height:10px;line-height:10px;font-size:0;">&nbsp;</div><a href="{{.CalendarICSURL}}" style="display:inline-block;font-family:` + fontStack + `;font-size:13px;color:` + colorAccent + `;text-decoration:none;">Download .ics</a>{{end}}
  </td></tr></table>
</td></tr>
{{end}}

{{if .ResourceLinks}}
<tr><td style="padding:20px 40px 0;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorLine + `;"><tr><td style="padding:20px;font-family:` + fontStack + `;">
    <div style="font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:` + colorMuted + `;margin-bottom:14px;">Event resources</div>
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
    {{range .ResourceLinks}}
      <tr><td style="padding:0 0 14px;">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorLine + `;"><tr><td style="padding:16px;">
          <div style="font-size:10px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:` + colorFaint + `;margin-bottom:8px;">{{resourceKindLabel .Kind}}</div>
          <div style="font-size:16px;font-weight:700;color:` + colorInk + `;margin-bottom:8px;">{{.Label}}</div>
          {{if hasText .Description}}<p style="margin:0 0 12px;font-size:13px;line-height:1.6;color:` + colorMuted + `;">{{.Description}}</p>{{end}}
          {{outlineButton (resourceActionLabel .Kind) .URL}}
        </td></tr></table>
      </td></tr>
    {{end}}
    </table>
  </td></tr></table>
</td></tr>
{{end}}

{{if or .PrimaryCTA .SecondaryCTA}}
<tr><td style="padding:24px 40px 0;">
  {{if .PrimaryCTA}}{{button .PrimaryCTA.Label .PrimaryCTA.URL}}{{end}}
  {{if and .PrimaryCTA .SecondaryCTA}}<div style="height:10px;line-height:10px;font-size:0;">&nbsp;</div>{{end}}
  {{if .SecondaryCTA}}{{outlineButton .SecondaryCTA.Label .SecondaryCTA.URL}}{{end}}
</td></tr>
{{end}}

{{if .FlyerImageURLs}}
<tr><td style="padding:24px 40px 0;">
  <div style="font-family:` + fontStack + `;font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:` + colorMuted + `;margin-bottom:12px;">Event flyers</div>
  {{range .FlyerImageURLs}}
  <div style="margin:0 0 12px;"><img src="{{.}}" alt="Event flyer" style="display:block;width:100%;height:auto;border:1px solid ` + colorLine + `;"></div>
  {{end}}
</td></tr>
{{end}}

{{if .ClosingHTML}}
<tr><td style="padding:20px 40px 0;font-family:` + fontStack + `;">
  <div style="font-size:15px;line-height:1.7;color:` + colorBody + `;">{{.ClosingHTML}}</div>
</td></tr>
{{end}}

{{if hasText .FooterNote}}
<tr><td style="padding:20px 40px 8px;font-family:` + fontStack + `;">
  <p style="margin:0;font-size:12px;line-height:1.6;color:` + colorFaint + `;">{{.FooterNote}}</p>
</td></tr>
{{end}}
`))

func RenderFormCampaignEmail(data FormCampaignTemplateData) string {
	view := data
	view.Branding = normalizeBranding(data.Branding)

	if strings.TrimSpace(view.PreviewText) == "" {
		view.PreviewText = "Save the date and open your calendar for this event."
	}
	if strings.TrimSpace(view.HeroEyebrow) == "" {
		view.HeroEyebrow = "Event Update"
	}
	if strings.TrimSpace(view.HeroTitle) == "" {
		switch {
		case strings.TrimSpace(view.EventTitle) != "":
			view.HeroTitle = strings.TrimSpace(view.EventTitle)
		case strings.TrimSpace(view.FormTitle) != "":
			view.HeroTitle = strings.TrimSpace(view.FormTitle)
		default:
			view.HeroTitle = "Upcoming Event"
		}
	}
	if strings.TrimSpace(view.RecipientName) == "" {
		view.RecipientName = "there"
	}
	if strings.TrimSpace(view.HeroSubtitle) == "" {
		view.HeroSubtitle = "please mark the date and keep this event on your calendar."
	}
	if strings.TrimSpace(view.FooterNote) == "" {
		view.FooterNote = "Keep this email handy so your event details and calendar links stay close."
	}

	var buf bytes.Buffer
	if err := formCampaignBodyTemplate.Execute(&buf, view); err != nil {
		fallback := renderBodyOpen() +
			renderHeading("Event update") +
			renderParagraph("Please check your event details and mark your calendar.") +
			renderBodyClose()
		return renderEmailShell(view.Branding, "", fallback)
	}
	return renderEmailShell(view.Branding, "", buf.String())
}
