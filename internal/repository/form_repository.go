// internal/repository/form_repository.go
package repository

import (
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
)

type FormRepository interface {
	List(offset, limit int) ([]models.Form, int64, error)
	GetByID(id string) (*models.Form, error)
	GetBySlug(slug string) (*models.Form, error)

	Create(form *models.Form) error
	Update(form *models.Form) error
	Delete(id string) error

	SlugExists(slug string) (bool, error)

	ReplaceFields(formID string, fields []models.FormField) error

	CountSubmissions(formID string) (int64, error)
	CreateSubmission(sub *models.FormSubmission) error

	ListSubmissions(formID string, offset, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error)
	ListRecentSubmissions(limit int, start, end *time.Time) ([]models.FormSubmissionWithForm, error)
	CountSubmissionsByForm(start, end *time.Time) ([]models.FormSubmissionCount, error)
	CountSubmissionsFiltered(formID string, start, end *time.Time) (int64, error)
	DeleteExpired(now time.Time) (int64, error)
}

type formRepository struct {
	db *database.Database
}

func NewFormRepository(db *database.Database) FormRepository {
	return &formRepository{db: db}
}

func (r *formRepository) List(offset, limit int) ([]models.Form, int64, error) {
	var items []models.Form
	var total int64

	q := r.db.DB.Model(&models.Form{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.DB.
		Preload("Fields", func(db *gorm.DB) *gorm.DB {
			// Keep ordering stable for the UI
			return db.Order(`"order" ASC`)
		}).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error

	return items, total, err
}

func (r *formRepository) GetByID(id string) (*models.Form, error) {
	var f models.Form
	err := r.db.DB.Preload("Fields", "deleted_at IS NULL").First(&f, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *formRepository) GetBySlug(slug string) (*models.Form, error) {
	var f models.Form
	err := r.db.DB.Preload("Fields", "deleted_at IS NULL").
		First(&f, "slug = ? AND is_published = true", slug).Error
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *formRepository) Create(form *models.Form) error {
	return r.db.DB.Create(form).Error
}

func (r *formRepository) Update(form *models.Form) error {
	return r.db.DB.Save(form).Error
}

func (r *formRepository) Delete(id string) error {
	return r.db.DB.Delete(&models.Form{}, "id = ?", id).Error
}

func (r *formRepository) DeleteExpired(now time.Time) (int64, error) {
	result := r.db.DB.Model(&models.Form{}).
		Where("deleted_at IS NULL").
		Where("settings ? 'expiresAt'").
		Where("NULLIF(settings->>'expiresAt','') IS NOT NULL").
		Where("(settings->>'expiresAt')::timestamptz <= ?", now).
		Delete(&models.Form{})
	return result.RowsAffected, result.Error
}

func (r *formRepository) SlugExists(slug string) (bool, error) {
	var count int64
	if err := r.db.DB.Model(&models.Form{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *formRepository) ReplaceFields(formID string, fields []models.FormField) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("form_id = ?", formID).Delete(&models.FormField{}).Error; err != nil {
			return err
		}

		for i := range fields {
			fields[i].FormID = formID
		}

		if len(fields) > 0 {
			if err := tx.Create(&fields).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *formRepository) CountSubmissions(formID string) (int64, error) {
	var count int64
	err := r.db.DB.Model(&models.FormSubmission{}).
		Where("form_id = ?", formID).
		Count(&count).Error
	return count, err
}

func (r *formRepository) CreateSubmission(sub *models.FormSubmission) error {
	return r.db.DB.Create(sub).Error
}

func applySubmissionFilters(q *gorm.DB, formID string, start, end *time.Time) *gorm.DB {
	if formID != "" {
		q = q.Where("form_id = ?", formID)
	}
	if start != nil {
		q = q.Where("created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("created_at <= ?", *end)
	}
	return q
}

func (r *formRepository) ListSubmissions(formID string, offset, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error) {
	var items []models.FormSubmission
	var total int64

	q := applySubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), formID, start, end)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error
	return items, total, err
}

func (r *formRepository) ListRecentSubmissions(limit int, start, end *time.Time) ([]models.FormSubmissionWithForm, error) {
	var items []models.FormSubmissionWithForm

	q := applySubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), "", start, end)
	err := q.Select(`form_submissions.id,
		form_submissions.form_id,
		COALESCE(forms.title, '') as form_title,
		form_submissions.name,
		form_submissions.email,
		form_submissions.contact_number,
		form_submissions.contact_address,
		form_submissions.values,
		form_submissions.created_at`).
		Joins("LEFT JOIN forms ON forms.id = form_submissions.form_id").
		Order("form_submissions.created_at DESC").
		Limit(limit).
		Scan(&items).Error

	return items, err
}

func (r *formRepository) CountSubmissionsByForm(start, end *time.Time) ([]models.FormSubmissionCount, error) {
	var rows []models.FormSubmissionCount

	q := applySubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), "", start, end)
	err := q.Select("form_id as form_id, COALESCE(forms.title,'') as form_title, COUNT(*) as count").
		Joins("LEFT JOIN forms ON forms.id = form_submissions.form_id").
		Group("form_id, forms.title").
		Order("count DESC").
		Scan(&rows).Error

	return rows, err
}

func (r *formRepository) CountSubmissionsFiltered(formID string, start, end *time.Time) (int64, error) {
	q := applySubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), formID, start, end)
	var count int64
	err := q.Count(&count).Error
	return count, err
}
