package service

import (
	"context"
	"errors"
	"testing"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service/payment"
)

// --- mock repository --------------------------------------------------------

type mockGivingRepo struct {
	categories []models.GivingCategory

	created       []*models.GivingTransaction
	createErr     error
	byRef         map[string]*models.GivingTransaction
	findByRefErr  error
	updateStatus  map[string]string // id -> status set via UpdateStatus
	updateErr     error
	listResult    []models.GivingTransaction
	listTotal     int64
	listErr       error
	summaryResult []models.GivingMonthlySummary
}

func newMockGivingRepo() *mockGivingRepo {
	return &mockGivingRepo{
		byRef:        map[string]*models.GivingTransaction{},
		updateStatus: map[string]string{},
	}
}

func (m *mockGivingRepo) CreateCategory(ctx context.Context, cat *models.GivingCategory) error {
	return nil
}

func (m *mockGivingRepo) ListCategories(ctx context.Context) ([]models.GivingCategory, error) {
	return m.categories, nil
}

func (m *mockGivingRepo) Create(ctx context.Context, tx *models.GivingTransaction) error {
	if m.createErr != nil {
		return m.createErr
	}
	if tx.ID == "" {
		tx.ID = "tx-" + tx.PaymentRef
	}
	m.created = append(m.created, tx)
	m.byRef[tx.PaymentRef] = tx
	return nil
}

func (m *mockGivingRepo) FindByRef(ctx context.Context, ref string) (*models.GivingTransaction, error) {
	if m.findByRefErr != nil {
		return nil, m.findByRefErr
	}
	tx, ok := m.byRef[ref]
	if !ok {
		return nil, errors.New("not found")
	}
	return tx, nil
}

func (m *mockGivingRepo) UpdateStatus(ctx context.Context, id, status string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateStatus[id] = status
	return nil
}

func (m *mockGivingRepo) List(ctx context.Context, filter repository.GivingFilter, limit, offset int) ([]models.GivingTransaction, int64, error) {
	return m.listResult, m.listTotal, m.listErr
}

func (m *mockGivingRepo) MonthlySummary(ctx context.Context, year, month int, campusID *string) ([]models.GivingMonthlySummary, error) {
	return m.summaryResult, nil
}

// --- mock payment provider ---------------------------------------------------

type mockProvider struct {
	initiateResp   *payment.InitiateResponse
	initiateErr    error
	verifyResp     *payment.VerifyResponse
	verifyErr      error
	webhookErr     error
	initiateCalled payment.InitiateRequest
}

func (m *mockProvider) Initiate(ctx context.Context, req payment.InitiateRequest) (*payment.InitiateResponse, error) {
	m.initiateCalled = req
	if m.initiateErr != nil {
		return nil, m.initiateErr
	}
	return m.initiateResp, nil
}

func (m *mockProvider) Verify(ctx context.Context, reference string) (*payment.VerifyResponse, error) {
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	return m.verifyResp, nil
}

func (m *mockProvider) ValidateWebhook(signature string, rawBody []byte) error {
	return m.webhookErr
}

// --- tests --------------------------------------------------------------

func TestGivingService_Initiate_RejectsNonPositiveAmount(t *testing.T) {
	repo := newMockGivingRepo()
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": &mockProvider{}})

	_, err := svc.Initiate(context.Background(), "paystack", InitiateGivingRequest{
		AmountKobo: 0,
		GiverEmail: "donor@example.com",
	})
	if err == nil {
		t.Fatal("expected error for zero amount, got nil")
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected no transaction to be created, got %d", len(repo.created))
	}
}

func TestGivingService_Initiate_RejectsMissingEmail(t *testing.T) {
	repo := newMockGivingRepo()
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": &mockProvider{}})

	_, err := svc.Initiate(context.Background(), "paystack", InitiateGivingRequest{
		AmountKobo: 100_000,
		GiverEmail: "   ",
	})
	if err == nil {
		t.Fatal("expected error for missing giver email, got nil")
	}
}

func TestGivingService_Initiate_RejectsUnknownProvider(t *testing.T) {
	repo := newMockGivingRepo()
	svc := NewGivingService(repo, map[string]payment.Provider{})

	_, err := svc.Initiate(context.Background(), "unknown", InitiateGivingRequest{
		AmountKobo: 100_000,
		GiverEmail: "donor@example.com",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected no transaction to be created for an unknown provider, got %d", len(repo.created))
	}
}

func TestGivingService_Initiate_HappyPath(t *testing.T) {
	repo := newMockGivingRepo()
	prov := &mockProvider{
		initiateResp: &payment.InitiateResponse{AuthorizationURL: "https://checkout.example.com/abc"},
	}
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": prov})

	resp, err := svc.Initiate(context.Background(), "PAYSTACK", InitiateGivingRequest{
		AmountKobo: 250_000,
		Currency:   "ngn",
		GiverName:  "Jane Doe",
		GiverEmail: "jane@example.com",
	})
	if err != nil {
		t.Fatalf("Initiate returned error: %v", err)
	}
	if resp.CheckoutURL != "https://checkout.example.com/abc" {
		t.Errorf("CheckoutURL = %q, want provider's authorization URL", resp.CheckoutURL)
	}
	if resp.Reference == "" {
		t.Error("expected a non-empty reference")
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected exactly one transaction to be created, got %d", len(repo.created))
	}
	created := repo.created[0]
	if created.Status != "pending" {
		t.Errorf("created transaction status = %q, want pending", created.Status)
	}
	if created.Currency != "NGN" {
		t.Errorf("created transaction currency = %q, want normalized NGN", created.Currency)
	}
	if created.PaymentProvider != "paystack" {
		t.Errorf("created transaction provider = %q, want lowercased paystack", created.PaymentProvider)
	}

	// The provider should have been called with the same reference the
	// transaction was recorded under, so verification can find it later.
	if prov.initiateCalled.Reference != resp.Reference {
		t.Errorf("provider called with reference %q, want %q", prov.initiateCalled.Reference, resp.Reference)
	}
}

func TestGivingService_Initiate_MarksTransactionFailedWhenProviderErrors(t *testing.T) {
	repo := newMockGivingRepo()
	prov := &mockProvider{initiateErr: errors.New("provider unreachable")}
	svc := NewGivingService(repo, map[string]payment.Provider{"stripe": prov})

	_, err := svc.Initiate(context.Background(), "stripe", InitiateGivingRequest{
		AmountKobo: 100_000,
		GiverEmail: "donor@example.com",
	})
	if err == nil {
		t.Fatal("expected error when provider.Initiate fails, got nil")
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected the pending transaction to still have been created, got %d", len(repo.created))
	}
	createdID := repo.created[0].ID
	if got := repo.updateStatus[createdID]; got != "failed" {
		t.Errorf("transaction status after provider failure = %q, want failed", got)
	}
}

func TestGivingService_VerifyAndRecord_UpdatesStatusFromProvider(t *testing.T) {
	repo := newMockGivingRepo()
	repo.byRef["REF-123"] = &models.GivingTransaction{ID: "tx-1", PaymentRef: "REF-123", Status: "pending"}
	prov := &mockProvider{verifyResp: &payment.VerifyResponse{Reference: "REF-123", Status: "success"}}
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": prov})

	tx, err := svc.VerifyAndRecord(context.Background(), "paystack", "REF-123")
	if err != nil {
		t.Fatalf("VerifyAndRecord returned error: %v", err)
	}
	if tx.Status != "success" {
		t.Errorf("returned transaction status = %q, want success", tx.Status)
	}
	if got := repo.updateStatus["tx-1"]; got != "success" {
		t.Errorf("repo.UpdateStatus called with status %q, want success", got)
	}
}

func TestGivingService_VerifyAndRecord_PropagatesProviderVerifyError(t *testing.T) {
	repo := newMockGivingRepo()
	prov := &mockProvider{verifyErr: errors.New("verify failed")}
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": prov})

	_, err := svc.VerifyAndRecord(context.Background(), "paystack", "REF-404")
	if err == nil {
		t.Fatal("expected error when provider.Verify fails, got nil")
	}
}

func TestGivingService_HandleWebhook_RejectsInvalidSignature(t *testing.T) {
	repo := newMockGivingRepo()
	prov := &mockProvider{webhookErr: errors.New("signature mismatch")}
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": prov})

	err := svc.HandleWebhook(context.Background(), "paystack", "bad-signature", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid webhook signature, got nil")
	}
}

func TestGivingService_HandleWebhook_AcceptsValidSignature(t *testing.T) {
	repo := newMockGivingRepo()
	prov := &mockProvider{}
	svc := NewGivingService(repo, map[string]payment.Provider{"paystack": prov})

	if err := svc.HandleWebhook(context.Background(), "paystack", "good-signature", []byte(`{}`)); err != nil {
		t.Fatalf("HandleWebhook returned error for a valid signature: %v", err)
	}
}

func TestGivingService_List_DelegatesToRepository(t *testing.T) {
	repo := newMockGivingRepo()
	repo.listResult = []models.GivingTransaction{{ID: "tx-1"}, {ID: "tx-2"}}
	repo.listTotal = 2
	svc := NewGivingService(repo, nil)

	items, total, err := svc.List(context.Background(), repository.GivingFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("List = (%d items, total %d), want (2, 2)", len(items), total)
	}
}
