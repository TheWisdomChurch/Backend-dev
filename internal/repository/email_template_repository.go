package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type EmailTemplateRepository interface {
	Create(tpl *models.EmailTemplate) error
	Update(tpl *models.EmailTemplate) error
	GetByID(id string) (*models.EmailTemplate, error)
	List(offset, limit int, ownerType, ownerID, templateKey, status string) ([]models.EmailTemplate, int64, error)
	GetActiveByOwner(ownerType, ownerID string) (*models.EmailTemplate, error)
	GetActiveByKey(templateKey string) (*models.EmailTemplate, error)
	DeactivateOthers(ownerType, ownerID, templateKey, keepID string) error
	ActivateExclusive(tpl *models.EmailTemplate) error
	NextVersion(templateKey string) (int, error)
}

type emailTemplateRepository struct {
	db *database.Database
}

func NewEmailTemplateRepository(db *database.Database) EmailTemplateRepository {
	return &emailTemplateRepository{db: db}
}

func (r *emailTemplateRepository) Create(tpl *models.EmailTemplate) error {
	return r.db.Create(tpl).Error
}

func (r *emailTemplateRepository) Update(tpl *models.EmailTemplate) error {
	return r.db.Save(tpl).Error
}

func (r *emailTemplateRepository) GetByID(id string) (*models.EmailTemplate, error) {
	var t models.EmailTemplate
	if err := r.db.First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *emailTemplateRepository) List(offset, limit int, ownerType, ownerID, templateKey, status string) ([]models.EmailTemplate, int64, error) {
	var items []models.EmailTemplate
	var total int64

	q := r.db.Model(&models.EmailTemplate{})
	if ownerType != "" {
		q = q.Where("owner_type = ?", ownerType)
	}
	if ownerID != "" {
		q = q.Where("owner_id = ?", ownerID)
	}
	if templateKey != "" {
		q = q.Where("template_key = ?", templateKey)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *emailTemplateRepository) GetActiveByOwner(ownerType, ownerID string) (*models.EmailTemplate, error) {
	if ownerType == "" || ownerID == "" {
		return nil, errors.New("ownerType and ownerID are required")
	}
	var t models.EmailTemplate
	err := r.db.
		Where("owner_type = ? AND owner_id = ? AND is_active = true", ownerType, ownerID).
		Order("version DESC").
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *emailTemplateRepository) GetActiveByKey(templateKey string) (*models.EmailTemplate, error) {
	if templateKey == "" {
		return nil, errors.New("templateKey is required")
	}
	var t models.EmailTemplate
	err := r.db.
		Where("template_key = ? AND is_active = true", templateKey).
		Order("version DESC").
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *emailTemplateRepository) DeactivateOthers(ownerType, ownerID, templateKey, keepID string) error {
	q := r.db.Model(&models.EmailTemplate{}).Where("id <> ?", keepID)
	if ownerType != "" && ownerID != "" {
		q = q.Where("owner_type = ? AND owner_id = ?", ownerType, ownerID)
	} else if templateKey != "" {
		q = q.Where("template_key = ?", templateKey)
	}
	return q.Updates(map[string]any{
		"is_active": false,
		"status":    models.EmailTemplateDraft,
	}).Error
}

// ActivateExclusive atomically activates tpl and deactivates every competing
// template in the same owner/key scope. The transaction prevents two active
// versions or a partially-applied activation when either write fails.
func (r *emailTemplateRepository) ActivateExclusive(tpl *models.EmailTemplate) error {
	if tpl == nil {
		return errors.New("template is required")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Serialize activations for the same logical scope. A transaction alone is
		// insufficient under READ COMMITTED: two concurrent transactions could
		// otherwise both deactivate the old row and then activate themselves.
		lockScope := "template-key:" + tpl.TemplateKey
		if derefRepositoryString(tpl.OwnerType) != "" && derefRepositoryString(tpl.OwnerID) != "" {
			lockScope = "template-owner:" + *tpl.OwnerType + ":" + *tpl.OwnerID
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockScope).Error; err != nil {
			return err
		}

		q := tx.Model(&models.EmailTemplate{}).Where("id <> ?", tpl.ID)
		if derefRepositoryString(tpl.OwnerType) != "" && derefRepositoryString(tpl.OwnerID) != "" {
			q = q.Where("owner_type = ? AND owner_id = ?", *tpl.OwnerType, *tpl.OwnerID)
		} else {
			q = q.Where("template_key = ?", tpl.TemplateKey)
		}
		if err := q.Updates(map[string]any{
			"is_active": false,
			"status":    models.EmailTemplateDraft,
		}).Error; err != nil {
			return err
		}

		tpl.IsActive = true
		tpl.Status = models.EmailTemplateActive
		return tx.Save(tpl).Error
	})
}

func derefRepositoryString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (r *emailTemplateRepository) NextVersion(templateKey string) (int, error) {
	if templateKey == "" {
		return 1, nil
	}
	var maxVersion int
	err := r.db.Model(&models.EmailTemplate{}).
		Select("COALESCE(MAX(version), 0)").
		Where("template_key = ?", templateKey).
		Scan(&maxVersion).Error
	if err != nil {
		return 1, err
	}
	return maxVersion + 1, nil
}
