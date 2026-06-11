package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type RefreshTokenRepository interface {
	// Create persists a new refresh token.
	Create(token *models.RefreshToken) error
	// FindByHash returns the token matching the given SHA-256 hash, or nil if not found.
	FindByHash(hash string) (*models.RefreshToken, error)
	// RevokeByID marks a single token as revoked.
	RevokeByID(id string) error
	// RevokeFamily marks all tokens in a family as revoked (replay attack response).
	RevokeFamily(familyID string) error
	// RevokeAllForUser revokes every active token for a user (forced global logout).
	RevokeAllForUser(userID string) error
	// DeleteExpired removes tokens that have passed their expiry (housekeeping).
	DeleteExpired() error
}

type refreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *database.Database) RefreshTokenRepository {
	return &refreshTokenRepository{db: db.DB}
}

func (r *refreshTokenRepository) Create(token *models.RefreshToken) error {
	return r.db.Create(token).Error
}

func (r *refreshTokenRepository) FindByHash(hash string) (*models.RefreshToken, error) {
	var token models.RefreshToken
	err := r.db.Where("token_hash = ?", hash).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &token, nil
}

func (r *refreshTokenRepository) RevokeByID(id string) error {
	now := time.Now().UTC()
	return r.db.Model(&models.RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", now).Error
}

func (r *refreshTokenRepository) RevokeFamily(familyID string) error {
	now := time.Now().UTC()
	return r.db.Model(&models.RefreshToken{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", now).Error
}

func (r *refreshTokenRepository) RevokeAllForUser(userID string) error {
	now := time.Now().UTC()
	return r.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}

func (r *refreshTokenRepository) DeleteExpired() error {
	return r.db.
		Where("expires_at < ?", time.Now().UTC()).
		Delete(&models.RefreshToken{}).Error
}
