// internal/email/sender.go
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	gomail "github.com/wneessen/go-mail"
)

type Sender struct {
	host       string
	port       int
	user       string
	pass       string
	from       string
	requireTLS bool
	timeout    time.Duration
	redis      *redis.Client

	// optional: template store + http client for image inlining
	tplStore *TemplateStore
	http     *http.Client
}

func NewSender(redisURL, host, port, user, pass, from string, requireTLS bool) (*Sender, error) {
	h := strings.TrimSpace(host)
	if h == "" {
		h = strings.TrimSpace(os.Getenv("SMTP_HOST"))
		if h == "" {
			h = strings.TrimSpace(os.Getenv("APP_SMTP_HOST"))
		}
	}

	p := strings.TrimSpace(port)
	if p == "" {
		p = strings.TrimSpace(os.Getenv("SMTP_PORT"))
		if p == "" {
			p = strings.TrimSpace(os.Getenv("APP_SMTP_PORT"))
		}
	}
	if p == "" {
		p = "25"
	}

	u := strings.TrimSpace(user)
	if u == "" {
		u = strings.TrimSpace(os.Getenv("SMTP_USER"))
		if u == "" {
			u = strings.TrimSpace(os.Getenv("APP_SMTP_USER"))
		}
	}

	pw := strings.TrimSpace(pass)
	if pw == "" {
		pw = strings.TrimSpace(os.Getenv("SMTP_PASS"))
		if pw == "" {
			pw = strings.TrimSpace(os.Getenv("APP_SMTP_PASS"))
		}
	}

	f := strings.TrimSpace(from)
	if f == "" {
		f = strings.TrimSpace(os.Getenv("SMTP_FROM"))
		if f == "" {
			fromEmail := strings.TrimSpace(os.Getenv("APP_SMTP_FROM_EMAIL"))
			fromName := strings.TrimSpace(os.Getenv("APP_SMTP_FROM_NAME"))
			switch {
			case fromEmail != "" && fromName != "":
				f = fmt.Sprintf("%s <%s>", fromName, fromEmail)
			case fromEmail != "":
				f = fromEmail
			}
		}
	}

	// TLS override from env if set
	if raw := strings.TrimSpace(os.Getenv("SMTP_TLS")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireTLS = parsed
		}
	} else if raw := strings.TrimSpace(os.Getenv("APP_SMTP_TLS")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireTLS = parsed
		}
	}

	if h == "" || p == "" {
		return nil, fmt.Errorf("smtp configuration is incomplete: SMTP_HOST/APP_SMTP_HOST and SMTP_PORT/APP_SMTP_PORT are required")
	}

	portInt, err := strconv.Atoi(p)
	if err != nil || portInt <= 0 || portInt > 65535 {
		return nil, fmt.Errorf("invalid smtp port: %q", p)
	}

	// Timeout (seconds)
	timeout := 20 * time.Second
	if raw := strings.TrimSpace(os.Getenv("APP_SMTP_TIMEOUT_SECONDS")); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	// Redis client (optional)
	var redisClient *redis.Client
	if strings.TrimSpace(redisURL) != "" {
		if opts, err := redis.ParseURL(redisURL); err == nil {
			redisClient = redis.NewClient(opts)
		}
	}

	// Template store (optional; only used when calling SendTemplateReceiptWithPDF)
	var ts *TemplateStore
	if strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL")) != "" {
		if store, err := NewTemplateStoreFromEnv(); err == nil {
			ts = store
		}
	}

	return &Sender{
		host:       h,
		port:       portInt,
		user:       u,
		pass:       pw,
		from:       f,
		requireTLS: requireTLS,
		timeout:    timeout,
		redis:      redisClient,
		tplStore:   ts,
		http: &http.Client{
			Timeout: 12 * time.Second,
		},
	}, nil
}

// SendHTML sends an HTML email.
func (s *Sender) SendHTML(to, subject, body string) error {
	return s.sendInternal(to, subject, body, nil)
}

// SendHTMLText sends a multipart email with text and HTML alternatives.
func (s *Sender) SendHTMLText(to, subject, htmlBody, textBody string) error {
	htmlBody = strings.TrimSpace(htmlBody)
	textBody = strings.TrimSpace(textBody)
	if htmlBody == "" {
		return fmt.Errorf("html body is required")
	}

	if textBody == "" {
		return s.SendHTML(to, subject, htmlBody)
	}

	return s.sendInternal(to, subject, "", func(msg *gomail.Msg) error {
		msg.SetBodyString(gomail.TypeTextPlain, textBody)
		msg.AddAlternativeString(gomail.TypeTextHTML, htmlBody)
		return nil
	})
}

// SendHTMLWithAttachment sends an HTML email with a single attachment (e.g. PDF).
func (s *Sender) SendHTMLWithAttachment(to, subject, body, attachmentName, contentType string, payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("attachment payload is empty")
	}
	if attachmentName == "" {
		attachmentName = "attachment.pdf"
	}
	if contentType == "" {
		contentType = "application/pdf"
	}

	return s.sendInternal(to, subject, body, func(msg *gomail.Msg) error {
		return msg.AttachReader(
			attachmentName,
			bytes.NewReader(payload),
			gomail.WithFileName(attachmentName),
			gomail.WithFileContentType(gomail.ContentType(contentType)),
		)
	})
}

// SendMultipartWithAttachment sends TEXT + HTML (alternative) + embeds (CID) + one attachment (PDF).
func (s *Sender) SendMultipartWithAttachment(
	to string,
	subject string,
	textBody string,
	htmlBody string,
	attachmentName string,
	contentType string,
	payload []byte,
	inlineImages []inlineImage, // optional (CID embeds)
) error {
	if len(payload) == 0 {
		return fmt.Errorf("attachment payload is empty")
	}
	if attachmentName == "" {
		attachmentName = "attachment.pdf"
	}
	if contentType == "" {
		contentType = "application/pdf"
	}

	return s.sendInternal(to, subject, "", func(msg *gomail.Msg) error {
		// Main body (text)
		msg.SetBodyString(gomail.TypeTextPlain, textBody)
		// Alternative body (html)
		msg.AddAlternativeString(gomail.TypeTextHTML, htmlBody)

		// Inline images (CID) -> use EmbedReader + WithFileContentID
		for _, im := range inlineImages {
			if len(im.Bytes) == 0 {
				continue
			}
			ct := im.ContentType
			if strings.TrimSpace(ct) == "" {
				ct = "application/octet-stream"
			}

			// EmbedReader will mark it as inline embed and Content-ID enables cid:xxx references
			if err := msg.EmbedReader(
				im.Filename,
				bytes.NewReader(im.Bytes),
				gomail.WithFileName(im.Filename),
				gomail.WithFileContentType(gomail.ContentType(ct)),
				gomail.WithFileContentID(im.CID),
			); err != nil {
				return fmt.Errorf("embed inline image failed: %w", err)
			}
		}

		// PDF attachment
		return msg.AttachReader(
			attachmentName,
			bytes.NewReader(payload),
			gomail.WithFileName(attachmentName),
			gomail.WithFileContentType(gomail.ContentType(contentType)),
		)
	})
}

// SendTemplateReceiptWithPDF fetches templates from S3, enforces size limits (in TemplateStore),
// optionally embeds images <= MaxInlineImgMB, and attaches the encrypted PDF.
func (s *Sender) SendTemplateReceiptWithPDF(
	ctx context.Context,
	toEmail string,
	subject string,
	templateKey string, // e.g. "receipt" -> receipt.html + receipt.txt
	data TemplateData,
	attachmentName string,
	encryptedPDF []byte,
) error {
	if s.tplStore == nil {
		return fmt.Errorf("template store not configured: check S3_PUBLIC_BASE_URL and S3_EMAIL_TEMPLATE_PATH")
	}

	textBody, htmlBody, rawHTML, err := s.tplStore.Render(ctx, templateKey, data)
	if err != nil {
		return err
	}

	inlineImgs, rewrittenHTML := s.tryInlineImages(ctx, htmlBody, rawHTML, s.tplStore.MaxInlineImgMB)

	finalHTML := htmlBody
	if rewrittenHTML != "" {
		finalHTML = rewrittenHTML
	}

	return s.SendMultipartWithAttachment(
		toEmail,
		subject,
		textBody,
		finalHTML,
		attachmentName,
		"application/pdf",
		encryptedPDF,
		inlineImgs,
	)
}

// sendInternal encapsulates the actual SMTP send logic.
// IMPORTANT: if body is empty, mutate must set bodies (multipart).
func (s *Sender) sendInternal(to, subject, body string, mutate func(msg *gomail.Msg) error) error {
	if s.host == "" || s.port == 0 {
		return fmt.Errorf("smtp configuration is incomplete")
	}

	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("recipient address is empty")
	}

	// Rate limiting: 10 mails/min/recipient
	if s.redis != nil {
		ctx := context.Background()
		key := fmt.Sprintf("email_rate:%s", strings.ToLower(to))
		if count, err := s.redis.Incr(ctx, key).Result(); err == nil {
			if count == 1 {
				s.redis.Expire(ctx, key, time.Minute)
			}
			if count > 10 {
				return fmt.Errorf("rate limit exceeded for %s", to)
			}
		}
	}

	headerFrom := strings.TrimSpace(s.from)
	if headerFrom == "" {
		headerFrom = strings.TrimSpace(s.user)
	}
	if headerFrom == "" {
		return fmt.Errorf("smtp from address is missing")
	}

	fromAddr, err := mail.ParseAddress(headerFrom)
	if err != nil {
		return fmt.Errorf("invalid SMTP_FROM address: %w", err)
	}

	// TLS policy
	tlsPolicy := gomail.TLSOpportunistic
	var tlsConfig *tls.Config
	if s.requireTLS {
		tlsPolicy = gomail.TLSMandatory
		tlsConfig = &tls.Config{
			ServerName: s.host,
			MinVersion: tls.VersionTLS12,
		}
	}

	options := []gomail.Option{
		gomail.WithPort(s.port),
		gomail.WithTimeout(s.timeout),
		gomail.WithTLSPolicy(tlsPolicy),
	}
	if tlsConfig != nil {
		options = append(options, gomail.WithTLSConfig(tlsConfig))
	}
	if s.port == 465 {
		options = append(options, gomail.WithSSL())
	}

	// Only configure SMTP AUTH when user+pass are set
	if s.user != "" && s.pass != "" {
		options = append(options,
			gomail.WithUsername(s.user),
			gomail.WithPassword(s.pass),
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		)
	}

	client, err := gomail.NewClient(s.host, options...)
	if err != nil {
		return fmt.Errorf("failed to initialize smtp client: %w", err)
	}
	defer client.Close()

	msg := gomail.NewMsg()
	if err := msg.From(fromAddr.String()); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}
	if err := msg.EnvelopeFrom(fromAddr.Address); err != nil {
		return fmt.Errorf("invalid envelope from address: %w", err)
	}

	msg.Subject(subject)

	// Legacy HTML support: if body provided, set it. Otherwise mutate must set body/alt.
	if strings.TrimSpace(body) != "" {
		msg.SetBodyString(gomail.TypeTextHTML, body)
	}

	if mutate != nil {
		if err := mutate(msg); err != nil {
			return fmt.Errorf("failed to apply message mutation: %w", err)
		}
	} else if strings.TrimSpace(body) == "" {
		return fmt.Errorf("no email body provided")
	}

	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	return nil
}

/*
=========================================================
Inline images (CID) with size limits
=========================================================
*/

type inlineImage struct {
	URL         string
	CID         string
	Filename    string
	ContentType string
	Bytes       []byte
}

// tryInlineImages:
// - scans HTML for <img src="...">
// - fetches image if https and <= maxMB
// - rewrites src="..." to src="cid:CID"
// - skips any image that exceeds limit or fails fetch (keeps remote URL)
func (s *Sender) tryInlineImages(ctx context.Context, renderedHTML string, _rawHTML string, maxMB int64) ([]inlineImage, string) {
	maxBytes := maxMB * 1024 * 1024
	srcs := ExtractImgSrcs(renderedHTML)
	if len(srcs) == 0 {
		return nil, ""
	}

	inline := make([]inlineImage, 0, len(srcs))
	outHTML := renderedHTML

	for i, src := range srcs {
		u, err := url.Parse(src)
		if err != nil || u.Scheme == "" {
			continue
		}
		if u.Scheme != "https" {
			continue
		}

		// HEAD to pre-check size (if available)
		if ok, contentLen, _ := s.headImage(ctx, src); ok && contentLen > 0 && contentLen > maxBytes {
			continue // do not inline > maxBytes
		}

		b, ct, ok := s.getImageLimited(ctx, src, maxBytes)
		if !ok {
			continue
		}

		cid := fmt.Sprintf("img-%d-%d@inline", time.Now().UnixNano(), i)
		filename := filenameFromURL(u)
		if filename == "" {
			filename = fmt.Sprintf("image-%d", i)
		}

		inline = append(inline, inlineImage{
			URL:         src,
			CID:         cid,
			Filename:    filename,
			ContentType: ct,
			Bytes:       b,
		})

		outHTML = strings.ReplaceAll(outHTML, `src="`+src+`"`, `src="cid:`+cid+`"`)
		outHTML = strings.ReplaceAll(outHTML, `src='`+src+`'`, `src='cid:`+cid+`'`)
	}

	if len(inline) == 0 {
		return nil, ""
	}
	return inline, outHTML
}

func (s *Sender) headImage(ctx context.Context, imgURL string) (ok bool, contentLen int64, contentType string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, imgURL, nil)
	if err != nil {
		return false, 0, ""
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return false, 0, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, 0, ""
	}
	return true, resp.ContentLength, resp.Header.Get("Content-Type")
}

func (s *Sender) getImageLimited(ctx context.Context, imgURL string, maxBytes int64) ([]byte, string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, "", false
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", false
	}

	if resp.ContentLength > maxBytes && resp.ContentLength != -1 {
		return nil, "", false
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", false
	}
	if int64(len(b)) > maxBytes {
		return nil, "", false
	}
	return b, ct, true
}

func filenameFromURL(u *url.URL) string {
	p := strings.TrimSpace(u.Path)
	if p == "" {
		return ""
	}
	base := path.Base(p)
	return strings.TrimSpace(base)
}
