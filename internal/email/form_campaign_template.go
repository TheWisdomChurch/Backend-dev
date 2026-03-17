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

var formCampaignHTMLTemplate = template.Must(template.New("form-campaign-email").Funcs(template.FuncMap{
	"logo": func(b Branding) template.HTML {
		return template.HTML(renderLogoBlock(b))
	},
	"hero": func(url, alt string) template.HTML {
		return template.HTML(renderHeroImageBlock(url, alt))
	},
	"footer": func(b Branding) template.HTML {
		return template.HTML(footerBlock(b))
	},
	"hasText": func(s string) bool {
		return strings.TrimSpace(s) != ""
	},
}).Parse(`<!DOCTYPE html>
<html>
  <body style="margin:0;padding:0;background:#f4efe3;">
    <div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">{{.PreviewText}}</div>
    <div style="padding:28px 14px;">
      <div style="max-width:680px;margin:0 auto;background:#fffdf8;border:1px solid #eadfcf;border-radius:28px;overflow:hidden;">
        <div style="height:10px;background:linear-gradient(90deg,#0f172a 0%,#b78b2a 48%,#f7e2a6 100%);"></div>
        <div style="padding:34px 38px 40px;">
          {{logo .Branding}}
          <div style="margin:0 0 8px;font-family:'Segoe UI',Arial,sans-serif;font-size:12px;letter-spacing:0.22em;text-transform:uppercase;color:#9a6b0d;font-weight:800;">{{.HeroEyebrow}}</div>
          <h1 style="margin:0 0 12px;font-family:Georgia,'Times New Roman',serif;font-size:34px;line-height:1.15;color:#101828;">{{.HeroTitle}}</h1>
          <p style="margin:0 0 18px;font-family:'Segoe UI',Arial,sans-serif;font-size:17px;line-height:1.7;color:#475467;">Hello {{.RecipientName}}, {{.HeroSubtitle}}</p>
          {{hero .HeroImageURL .HeroTitle}}

          {{if .IntroHTML}}
          <div style="margin:22px 0 0;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;line-height:1.8;color:#344054;">{{.IntroHTML}}</div>
          {{end}}

          {{if or (hasText .EventTitle) (hasText .EventDate) (hasText .EventTime) (hasText .EventLocation)}}
          <div style="margin:26px 0 0;padding:24px;background:#fff7e8;border:1px solid #e8d7b4;border-radius:22px;">
            <div style="margin:0 0 10px;font-family:'Segoe UI',Arial,sans-serif;font-size:11px;letter-spacing:0.18em;text-transform:uppercase;color:#7a5608;font-weight:800;">Event Details</div>
            {{if hasText .EventTitle}}<div style="margin:0 0 12px;font-family:Georgia,'Times New Roman',serif;font-size:26px;line-height:1.2;color:#101828;">{{.EventTitle}}</div>{{end}}
            {{if hasText .EventDate}}<div style="margin:0 0 8px;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;color:#344054;"><strong style="color:#101828;">Date:</strong> {{.EventDate}}</div>{{end}}
            {{if hasText .EventTime}}<div style="margin:0 0 8px;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;color:#344054;"><strong style="color:#101828;">Time:</strong> {{.EventTime}}</div>{{end}}
            {{if hasText .EventLocation}}<div style="margin:0;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;color:#344054;"><strong style="color:#101828;">Venue:</strong> {{.EventLocation}}</div>{{end}}
          </div>
          {{end}}

          {{if hasText .RegistrationCode}}
          <div style="margin:20px 0 0;display:inline-block;padding:14px 18px;background:#111827;border-radius:16px;">
            <div style="margin:0 0 4px;font-family:'Segoe UI',Arial,sans-serif;font-size:11px;letter-spacing:0.16em;text-transform:uppercase;color:#f9d77f;font-weight:700;">Registration Code</div>
            <div style="font-family:'Segoe UI',Arial,sans-serif;font-size:22px;letter-spacing:0.12em;color:#ffffff;font-weight:800;">{{.RegistrationCode}}</div>
          </div>
          {{end}}

          {{if .Highlights}}
          <div style="margin:26px 0 0;">
            {{range .Highlights}}
            <div style="margin:0 0 12px;padding:14px 16px;background:#f8fafc;border:1px solid #e4e7ec;border-radius:16px;">
              <div style="margin:0 0 4px;font-family:'Segoe UI',Arial,sans-serif;font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#667085;font-weight:800;">{{.Label}}</div>
              <div style="font-family:'Segoe UI',Arial,sans-serif;font-size:15px;line-height:1.7;color:#101828;font-weight:600;">{{.Value}}</div>
            </div>
            {{end}}
          </div>
          {{end}}

          {{if .BodyHTML}}
          <div style="margin:26px 0 0;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;line-height:1.9;color:#344054;">{{.BodyHTML}}</div>
          {{end}}

          {{if or (hasText .CalendarOptInURL) (hasText .GoogleCalendarURL) (hasText .CalendarICSURL)}}
          <div style="margin:28px 0 0;padding:24px;background:#101828;border-radius:24px;">
            <div style="margin:0 0 10px;font-family:'Segoe UI',Arial,sans-serif;font-size:11px;letter-spacing:0.16em;text-transform:uppercase;color:#f9d77f;font-weight:800;">Calendar Action</div>
            <div style="margin:0 0 10px;font-family:Georgia,'Times New Roman',serif;font-size:28px;line-height:1.2;color:#ffffff;">Open your calendar now</div>
            <p style="margin:0 0 16px;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;line-height:1.8;color:#d0d5dd;">Save this event immediately so it is already on your schedule before the day arrives.</p>
            {{if hasText .CalendarOptInURL}}
            <a href="{{.CalendarOptInURL}}" style="display:inline-block;margin:0 12px 12px 0;padding:13px 18px;background:#f9d77f;color:#101828;text-decoration:none;border-radius:999px;font-family:'Segoe UI',Arial,sans-serif;font-size:14px;font-weight:800;">Add To Calendar</a>
            {{else if hasText .GoogleCalendarURL}}
            <a href="{{.GoogleCalendarURL}}" style="display:inline-block;margin:0 12px 12px 0;padding:13px 18px;background:#f9d77f;color:#101828;text-decoration:none;border-radius:999px;font-family:'Segoe UI',Arial,sans-serif;font-size:14px;font-weight:800;">Open Google Calendar</a>
            {{end}}
            {{if and (hasText .CalendarOptInURL) (hasText .GoogleCalendarURL)}}
            <a href="{{.GoogleCalendarURL}}" style="display:inline-block;margin:0 12px 12px 0;padding:13px 18px;background:#ffffff;color:#101828;text-decoration:none;border-radius:999px;font-family:'Segoe UI',Arial,sans-serif;font-size:14px;font-weight:800;">Google Calendar</a>
            {{end}}
            {{if hasText .CalendarICSURL}}
            <a href="{{.CalendarICSURL}}" style="display:inline-block;margin:0 12px 12px 0;padding:13px 18px;background:#1d2939;color:#ffffff;text-decoration:none;border-radius:999px;font-family:'Segoe UI',Arial,sans-serif;font-size:14px;font-weight:800;border:1px solid #344054;">Download .ics</a>
            {{end}}
          </div>
          {{end}}

          {{if or .PrimaryCTA .SecondaryCTA}}
          <div style="margin:26px 0 0;">
            {{if .PrimaryCTA}}
            <a href="{{.PrimaryCTA.URL}}" style="display:inline-block;margin:0 12px 12px 0;padding:14px 20px;background:#b78b2a;color:#ffffff;text-decoration:none;border-radius:999px;font-family:'Segoe UI',Arial,sans-serif;font-size:14px;font-weight:800;">{{.PrimaryCTA.Label}}</a>
            {{end}}
            {{if .SecondaryCTA}}
            <a href="{{.SecondaryCTA.URL}}" style="display:inline-block;margin:0 12px 12px 0;padding:14px 20px;background:#ffffff;color:#101828;text-decoration:none;border-radius:999px;font-family:'Segoe UI',Arial,sans-serif;font-size:14px;font-weight:800;border:1px solid #d0d5dd;">{{.SecondaryCTA.Label}}</a>
            {{end}}
          </div>
          {{end}}

          {{if .FlyerImageURLs}}
          <div style="margin:28px 0 0;">
            <div style="margin:0 0 12px;font-family:'Segoe UI',Arial,sans-serif;font-size:11px;letter-spacing:0.16em;text-transform:uppercase;color:#7a5608;font-weight:800;">Event Flyers</div>
            {{range .FlyerImageURLs}}
            <div style="margin:0 0 14px;"><img src="{{.}}" alt="Event flyer" style="display:block;width:100%;height:auto;border-radius:20px;border:1px solid #eadfcf;"></div>
            {{end}}
          </div>
          {{end}}

          {{if .ClosingHTML}}
          <div style="margin:26px 0 0;font-family:'Segoe UI',Arial,sans-serif;font-size:15px;line-height:1.8;color:#344054;">{{.ClosingHTML}}</div>
          {{end}}

          {{if hasText .FooterNote}}
          <p style="margin:26px 0 0;font-family:'Segoe UI',Arial,sans-serif;font-size:13px;line-height:1.8;color:#667085;">{{.FooterNote}}</p>
          {{end}}

          {{footer .Branding}}
        </div>
      </div>
    </div>
  </body>
</html>`))

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
	if err := formCampaignHTMLTemplate.Execute(&buf, view); err != nil {
		return "<!DOCTYPE html><html><body style=\"font-family:'Segoe UI',Arial,sans-serif;padding:24px;background:#f8fafc;\"><div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\"><h2 style=\"margin:0 0 12px;\">Event update</h2><p style=\"margin:0;\">Please check your event details and mark your calendar.</p></div></body></html>"
	}
	return buf.String()
}
