package repository

import (
	"gorm.io/gorm"
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type FormCalendarReminderRepository interface {
	Create(item *models.FormCalendarReminder) error
	Update(item *models.FormCalendarReminder) error
	GetBySubmissionID(submissionID string) (*models.FormCalendarReminder, error)
	GetBySlugAndToken(slug, token string) (*models.FormCalendarReminder, error)
	MarkOptedIn(id string, at time.Time) error
	ClaimDue(now, until time.Time, limit int, worker string) ([]models.FormCalendarReminder, error)
	MarkReminderSent(id string, at time.Time) error
	MarkReminderFailed(id string, message string) error
}

type formCalendarReminderRepository struct {
	db *database.Database
}

func NewFormCalendarReminderRepository(db *database.Database) FormCalendarReminderRepository {
	return &formCalendarReminderRepository{db: db}
}

func (r *formCalendarReminderRepository) Create(item *models.FormCalendarReminder) error {
	return r.db.DB.Create(item).Error
}

func (r *formCalendarReminderRepository) Update(item *models.FormCalendarReminder) error {
	return r.db.DB.Save(item).Error
}

func (r *formCalendarReminderRepository) GetBySubmissionID(submissionID string) (*models.FormCalendarReminder, error) {
	var row models.FormCalendarReminder
	if err := r.db.DB.
		Where("submission_id = ?", submissionID).
		First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *formCalendarReminderRepository) GetBySlugAndToken(slug, token string) (*models.FormCalendarReminder, error) {
	var row models.FormCalendarReminder
	if err := r.db.DB.
		Where("slug = ? AND calendar_token = ?", slug, token).
		First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *formCalendarReminderRepository) MarkOptedIn(id string, at time.Time) error {
	return r.db.DB.Model(&models.FormCalendarReminder{}).
		Where("id = ?", id).
		Where("opted_in_at IS NULL").
		Update("opted_in_at", at).Error
}

func (r *formCalendarReminderRepository) ClaimDue(now, until time.Time, limit int, worker string) ([]models.FormCalendarReminder, error) {
	if limit <= 0 {
		limit = 500
	}
	var rows []models.FormCalendarReminder
	err := r.db.DB.Transaction(func(tx *gorm.DB) error {
		stale := now.Add(-10 * time.Minute)
		if err := tx.Raw(`SELECT * FROM form_calendar_reminders
			WHERE opted_in_at IS NOT NULL AND reminder_sent_at IS NULL
			  AND event_starts_at > ? AND event_starts_at <= ?
			  AND delivery_attempt < 5
			  AND (delivery_status IN ('pending','failed') OR (delivery_status = 'processing' AND claimed_at < ?))
			ORDER BY event_starts_at ASC LIMIT ? FOR UPDATE SKIP LOCKED`, now, until, stale, limit).Scan(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := tx.Model(&models.FormCalendarReminder{}).Where("id = ? AND reminder_sent_at IS NULL", rows[i].ID).Updates(map[string]any{"delivery_status": "processing", "delivery_attempt": gorm.Expr("delivery_attempt + 1"), "claimed_at": now, "claimed_by": worker, "last_error": nil}).Error; err != nil {
				return err
			}
			rows[i].DeliveryStatus = "processing"
			rows[i].DeliveryAttempt++
		}
		return nil
	})
	return rows, err
}

func (r *formCalendarReminderRepository) MarkReminderSent(id string, at time.Time) error {
	return r.db.DB.Model(&models.FormCalendarReminder{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"reminder_sent_at": at,
			"delivery_status":  "provider_accepted",
			"claimed_at":       nil,
			"claimed_by":       nil,
			"last_error":       nil,
		}).Error
}

func (r *formCalendarReminderRepository) MarkReminderFailed(id string, message string) error {
	return r.db.DB.Model(&models.FormCalendarReminder{}).Where("id = ? AND reminder_sent_at IS NULL", id).Updates(map[string]any{"delivery_status": "failed", "last_error": message, "claimed_at": nil, "claimed_by": nil}).Error
}

var _ FormCalendarReminderRepository = (*formCalendarReminderRepository)(nil)
