package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

/* ============================================================================

   Google OAuth

============================================================================ */

type googleOAuthState struct {
	State      string    `json:"state"`
	RememberMe bool      `json:"rememberMe"`
	IssuedAt   time.Time `json:"issuedAt"`
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type googleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
}

func (h *AuthHandler) StartGoogleOAuth(c *gin.Context) {
	if !h.googleOAuthEnabled() {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "Google sign-in is not configured")
		return
	}
	if h.protector == nil {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "Authentication security is not configured")
		return
	}

	stateValue, err := generateOAuthStateValue()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to start Google sign-in")
		return
	}

	payload := googleOAuthState{
		State:      stateValue,
		RememberMe: strings.EqualFold(strings.TrimSpace(c.Query("rememberMe")), "true"),
		IssuedAt:   time.Now().UTC(),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to start Google sign-in")
		return
	}

	h.setOAuthStateCookie(c, h.protector.SignPayload(raw))

	query := url.Values{}
	query.Set("client_id", h.googleClientID)
	query.Set("redirect_uri", h.googleRedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", "openid email profile")
	query.Set("state", stateValue)
	query.Set("prompt", "select_account")
	if h.googleHostedDomain != "" {
		query.Set("hd", h.googleHostedDomain)
	}

	c.Redirect(http.StatusFound, "https://accounts.google.com/o/oauth2/v2/auth?"+query.Encode())
}

func (h *AuthHandler) HandleGoogleOAuthCallback(c *gin.Context) {
	if !h.googleOAuthEnabled() {
		h.renderOAuthError(c, http.StatusServiceUnavailable, "Google sign-in is not configured")
		return
	}
	if h.protector == nil {
		h.renderOAuthError(c, http.StatusServiceUnavailable, "Authentication security is not configured")
		return
	}
	if errParam := strings.TrimSpace(c.Query("error")); errParam != "" {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in was cancelled or denied")
		return
	}

	state, err := h.readOAuthState(c)
	if err != nil {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in session is invalid or expired")
		return
	}
	if strings.TrimSpace(c.Query("state")) != state.State {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in validation failed")
		return
	}

	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in did not return an authorization code")
		return
	}

	userInfo, err := h.fetchGoogleUserInfo(code)
	h.clearOAuthStateCookie(c)
	if err != nil {
		h.renderOAuthError(c, http.StatusBadGateway, "Failed to verify the Google account")
		return
	}
	if !userInfo.EmailVerified {
		h.renderOAuthError(c, http.StatusForbidden, "Google email address is not verified")
		return
	}

	result, err := h.service.CompleteGoogleLogin(
		userInfo.Sub,
		userInfo.Email,
		userInfo.GivenName,
		userInfo.FamilyName,
		h.loginMetadata(c),
	)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrAdminPending) {
			status = http.StatusForbidden
		}
		h.clearAuthCookie(c)
		h.renderOAuthError(c, status, err.Error())
		return
	}

	if result == nil || result.User == nil {
		h.clearAuthCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in did not return a valid user")
		return
	}
	if isAdminAccessRole(result.User.Role) && !result.User.AdminApproved {
		h.clearAuthCookie(c)
		h.renderOAuthError(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.")
		return
	}

	if result.OTPRequired {
		h.renderOAuthMFAPage(c, result, state.RememberMe)
		return
	}

	if err := h.issueAuthenticatedSession(c, result.User, state.RememberMe, result.AuthMethod); err != nil {
		h.clearAuthCookie(c)
		if errors.Is(err, service.ErrAdminPending) {
			h.renderOAuthError(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.")
			return
		}
		h.renderOAuthError(c, http.StatusInternalServerError, "Failed to create sign-in session")
		return
	}

	redirectURL := h.effectivePostLoginRedirectURL()
	if redirectURL != "" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	h.renderOAuthSuccess(c, "Google sign-in completed successfully.")
}

func (h *AuthHandler) googleOAuthEnabled() bool {
	return h.googleClientID != "" && h.googleClientSecret != "" && h.googleRedirectURL != ""
}

func (h *AuthHandler) setOAuthStateCookie(c *gin.Context, value string) {
	h.cookies.SetOAuthState(c.Writer, value)
}

func (h *AuthHandler) clearOAuthStateCookie(c *gin.Context) {
	h.cookies.ClearOAuthState(c.Writer)
}

func (h *AuthHandler) readOAuthState(c *gin.Context) (*googleOAuthState, error) {
	cookieValue, err := latestCookieValue(c, oauthStateCookieName)
	if err != nil || strings.TrimSpace(cookieValue) == "" {
		return nil, errors.New("missing oauth state")
	}

	payload, err := h.protector.VerifySignedPayload(cookieValue)
	if err != nil {
		return nil, err
	}

	var state googleOAuthState
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.State) == "" || time.Since(state.IssuedAt) > 10*time.Minute {
		return nil, errors.New("oauth state expired")
	}

	return &state, nil
}

func (h *AuthHandler) fetchGoogleUserInfo(code string) (*googleUserInfo, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", h.googleClientID)
	form.Set("client_secret", h.googleClientSecret)
	form.Set("redirect_uri", h.googleRedirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequest(http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("google token exchange failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenPayload googleTokenResponse
	if err := json.Unmarshal(body, &tokenPayload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return nil, errors.New("google access token missing")
	}

	userReq, err := http.NewRequest(http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)

	userResp, err := h.httpClient.Do(userReq)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()

	if userResp.StatusCode < 200 || userResp.StatusCode >= 300 {
		return nil, errors.New("google user info request failed")
	}

	userBody, err := io.ReadAll(userResp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo googleUserInfo
	if err := json.Unmarshal(userBody, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

func (h *AuthHandler) renderOAuthMFAPage(c *gin.Context, result *service.LoginResult, rememberMe bool) {
	if result == nil || result.User == nil {
		h.renderOAuthError(c, http.StatusBadRequest, "Additional verification could not be started")
		return
	}

	redirectURL := h.effectivePostLoginRedirectURL()
	methodLabel := "verification code"
	instructions := "Enter the code sent to your email to complete sign-in."
	if strings.TrimSpace(result.MFAMethod) == "totp" {
		methodLabel = "authenticator code"
		instructions = "Open your authenticator app and enter the current 6-digit code."
	}

	page := oauthMFAPageTemplate{
		Title:        "Complete Sign-in",
		Instructions: instructions,
		Email:        result.User.Email,
		Purpose:      result.OTPPurpose,
		Method:       result.MFAMethod,
		RememberMe:   rememberMe,
		RedirectURL:  redirectURL,
		InputLabel:   methodLabel,
	}

	renderOAuthTemplate(c, http.StatusAccepted, oauthMFATemplate, page)
}

func (h *AuthHandler) renderOAuthSuccess(c *gin.Context, message string) {
	renderOAuthTemplate(c, http.StatusOK, oauthMessageTemplate, oauthMessagePage{
		Title:       "Sign-in Complete",
		Message:     message,
		RedirectURL: h.effectivePostLoginRedirectURL(),
		IsError:     false,
	})
}

func (h *AuthHandler) renderOAuthError(c *gin.Context, status int, message string) {
	renderOAuthTemplate(c, status, oauthMessageTemplate, oauthMessagePage{
		Title:       "Sign-in Unavailable",
		Message:     message,
		RedirectURL: h.effectivePostLoginRedirectURL(),
		IsError:     true,
	})
}

type oauthMessagePage struct {
	Title       string
	Message     string
	RedirectURL string
	IsError     bool
}

type oauthMFAPageTemplate struct {
	Title        string
	Instructions string
	Email        string
	Purpose      string
	Method       string
	InputLabel   string
	RememberMe   bool
	RedirectURL  string
}

func renderOAuthTemplate(c *gin.Context, status int, markup string, data any) {
	tpl := template.Must(template.New("oauth-page").Parse(markup))
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	_ = tpl.Execute(c.Writer, data)
}

const oauthMessageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { margin: 0; font-family: "Segoe UI", Tahoma, Arial, sans-serif; background: linear-gradient(180deg, #eef4fb 0%, #f7fafc 100%); color: #0f172a; }
    main { max-width: 760px; margin: 80px auto; padding: 32px; background: #fff; border: 1px solid #dbe3ef; border-radius: 24px; box-shadow: 0 18px 50px rgba(15,23,42,.08); }
    h1 { margin: 0 0 12px; font-size: 34px; }
    p { margin: 0; line-height: 1.7; color: #475569; }
    a { display: inline-block; margin-top: 20px; padding: 12px 18px; border-radius: 999px; text-decoration: none; background: {{if .IsError}}#ffffff{{else}}#0f4c81{{end}}; color: {{if .IsError}}#0f4c81{{else}}#ffffff{{end}}; border: 1px solid #0f4c81; font-weight: 700; }
  </style>
</head>
<body>
  <main>
    <h1>{{.Title}}</h1>
    <p>{{.Message}}</p>
    {{if .RedirectURL}}<a href="{{.RedirectURL}}">Continue</a>{{end}}
  </main>
</body>
</html>`

const oauthMFATemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { margin: 0; font-family: "Segoe UI", Tahoma, Arial, sans-serif; background: linear-gradient(180deg, #eef4fb 0%, #f7fafc 100%); color: #0f172a; }
    main { max-width: 760px; margin: 80px auto; padding: 32px; background: #fff; border: 1px solid #dbe3ef; border-radius: 24px; box-shadow: 0 18px 50px rgba(15,23,42,.08); }
    h1 { margin: 0 0 12px; font-size: 34px; }
    p { margin: 0 0 20px; line-height: 1.7; color: #475569; }
    label { display: block; margin-bottom: 8px; font-weight: 700; }
    input { width: 100%; box-sizing: border-box; border: 1px solid #cbd5e1; border-radius: 16px; padding: 14px 16px; font: inherit; }
    button { margin-top: 16px; border: 0; border-radius: 999px; padding: 14px 22px; background: #0f4c81; color: #fff; font: inherit; font-weight: 700; cursor: pointer; }
    .status { margin-top: 18px; padding: 14px 16px; border-radius: 16px; display: none; }
    .status.error { display: block; background: #fef3f2; color: #b42318; border: 1px solid #fecdca; }
    .status.success { display: block; background: #ecfdf3; color: #166534; border: 1px solid #abefc6; }
  </style>
</head>
<body>
  <main>
    <h1>{{.Title}}</h1>
    <p>{{.Instructions}}</p>
    <form id="mfa-form">
      <label for="code">{{.InputLabel}}</label>
      <input id="code" name="code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" required>
      <button type="submit">Complete Sign-in</button>
    </form>
    <div id="status" class="status"></div>
  </main>
  <script>
    const form = document.getElementById('mfa-form');
    const status = document.getElementById('status');
    const redirectUrl = {{printf "%q" .RedirectURL}};
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      status.className = 'status';
      status.textContent = '';
      const code = (document.getElementById('code').value || '').trim();
      if (code.length !== 6) {
        status.className = 'status error';
        status.textContent = 'Enter the 6-digit verification code.';
        return;
      }
      const response = await fetch('/api/v1/auth/otp/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: {{printf "%q" .Email}},
          purpose: {{printf "%q" .Purpose}},
          method: {{printf "%q" .Method}},
          rememberMe: {{if .RememberMe}}true{{else}}false{{end}},
          code,
        }),
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) {
        status.className = 'status error';
        status.textContent = body.message || 'Unable to complete sign-in.';
        return;
      }
      status.className = 'status success';
      status.textContent = 'Sign-in completed successfully.';
      if (redirectUrl) {
        window.location.assign(redirectUrl);
      }
    });
  </script>
</body>
</html>`

func generateOAuthStateValue() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
