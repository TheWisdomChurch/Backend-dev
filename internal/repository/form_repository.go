// internal/repository/form_repository.go
package repository

import (
	"fmt"
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
)

type FormRepository interface {
	List(offset, limit int) ([]models.Form, int64, error)
	GetByID(id string) (*models.Form, error)
	GetBySlug(slug string) (*models.Form, error)
	GetAnyBySlug(slug string) (*models.Form, error)

	Create(form *models.Form) error
	Update(form *models.Form) error
	Delete(id string) error

	SlugExists(slug string) (bool, error)
	TitleExists(title string, excludeID string) (bool, error)

	ReplaceFields(formID string, fields []models.FormField) error

	CountSubmissions(formID string) (int64, error)
	CreateSubmission(sub *models.FormSubmission) error
	ListEmailSubmissions(formID string) ([]models.FormSubmission, error)
	CreateCampaignDelivery(item *models.FormCampaignDelivery) error
	ListCampaignDeliveries(formID string, offset, limit int) ([]models.FormCampaignDelivery, int64, error)

	ListSubmissions(formID string, offset, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error)
	DeleteSubmission(id string) error
	ListRecentSubmissions(limit int, start, end *time.Time) ([]models.FormSubmissionWithForm, error)
	CountSubmissionsByForm(start, end *time.Time) ([]models.FormSubmissionCount, error)
	CountSubmissionsFiltered(formID string, start, end *time.Time) (int64, error)
	CountSubmissionsByDay(formID string, start, end *time.Time) ([]models.FormSubmissionDailyCount, error)
	ListNewMemberForms() ([]models.NewMemberFormSummary, error)
	ListNewMemberSubmissions(offset, limit int, start, end *time.Time) ([]models.NewMemberSubmission, int64, error)
	CountNewMemberSubmissions(start, end *time.Time) (int64, error)
	CountNewMemberSubmissionsByPeriod(period string, start, end *time.Time) ([]models.GrowthBucket, error)
	ListLeadershipIntakeSubmissions(limit int) ([]models.FormSubmissionWithForm, error)
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

	if err == nil {
		err = r.attachSubmissionCounts(items)
	}

	return items, total, err
}

func (r *formRepository) GetByID(id string) (*models.Form, error) {
	var f models.Form
	err := r.db.DB.Preload("Fields", "deleted_at IS NULL").First(&f, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	count, err := r.CountSubmissions(f.ID)
	if err != nil {
		return nil, err
	}
	f.SubmissionCount = count
	return &f, nil
}

func (r *formRepository) attachSubmissionCounts(items []models.Form) error {
	if len(items) == 0 {
		return nil
	}

	ids := make([]string, 0, len(items))
	for i := range items {
		ids = append(ids, items[i].ID)
	}

	var rows []struct {
		FormID string
		Count  int64
	}
	if err := r.db.DB.Model(&models.FormSubmission{}).
		Select("form_id, COUNT(*) as count").
		Where("form_id IN ?", ids).
		Group("form_id").
		Scan(&rows).Error; err != nil {
		return err
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.FormID] = row.Count
	}
	for i := range items {
		items[i].SubmissionCount = counts[items[i].ID]
	}
	return nil
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

func (r *formRepository) GetAnyBySlug(slug string) (*models.Form, error) {
	var f models.Form
	err := r.db.DB.Preload("Fields", "deleted_at IS NULL").
		First(&f, "slug = ?", slug).Error
	if err != nil {
		return nil, err
	}
	count, err := r.CountSubmissions(f.ID)
	if err != nil {
		return nil, err
	}
	f.SubmissionCount = count
	return &f, nil
}

func (r *formRepository) Create(form *models.Form) error {
	return r.db.DB.Create(form).Error
}

func (r *formRepository) Update(form *models.Form) error {
	return r.db.DB.Save(form).Error
}

func (r *formRepository) Delete(id string) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("form_id = ?", id).Delete(&models.FormCampaignDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("form_id = ?", id).Delete(&models.FormCalendarReminder{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("form_id = ?", id).Delete(&models.FormField{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("form_id = ?", id).Delete(&models.FormSubmission{}).Error; err != nil {
			return err
		}
		result := tx.Unscoped().Delete(&models.Form{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *formRepository) DeleteExpired(now time.Time) (int64, error) {
	updates := map[string]any{
		"status":       models.FormStatusInvalid,
		"is_published": false,
		"slug":         nil,
		"deleted_at":   now,
	}

	result := r.db.DB.Model(&models.Form{}).
		Where("deleted_at IS NULL").
		Where("settings ? 'expiresAt'").
		Where("NULLIF(settings->>'expiresAt','') IS NOT NULL").
		Where("(settings->>'expiresAt')::timestamptz <= ?", now).
		Updates(updates)
	return result.RowsAffected, result.Error
}

func (r *formRepository) SlugExists(slug string) (bool, error) {
	var count int64
	if err := r.db.DB.Model(&models.Form{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *formRepository) TitleExists(title string, excludeID string) (bool, error) {
	var count int64
	q := r.db.DB.Model(&models.Form{}).Where("LOWER(title) = LOWER(?)", title)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
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

func (r *formRepository) ListEmailSubmissions(formID string) ([]models.FormSubmission, error) {
	var items []models.FormSubmission
	err := r.db.DB.
		Where("form_id = ?", formID).
		Where("email IS NOT NULL").
		Where("TRIM(email) <> ''").
		Order("created_at DESC").
		Find(&items).Error
	return items, err
}

func (r *formRepository) CreateCampaignDelivery(item *models.FormCampaignDelivery) error {
	return r.db.DB.Create(item).Error
}

func (r *formRepository) ListCampaignDeliveries(formID string, offset, limit int) ([]models.FormCampaignDelivery, int64, error) {
	var items []models.FormCampaignDelivery
	var total int64

	q := r.db.DB.Model(&models.FormCampaignDelivery{}).
		Where("form_id = ?", formID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error
	return items, total, err
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

// applyNewMemberFormFilter matches forms whose submissions should count as
// "new member" intake. The primary signal is settings.submissionTarget = "member"
// — the same field internal/service/form_service_target_sync.go uses to
// actually route a submission into the members table, so a form configured
// that way is guaranteed to show up here too. The slug/title checks remain
// as a fallback for forms created before submissionTarget existed.
func applyNewMemberFormFilter(q *gorm.DB) *gorm.DB {
	return q.Where(`(
		LOWER(COALESCE(forms.settings->>'submissionTarget', '')) = 'member'
		OR LOWER(COALESCE(forms.slug, '')) = 'add-new-member'
		OR trim(regexp_replace(LOWER(COALESCE(forms.title, '')), '[^a-z0-9]+', ' ', 'g')) = 'add new member'
	)`)
}

func applyLeadershipIntakeFormFilter(q *gorm.DB) *gorm.DB {
	return q.Where(`(
		LOWER(COALESCE(forms.settings->>'submissionTarget', '')) = 'leadership'
		OR LOWER(COALESCE(forms.settings->>'formType', '')) = 'leadership'
		OR LOWER(COALESCE(forms.slug, '')) = 'leadership-biodata'
		OR LOWER(COALESCE(forms.slug, '')) = 'leadership-application'
		OR LOWER(COALESCE(forms.title, '')) LIKE '%leadership%'
	)`)
}

func applyNewMemberSubmissionFilters(q *gorm.DB, start, end *time.Time) *gorm.DB {
	q = q.Joins("JOIN forms ON forms.id = form_submissions.form_id")
	q = applyNewMemberFormFilter(q)
	if start != nil {
		q = q.Where("form_submissions.created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("form_submissions.created_at <= ?", *end)
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

func (r *formRepository) DeleteSubmission(id string) error {
	return r.db.DB.Delete(&models.FormSubmission{}, "id = ?", id).Error
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
		form_submissions.registration_code,
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

func (r *formRepository) CountSubmissionsByDay(formID string, start, end *time.Time) ([]models.FormSubmissionDailyCount, error) {
	var rows []models.FormSubmissionDailyCount
	q := applySubmissionFilters(
		r.db.DB.Model(&models.FormSubmission{}).
			Select("date_trunc('day', created_at) as day, COUNT(*) as count"),
		formID,
		start,
		end,
	).Group("day").Order("day ASC")

	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *formRepository) CountSubmissionsFiltered(formID string, start, end *time.Time) (int64, error) {
	q := applySubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), formID, start, end)
	var count int64
	err := q.Count(&count).Error
	return count, err
}

func (r *formRepository) ListNewMemberForms() ([]models.NewMemberFormSummary, error) {
	var items []models.NewMemberFormSummary

	err := applyNewMemberFormFilter(r.db.DB.Model(&models.Form{})).
		Select(`forms.id as form_id,
			forms.title as form_title,
			forms.slug,
			forms.is_published,
			COUNT(form_submissions.id) as submission_count,
			MAX(form_submissions.created_at) as last_submission_at`).
		Joins("LEFT JOIN form_submissions ON form_submissions.form_id = forms.id AND form_submissions.deleted_at IS NULL").
		Where("forms.deleted_at IS NULL").
		Group("forms.id, forms.title, forms.slug, forms.is_published").
		Order("last_submission_at DESC NULLS LAST, forms.updated_at DESC").
		Scan(&items).Error

	return items, err
}

func (r *formRepository) ListNewMemberSubmissions(offset, limit int, start, end *time.Time) ([]models.NewMemberSubmission, int64, error) {
	var items []models.NewMemberSubmission
	var total int64

	base := applyNewMemberSubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), start, end).
		Where("form_submissions.deleted_at IS NULL")

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Select(`form_submissions.id,
		form_submissions.form_id,
		forms.title as form_title,
		form_submissions.name,
		form_submissions.email,
		form_submissions.contact_number,
		form_submissions.contact_address,
		form_submissions.registration_code,
		form_submissions.values,
		form_submissions.created_at`).
		Order("form_submissions.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error

	return items, total, err
}

func (r *formRepository) CountNewMemberSubmissions(start, end *time.Time) (int64, error) {
	q := applyNewMemberSubmissionFilters(r.db.DB.Model(&models.FormSubmission{}), start, end).
		Where("form_submissions.deleted_at IS NULL")
	var count int64
	err := q.Count(&count).Error
	return count, err
}

func (r *formRepository) CountNewMemberSubmissionsByPeriod(period string, start, end *time.Time) ([]models.GrowthBucket, error) {
	var rows []models.GrowthBucket
	trunc := "month"
	switch period {
	case "week", "quarter", "year":
		trunc = period
	}
	bucketExpr := fmt.Sprintf("date_trunc('%s', form_submissions.created_at)", trunc)

	q := applyNewMemberSubmissionFilters(
		r.db.DB.Model(&models.FormSubmission{}).
			Select("to_char("+bucketExpr+", 'YYYY-MM-DD') as period, COUNT(*) as count").
			Where("form_submissions.deleted_at IS NULL"),
		start,
		end,
	)

	if err := q.Group(bucketExpr).
		Order(bucketExpr + " ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *formRepository) ListLeadershipIntakeSubmissions(limit int) ([]models.FormSubmissionWithForm, error) {
	var items []models.FormSubmissionWithForm
	if limit < 1 || limit > 1000 {
		limit = 500
	}

	q := applyLeadershipIntakeFormFilter(r.db.DB.Model(&models.FormSubmission{})).
		Select(`form_submissions.id,
			form_submissions.form_id,
			COALESCE(forms.title, '') as form_title,
			form_submissions.name,
			form_submissions.email,
			form_submissions.contact_number,
			form_submissions.contact_address,
			form_submissions.registration_code,
			form_submissions.values,
			form_submissions.created_at`).
		Joins("JOIN forms ON forms.id = form_submissions.form_id").
		Where("form_submissions.deleted_at IS NULL").
		Order("form_submissions.created_at DESC").
		Limit(limit)

	return items, q.Scan(&items).Error
}
