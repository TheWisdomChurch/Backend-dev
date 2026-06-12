package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service/payment"
)

type InitiateGivingRequest struct {
	CategoryID  string  `json:"category_id"`
	AmountKobo  int64   `json:"amount_kobo"`
	Currency    string  `json:"currency"`
	GiverName   string  `json:"giver_name"`
	GiverEmail  string  `json:"giver_email"`
	MemberID    *string `json:"member_id,omitempty"`
	CampusID    *string `json:"campus_id,omitempty"`
	CallbackURL string  `json:"callback_url"`
}

type InitiateGivingResponse struct {
	CheckoutURL string `json:"checkout_url"`
	Reference   string `json:"reference"`
}

type GivingService interface {
	ListCategories(ctx context.Context) ([]models.GivingCategory, error)
	Initiate(ctx context.Context, provider string, req InitiateGivingRequest) (*InitiateGivingResponse, error)
	VerifyAndRecord(ctx context.Context, provider, reference string) (*models.GivingTransaction, error)
	HandleWebhook(ctx context.Context, provider string, signature string, rawBody []byte) error
	List(ctx context.Context, filter repository.GivingFilter, limit, offset int) ([]models.GivingTransaction, int64, error)
	MonthlySummary(ctx context.Context, year, month int, campusID *string) ([]models.GivingMonthlySummary, error)
}

type givingService struct {
	repo      repository.GivingRepository
	providers map[string]payment.Provider
}

func NewGivingService(repo repository.GivingRepository, providers map[string]payment.Provider) GivingService {
	return &givingService{repo: repo, providers: providers}
}

func (s *givingService) ListCategories(ctx context.Context) ([]models.GivingCategory, error) {
	return s.repo.ListCategories(ctx)
}

func (s *givingService) Initiate(ctx context.Context, providerName string, req InitiateGivingRequest) (*InitiateGivingResponse, error) {
	if req.AmountKobo <= 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}
	if strings.TrimSpace(req.GiverEmail) == "" {
		return nil, fmt.Errorf("giver email is required")
	}

	prov, err := s.provider(providerName)
	if err != nil {
		return nil, err
	}

	ref := uuid.NewString()
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "NGN"
	}

	// Create a pending transaction record before calling the provider.
	tx := &models.GivingTransaction{
		CategoryID:      req.CategoryID,
		MemberID:        req.MemberID,
		CampusID:        req.CampusID,
		AmountKobo:      req.AmountKobo,
		Currency:        currency,
		Channel:         "online",
		PaymentRef:      ref,
		PaymentProvider: strings.ToLower(providerName),
		Status:          "pending",
		GiverName:       req.GiverName,
		GiverEmail:      req.GiverEmail,
		GivenAt:         time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("giving: create pending transaction: %w", err)
	}

	resp, err := prov.Initiate(ctx, payment.InitiateRequest{
		AmountKobo:  req.AmountKobo,
		Currency:    currency,
		Email:       req.GiverEmail,
		Name:        req.GiverName,
		Reference:   ref,
		CallbackURL: req.CallbackURL,
	})
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, tx.ID, "failed")
		return nil, fmt.Errorf("giving: provider initiate: %w", err)
	}

	return &InitiateGivingResponse{
		CheckoutURL: resp.AuthorizationURL,
		Reference:   ref,
	}, nil
}

func (s *givingService) VerifyAndRecord(ctx context.Context, providerName, reference string) (*models.GivingTransaction, error) {
	prov, err := s.provider(providerName)
	if err != nil {
		return nil, err
	}

	result, err := prov.Verify(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("giving: verify payment: %w", err)
	}

	existing, err := s.repo.FindByRef(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("giving: find transaction: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, existing.ID, result.Status); err != nil {
		return nil, fmt.Errorf("giving: update status: %w", err)
	}
	existing.Status = result.Status
	return existing, nil
}

func (s *givingService) HandleWebhook(ctx context.Context, providerName, signature string, rawBody []byte) error {
	prov, err := s.provider(providerName)
	if err != nil {
		return err
	}
	if err := prov.ValidateWebhook(signature, rawBody); err != nil {
		return fmt.Errorf("giving: webhook signature invalid: %w", err)
	}
	// Webhook events are provider-specific; parse reference and re-verify.
	// This minimal implementation delegates the full update to VerifyAndRecord
	// which is called by the handler after this returns nil.
	return nil
}

func (s *givingService) List(ctx context.Context, filter repository.GivingFilter, limit, offset int) ([]models.GivingTransaction, int64, error) {
	return s.repo.List(ctx, filter, limit, offset)
}

func (s *givingService) MonthlySummary(ctx context.Context, year, month int, campusID *string) ([]models.GivingMonthlySummary, error) {
	return s.repo.MonthlySummary(ctx, year, month, campusID)
}

func (s *givingService) provider(name string) (payment.Provider, error) {
	p, ok := s.providers[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("giving: unknown payment provider %q", name)
	}
	return p, nil
}
