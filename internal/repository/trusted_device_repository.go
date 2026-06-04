package repository

import (
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TrustedDeviceRepository interface {
	Find(userID, deviceID string) (*models.TrustedDevice, error)
	Upsert(device *models.TrustedDevice) error
	MarkUntrusted(userID, deviceID string) error
}

type trustedDeviceRepository struct {
	db *gorm.DB
}

func NewTrustedDeviceRepository(db *database.Database) TrustedDeviceRepository {
	return &trustedDeviceRepository{db: db.DB}
}

func (r *trustedDeviceRepository) Find(userID, deviceID string) (*models.TrustedDevice, error) {
	var dev models.TrustedDevice
	err := r.db.Where("user_id = ? AND device_id = ? AND expires_at > ?", userID, deviceID, time.Now().UTC()).First(&dev).Error
	if err != nil {
		return nil, err
	}
	return &dev, nil
}

func (r *trustedDeviceRepository) Upsert(device *models.TrustedDevice) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_ip", "user_agent", "trusted", "expires_at", "last_seen_at", "updated_at"}),
	}).Create(device).Error
}

func (r *trustedDeviceRepository) MarkUntrusted(userID, deviceID string) error {
	return r.db.Model(&models.TrustedDevice{}).
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		Updates(map[string]interface{}{
			"trusted":      false,
			"expires_at":   time.Now().UTC(),
			"updated_at":   time.Now().UTC(),
			"last_seen_at": time.Now().UTC(),
		}).Error
}
