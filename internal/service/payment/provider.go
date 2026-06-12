package payment

import "context"

// InitiateRequest carries the parameters needed to start a payment.
type InitiateRequest struct {
	AmountKobo int64  // amount in lowest currency unit (kobo/cents)
	Currency   string // e.g. "NGN", "USD"
	Email      string
	Name       string
	Reference  string // unique idempotency key
	CallbackURL string
	Metadata   map[string]string
}

// InitiateResponse is the provider-specific checkout URL or charge reference.
type InitiateResponse struct {
	AuthorizationURL string // redirect the user here for hosted checkout
	Reference        string // provider-assigned reference
}

// VerifyResponse is the normalized result of a payment verification call.
type VerifyResponse struct {
	Reference string
	Status    string // "success" | "failed" | "pending"
	AmountKobo int64
	Currency  string
	Channel   string
	PaidAt    string
}

// Provider is the common interface implemented by Paystack, Stripe, etc.
type Provider interface {
	// Initiate starts a payment and returns a hosted checkout URL.
	Initiate(ctx context.Context, req InitiateRequest) (*InitiateResponse, error)
	// Verify checks whether a reference has been paid.
	Verify(ctx context.Context, reference string) (*VerifyResponse, error)
	// ValidateWebhook verifies the webhook signature and returns the raw event body.
	// rawBody must be the unmodified request body read before JSON parsing.
	ValidateWebhook(signature string, rawBody []byte) error
}
