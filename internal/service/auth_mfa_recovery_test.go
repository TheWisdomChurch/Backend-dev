package service

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type dummyUserRepo struct {
	user *models.User
}

func (r *dummyUserRepo) Create(user *models.User) error {
	r.user = user
	return nil
}
func (r *dummyUserRepo) FindByEmail(email string) (*models.User, error) {
	return r.user, nil
}
func (r *dummyUserRepo) FindByFederatedAccount(provider, subject string) (*models.User, error) {
	return nil, nil
}
func (r *dummyUserRepo) FindByID(id string) (*models.User, error) {
	return r.user, nil
}
func (r *dummyUserRepo) FindAll() ([]models.User, error) {
	return []models.User{*r.user}, nil
}
func (r *dummyUserRepo) FindByRoles(roles []string) ([]models.User, error) {
	return []models.User{*r.user}, nil
}
func (r *dummyUserRepo) Update(user *models.User) error {
	r.user = user
	return nil
}
func (r *dummyUserRepo) Delete(id string) error {
	return nil
}
func (r *dummyUserRepo) DeleteHard(id string) error {
	return nil
}
func (r *dummyUserRepo) GetTotalCount() (int64, error) {
	return 1, nil
}
func (r *dummyUserRepo) WithTx(tx *gorm.DB) repository.UserRepository {
	return r
}

func TestRecoveryCodesLifecycle(t *testing.T) {
	protector, err := authutil.NewProtector("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("failed to create protector: %v", err)
	}

	repo := &dummyUserRepo{
		user: &models.User{
			ID:          "user-123",
			Email:       "admin@example.com",
			Role:        "admin",
			IsActive:    true,
			TOTPEnabled: true,
		},
	}

	svc := &authServiceImpl{
		totpProtector: protector,
		userRepo:      repo,
	}

	// 1. Generate recovery codes
	codes, err := svc.GenerateRecoveryCodes("user-123")
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes failed: %v", err)
	}
	if len(codes) != 8 {
		t.Fatalf("expected 8 codes, got %d", len(codes))
	}

	// Verify remaining count
	if rem := svc.countRemainingRecoveryCodes(repo.user); rem != 8 {
		t.Fatalf("expected 8 remaining, got %d", rem)
	}

	// 2. Consume first code (lowercase with spaces and dashes)
	firstCode := codes[0]
	if !svc.verifyAndConsumeRecoveryCode(repo.user, " "+firstCode+" ") {
		t.Fatalf("expected first code to verify and consume")
	}

	// Check remaining is 7
	if rem := svc.countRemainingRecoveryCodes(repo.user); rem != 7 {
		t.Fatalf("expected 7 remaining, got %d", rem)
	}

	// 3. Second attempt with SAME code must fail
	if svc.verifyAndConsumeRecoveryCode(repo.user, firstCode) {
		t.Fatalf("expected already-used code to fail")
	}

	// 4. Invalid code must fail
	if svc.verifyAndConsumeRecoveryCode(repo.user, "INVALID-CODE") {
		t.Fatalf("expected invalid code to fail")
	}

	// 5. Admin reset 2FA clears all codes and TOTP
	if err := svc.AdminResetUser2FA("user-123"); err != nil {
		t.Fatalf("AdminResetUser2FA failed: %v", err)
	}
	if repo.user.TOTPEnabled {
		t.Fatalf("expected TOTPEnabled to be false after reset")
	}
	if repo.user.TOTPRecoveryCodesEnc != nil {
		t.Fatalf("expected recovery codes to be nil after reset")
	}
	if repo.user.TOTPSecretEnc != nil {
		t.Fatalf("expected secret to be nil after reset")
	}
}

func TestStoreAndLoadRecoveryCodes(t *testing.T) {
	protector, err := authutil.NewProtector("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("failed to create protector: %v", err)
	}

	svc := &authServiceImpl{totpProtector: protector}
	user := &models.User{}

	records := []RecoveryCodeRecord{
		{Code: "AAAA-1111", Used: false},
		{Code: "BBBB-2222", Used: true, UsedAt: func() *time.Time { now := time.Now().UTC(); return &now }()},
	}

	if err := svc.storeRecoveryCodes(user, records); err != nil {
		t.Fatalf("storeRecoveryCodes failed: %v", err)
	}

	loaded := svc.loadRecoveryCodes(user)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded records, got %d", len(loaded))
	}
	if loaded[0].Code != "AAAA-1111" || loaded[0].Used != false {
		t.Fatalf("unexpected record 0: %+v", loaded[0])
	}
	if loaded[1].Code != "BBBB-2222" || loaded[1].Used != true || loaded[1].UsedAt == nil {
		t.Fatalf("unexpected record 1: %+v", loaded[1])
	}
}
