package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	texttmpl "text/template"
	"time"
)

type TemplateStore struct {
	BaseURL        string        // e.g. https://churchasset.fra1.digitaloceanspaces.com/email_template
	AllowedHosts   []string      // SSRF protection
	TTL            time.Duration // cache TTL
	MaxTemplateKB  int64         // templates size limit in KB
	MaxInlineImgMB int64         // inline image limit in MB
	client         *http.Client
	mu             sync.Mutex
	cache          map[string]cachedPair
}

type cachedPair struct {
	fetchedAt time.Time
	textTpl   *texttmpl.Template
	htmlTpl   *template.Template
	rawHTML   string
}

var errTemplateNotFound = errors.New("template not found")

func NewTemplateStoreFromEnv() (*TemplateStore, error) {
	base := strings.TrimRight(firstEnv("S3_PUBLIC_BASE_URL", "SPACES_PUBLIC_BASE_URL"), "/")
	path := strings.Trim(firstEnv("S3_EMAIL_TEMPLATE_PATH", "SPACES_EMAIL_TEMPLATE_PATH"), "/")
	if base == "" {
		base = deriveSpacesPublicBaseURL()
	}
	if base == "" {
		return nil, fmt.Errorf("S3_PUBLIC_BASE_URL is required for template fetch")
	}

	baseURL := base
	if path != "" {
		baseURL = base + "/" + path
	}

	allowedHosts := splitCSV(os.Getenv("EMAIL_TEMPLATES_ALLOWED_HOSTS"))
	if len(allowedHosts) == 0 {
		// Safe default: allow only the host from baseURL
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid template base url: %w", err)
		}
		allowedHosts = []string{u.Host}
	}

	maxKB := int64(200) // default 200KB templates
	if v := strings.TrimSpace(os.Getenv("EMAIL_TEMPLATE_MAX_KB")); v != "" {
		if n, err := parseInt64(v); err == nil && n > 0 {
			maxKB = n
		}
	}

	maxMB := int64(5) // default 5MB inline images
	if v := strings.TrimSpace(os.Getenv("EMAIL_IMAGE_MAX_MB")); v != "" {
		if n, err := parseInt64(v); err == nil && n > 0 {
			maxMB = n
		}
	}

	ttl := 10 * time.Minute
	if v := strings.TrimSpace(os.Getenv("EMAIL_TEMPLATE_CACHE_SECONDS")); v != "" {
		if sec, err := parseInt64(v); err == nil && sec > 0 {
			ttl = time.Duration(sec) * time.Second
		}
	}

	return &TemplateStore{
		BaseURL:        strings.TrimRight(baseURL, "/"),
		AllowedHosts:   normalizeHosts(allowedHosts),
		TTL:            ttl,
		MaxTemplateKB:  maxKB,
		MaxInlineImgMB: maxMB,
		client: &http.Client{
			Timeout: 12 * time.Second,
		},
		cache: make(map[string]cachedPair),
	}, nil
}

func deriveSpacesPublicBaseURL() string {
	bucket := firstEnv("S3_BUCKET", "SPACES_BUCKET")
	endpoint := firstEnv("S3_ENDPOINT", "SPACES_ENDPOINT")
	region := firstEnv("S3_REGION", "SPACES_REGION")
	if bucket == "" {
		return ""
	}

	endpoint = normalizeEndpoint(endpoint)
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err == nil {
			scheme := u.Scheme
			host := u.Host
			if host == "" {
				host = strings.TrimSpace(u.Path)
			}
			if scheme == "" {
				scheme = "https"
			}
			if host != "" {
				if !strings.HasPrefix(host, bucket+".") {
					host = bucket + "." + host
				}
				return scheme + "://" + host
			}
		}
	}

	if region != "" {
		return fmt.Sprintf("https://%s.%s.digitaloceanspaces.com", bucket, region)
	}

	return ""
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		val := strings.TrimSpace(os.Getenv(key))
		if val != "" {
			return val
		}
	}
	return ""
}

func normalizeEndpoint(endpoint string) string {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	return "https://" + raw
}

type TemplateData struct {
	BrandName    string
	Name         string
	FormTitle    string
	Filename     string
	PasswordHint string
	Year         int
	// You can add more fields anytime; templates use Go template syntax.
}

func (ts *TemplateStore) Render(ctx context.Context, templateKey string, data TemplateData) (textOut, htmlOut string, rawHTML string, err error) {
	return ts.RenderWithData(ctx, templateKey, data)
}

// RenderWithData renders a template pair using any data structure (map or struct).
func (ts *TemplateStore) RenderWithData(ctx context.Context, templateKey string, data any) (textOut, htmlOut string, rawHTML string, err error) {
	txtName := templateKey + ".txt"
	htmlName := templateKey + ".html"

	textTpl, htmlTpl, raw, err := ts.loadPair(ctx, txtName, htmlName)
	if err != nil {
		return "", "", "", err
	}

	var tb bytes.Buffer
	if err := textTpl.Execute(&tb, data); err != nil {
		return "", "", "", fmt.Errorf("render %s: %w", txtName, err)
	}

	var hb bytes.Buffer
	if err := htmlTpl.Execute(&hb, data); err != nil {
		return "", "", "", fmt.Errorf("render %s: %w", htmlName, err)
	}

	return tb.String(), hb.String(), raw, nil
}

func (ts *TemplateStore) loadPair(ctx context.Context, txtName, htmlName string) (*texttmpl.Template, *template.Template, string, error) {
	key := txtName + "|" + htmlName

	ts.mu.Lock()
	c, ok := ts.cache[key]
	if ok && time.Since(c.fetchedAt) < ts.TTL && c.textTpl != nil && c.htmlTpl != nil {
		ts.mu.Unlock()
		return c.textTpl, c.htmlTpl, c.rawHTML, nil
	}
	ts.mu.Unlock()

	htmlRaw, err := ts.fetchLimited(ctx, htmlName, ts.MaxTemplateKB*1024)
	if err != nil {
		return nil, nil, "", err
	}
	txtRaw, err := ts.fetchLimited(ctx, txtName, ts.MaxTemplateKB*1024)
	if err != nil {
		if errors.Is(err, errTemplateNotFound) {
			txtRaw = []byte(stripHTML(string(htmlRaw)))
		} else {
			return nil, nil, "", err
		}
	}

	textTpl, err := texttmpl.New(txtName).Option("missingkey=error").Parse(string(txtRaw))
	if err != nil {
		return nil, nil, "", fmt.Errorf("parse %s: %w", txtName, err)
	}
	htmlTpl, err := template.New(htmlName).Option("missingkey=error").Parse(string(htmlRaw))
	if err != nil {
		return nil, nil, "", fmt.Errorf("parse %s: %w", htmlName, err)
	}

	ts.mu.Lock()
	ts.cache[key] = cachedPair{
		fetchedAt: time.Now(),
		textTpl:   textTpl,
		htmlTpl:   htmlTpl,
		rawHTML:   string(htmlRaw),
	}
	ts.mu.Unlock()

	return textTpl, htmlTpl, string(htmlRaw), nil
}

func (ts *TemplateStore) fetchLimited(ctx context.Context, name string, maxBytes int64) ([]byte, error) {
	u := ts.BaseURL + "/" + strings.TrimLeft(name, "/")

	parsed, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("bad template url: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("template url must be https")
	}
	if !hostAllowed(parsed.Host, ts.AllowedHosts) {
		return nil, fmt.Errorf("blocked template host: %s", parsed.Host)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch template failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusNotFound {
			return nil, errTemplateNotFound
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("template fetch %s failed: status=%d body=%q", name, resp.StatusCode, string(b))
	}

	// If server provides Content-Length, enforce early.
	if resp.ContentLength > maxBytes && resp.ContentLength != -1 {
		return nil, fmt.Errorf("template %s exceeds limit: %d bytes > %d bytes", name, resp.ContentLength, maxBytes)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("template %s exceeds limit: %d bytes > %d bytes", name, len(b), maxBytes)
	}

	return b, nil
}

func stripHTML(raw string) string {
	if raw == "" {
		return ""
	}
	re := regexp.MustCompile(`(?s)<[^>]*>`)
	text := re.ReplaceAllString(raw, "")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	return strings.TrimSpace(text)
}

// ExtractImgSrcs finds image URLs in HTML (simple, email-safe approach).
var imgSrcRe = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)

func ExtractImgSrcs(html string) []string {
	matches := imgSrcRe.FindAllStringSubmatch(html, -1)
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{})
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		u := strings.TrimSpace(m[1])
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

func hostAllowed(host string, allowed []string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	for _, a := range allowed {
		if a == h {
			return true
		}
	}
	return false
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}
