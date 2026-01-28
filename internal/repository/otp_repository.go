package repository

import (
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type OTPRepository struct {
	db *database.Database
}

func NewOTPRepository(db *database.Database) *OTPRepository {
	return &OTPRepository{db: db}
}

func (r *OTPRepository) Create(otp *models.OTP) error {
	return r.db.Create(otp).Error
}

func (r *OTPRepository) InvalidateActive(email, purpose string, usedAt time.Time) error {
	return r.db.Model(&models.OTP{}).
		Where("email = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?", email, purpose, time.Now().UTC()).
		Updates(map[string]interface{}{"used_at": usedAt}).Error
}

func (r *OTPRepository) GetActive(email, purpose string) (*models.OTP, error) {
	var otp models.OTP
	if err := r.db.Where("email = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?", email, purpose, time.Now().UTC()).
		Order("created_at desc").
		First(&otp).Error; err != nil {
		return nil, err
	}
	return &otp, nil
}

func (r *OTPRepository) MarkUsed(id string, usedAt time.Time) error {
	return r.db.Model(&models.OTP{}).Where("id = ?", id).
		Updates(map[string]interface{}{"used_at": usedAt}).Error
}
