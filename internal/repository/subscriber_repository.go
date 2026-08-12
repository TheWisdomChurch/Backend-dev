package repository

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type SubscriberRepository struct {
	db *database.Database
}

func NewSubscriberRepository(db *database.Database) *SubscriberRepository {
	return &SubscriberRepository{db: db}
}

func (r *SubscriberRepository) Create(sub *models.Subscriber) error {
	return r.db.Create(sub).Error
}

func (r *SubscriberRepository) Update(sub *models.Subscriber) error {
	return r.db.Save(sub).Error
}

func (r *SubscriberRepository) GetByEmail(email string) (*models.Subscriber, error) {
	var sub models.Subscriber
	normalized := strings.ToLower(strings.TrimSpace(email))
	if err := r.db.First(&sub, "email = ?", normalized).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (r *SubscriberRepository) List(offset, limit int, status, search, source string) ([]models.Subscriber, int64, error) {
	var items []models.Subscriber
	var total int64

	query := r.db.Model(&models.Subscriber{})
	query = applySubscriberFilters(query, status, search, source)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *SubscriberRepository) ListActive() ([]models.Subscriber, error) {
	var items []models.Subscriber
	if err := r.db.Where("status = ?", models.SubscriberStatusActive).
		Order("created_at desc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *SubscriberRepository) ListUnsubscribedEmails() ([]string, error) {
	var emails []string
	err := r.db.Model(&models.Subscriber{}).Where("status = ?", models.SubscriberStatusUnsubscribed).Pluck("email", &emails).Error
	return emails, err
}

func (r *SubscriberRepository) GetSummary() (*models.SubscriberSummary, error) {
	summary := &models.SubscriberSummary{}

	if err := r.db.Model(&models.Subscriber{}).Count(&summary.Total).Error; err != nil {
		return nil, err
	}
	if err := r.db.Model(&models.Subscriber{}).Where("status = ?", models.SubscriberStatusActive).Count(&summary.Active).Error; err != nil {
		return nil, err
	}
	if err := r.db.Model(&models.Subscriber{}).Where("status = ?", models.SubscriberStatusUnsubscribed).Count(&summary.Unsubscribed).Error; err != nil {
		return nil, err
	}

	windowStart := time.Now().UTC().AddDate(0, 0, -30)
	if err := r.db.Model(&models.Subscriber{}).Where("created_at >= ?", windowStart).Count(&summary.RecentlyAdded30d).Error; err != nil {
		return nil, err
	}
	if err := r.db.Model(&models.Subscriber{}).Where("unsubscribed_at IS NOT NULL AND unsubscribed_at >= ?", windowStart).Count(&summary.RecentlyOptedOut30d).Error; err != nil {
		return nil, err
	}

	var latest models.Subscriber
	if err := r.db.Where("last_notified_at IS NOT NULL").Order("last_notified_at desc").Limit(1).First(&latest).Error; err == nil {
		summary.LastNotifiedAt = latest.LastNotifiedAt
	}

	return summary, nil
}

func applySubscriberFilters(q *gorm.DB, status, search, source string) *gorm.DB {
	if status = strings.TrimSpace(strings.ToLower(status)); status != "" {
		switch status {
		case string(models.SubscriberStatusActive), string(models.SubscriberStatusUnsubscribed):
			q = q.Where("status = ?", status)
		}
	}

	if source = strings.TrimSpace(strings.ToLower(source)); source != "" {
		q = q.Where("LOWER(COALESCE(source, '')) = ?", source)
	}

	if search = strings.TrimSpace(strings.ToLower(search)); search != "" {
		like := "%" + search + "%"
		q = q.Where("LOWER(email) LIKE ? OR LOWER(COALESCE(name, '')) LIKE ?", like, like)
	}
	return q
}
