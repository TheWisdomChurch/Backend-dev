package repository

import (
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
	ListDue(now, until time.Time, limit int) ([]models.FormCalendarReminder, error)
	MarkReminderSent(id string, at time.Time) error
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

func (r *formCalendarReminderRepository) ListDue(now, until time.Time, limit int) ([]models.FormCalendarReminder, error) {
	if limit <= 0 {
		limit = 500
	}
	var rows []models.FormCalendarReminder
	err := r.db.DB.
		Where("opted_in_at IS NOT NULL").
		Where("reminder_sent_at IS NULL").
		Where("event_starts_at > ? AND event_starts_at <= ?", now, until).
		Order("event_starts_at ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *formCalendarReminderRepository) MarkReminderSent(id string, at time.Time) error {
	return r.db.DB.Model(&models.FormCalendarReminder{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"reminder_sent_at": at,
		}).Error
}

var _ FormCalendarReminderRepository = (*formCalendarReminderRepository)(nil)
