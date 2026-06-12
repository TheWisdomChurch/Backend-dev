package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const paystackBaseURL = "https://api.paystack.co"

type paystackProvider struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
}

// NewPaystack returns a Provider backed by the Paystack API.
func NewPaystack(secretKey, webhookSecret string) Provider {
	return &paystackProvider{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *paystackProvider) Initiate(ctx context.Context, req InitiateRequest) (*InitiateResponse, error) {
	body := map[string]interface{}{
		"amount":       req.AmountKobo,
		"email":        req.Email,
		"reference":    req.Reference,
		"currency":     req.Currency,
		"callback_url": req.CallbackURL,
		"metadata": map[string]interface{}{
			"name":   req.Name,
			"custom": req.Metadata,
		},
	}
	raw, err := p.post(ctx, "/transaction/initialize", body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status bool `json:"status"`
		Data   struct {
			AuthorizationURL string `json:"authorization_url"`
			Reference        string `json:"reference"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("paystack: decode initiate response: %w", err)
	}
	if !resp.Status {
		return nil, fmt.Errorf("paystack: initiate failed: %s", resp.Message)
	}
	return &InitiateResponse{
		AuthorizationURL: resp.Data.AuthorizationURL,
		Reference:        resp.Data.Reference,
	}, nil
}

func (p *paystackProvider) Verify(ctx context.Context, reference string) (*VerifyResponse, error) {
	raw, err := p.get(ctx, "/transaction/verify/"+reference)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status bool `json:"status"`
		Data   struct {
			Status    string `json:"status"`
			Reference string `json:"reference"`
			Amount    int64  `json:"amount"`
			Currency  string `json:"currency"`
			Channel   string `json:"channel"`
			PaidAt    string `json:"paid_at"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("paystack: decode verify response: %w", err)
	}
	if !resp.Status {
		return nil, fmt.Errorf("paystack: verify failed: %s", resp.Message)
	}
	return &VerifyResponse{
		Reference:  resp.Data.Reference,
		Status:     normalizeStatus(resp.Data.Status),
		AmountKobo: resp.Data.Amount,
		Currency:   resp.Data.Currency,
		Channel:    resp.Data.Channel,
		PaidAt:     resp.Data.PaidAt,
	}, nil
}

func (p *paystackProvider) ValidateWebhook(signature string, rawBody []byte) error {
	mac := hmac.New(sha512.New, []byte(p.webhookSecret))
	_, _ = mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected)) {
		return fmt.Errorf("paystack: invalid webhook signature")
	}
	return nil
}

func (p *paystackProvider) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("paystack: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paystackBaseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack: HTTP POST %s: %w", path, err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func (p *paystackProvider) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, paystackBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack: HTTP GET %s: %w", path, err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func normalizeStatus(s string) string {
	switch strings.ToLower(s) {
	case "success":
		return "success"
	case "failed", "abandoned":
		return "failed"
	default:
		return "pending"
	}
}
