package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const stripeBaseURL = "https://api.stripe.com/v1"

type stripeProvider struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
}

// NewStripe returns a Provider backed by the Stripe API.
func NewStripe(secretKey, webhookSecret string) Provider {
	return &stripeProvider{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *stripeProvider) Initiate(ctx context.Context, req InitiateRequest) (*InitiateResponse, error) {
	params := url.Values{}
	params.Set("payment_method_types[]", "card")
	params.Set("line_items[0][price_data][currency]", strings.ToLower(req.Currency))
	params.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(req.AmountKobo, 10))
	params.Set("line_items[0][price_data][product_data][name]", req.Name)
	params.Set("line_items[0][quantity]", "1")
	params.Set("mode", "payment")
	params.Set("success_url", req.CallbackURL+"?ref="+req.Reference+"&status=success")
	params.Set("cancel_url", req.CallbackURL+"?ref="+req.Reference+"&status=cancelled")
	params.Set("client_reference_id", req.Reference)
	params.Set("customer_email", req.Email)

	raw, err := p.post(ctx, "/checkout/sessions", params)
	if err != nil {
		return nil, err
	}

	var resp struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("stripe: decode checkout session: %w", err)
	}
	return &InitiateResponse{
		AuthorizationURL: resp.URL,
		Reference:        resp.ID,
	}, nil
}

func (p *stripeProvider) Verify(ctx context.Context, reference string) (*VerifyResponse, error) {
	raw, err := p.get(ctx, "/payment_intents/"+reference)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status   string `json:"status"`
		ID       string `json:"id"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("stripe: decode payment intent: %w", err)
	}
	status := "pending"
	if resp.Status == "succeeded" {
		status = "success"
	} else if resp.Status == "canceled" || resp.Status == "requires_payment_method" {
		status = "failed"
	}
	return &VerifyResponse{
		Reference:  resp.ID,
		Status:     status,
		AmountKobo: resp.Amount,
		Currency:   strings.ToUpper(resp.Currency),
	}, nil
}

// ValidateWebhook verifies a Stripe-Signature header against the webhook secret.
// rawBody must be the unmodified request body.
func (p *stripeProvider) ValidateWebhook(signature string, rawBody []byte) error {
	// Stripe-Signature: t=<ts>,v1=<sig>
	parts := strings.Split(signature, ",")
	ts := ""
	var v1sigs []string
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			v1sigs = append(v1sigs, kv[1])
		}
	}
	if ts == "" || len(v1sigs) == 0 {
		return fmt.Errorf("stripe: malformed Stripe-Signature header")
	}

	payload := ts + "." + string(rawBody)
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	_, _ = mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range v1sigs {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}
	return fmt.Errorf("stripe: invalid webhook signature")
}

func (p *stripeProvider) post(ctx context.Context, path string, params url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stripeBaseURL+path,
		bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: HTTP POST %s: %w", path, err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func (p *stripeProvider) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stripeBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.secretKey, "")

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: HTTP GET %s: %w", path, err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}
