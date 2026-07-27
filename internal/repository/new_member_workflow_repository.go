package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type NewMemberWorkflowRepository interface {
	EnsureForSubmissions(ctx context.Context, submissions []models.NewMemberSubmission) (int64, error)
	List(ctx context.Context, page, limit int, stage, ownerID, escalation string) ([]models.NewMemberWorkflow, int64, error)
	Get(ctx context.Context, id string) (*models.NewMemberWorkflow, error)
	Update(ctx context.Context, id string, updates map[string]interface{}, history *models.NewMemberWorkflowHistory) (*models.NewMemberWorkflow, error)
	AddContact(ctx context.Context, contact *models.NewMemberContact, nextActionAt *time.Time, history *models.NewMemberWorkflowHistory) error
	ListContacts(ctx context.Context, workflowID string) ([]models.NewMemberContact, error)
	ListHistory(ctx context.Context, workflowID string) ([]models.NewMemberWorkflowHistory, error)
	ListDue(ctx context.Context, now time.Time, limit int) ([]models.NewMemberWorkflow, error)
	MarkReminder(ctx context.Context, workflowID string, now time.Time, escalate bool) error
}

type newMemberWorkflowRepository struct{ db *database.Database }

func NewNewMemberWorkflowRepository(db *database.Database) NewMemberWorkflowRepository {
	return &newMemberWorkflowRepository{db: db}
}

func (r *newMemberWorkflowRepository) EnsureForSubmissions(ctx context.Context, submissions []models.NewMemberSubmission) (int64, error) {
	if len(submissions) == 0 {
		return 0, nil
	}
	rows := make([]models.NewMemberWorkflow, 0, len(submissions))
	for _, sub := range submissions {
		next := sub.CreatedAt.UTC().Add(24 * time.Hour)
		rows = append(rows, models.NewMemberWorkflow{SubmissionID: sub.ID, Stage: models.NewMemberStageNew, EscalationStatus: "none", NextActionAt: &next})
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "submission_id"}}, DoNothing: true}).Create(&rows)
	return result.RowsAffected, result.Error
}

func (r *newMemberWorkflowRepository) List(ctx context.Context, page, limit int, stage, ownerID, escalation string) ([]models.NewMemberWorkflow, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.NewMemberWorkflow{})
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	if ownerID != "" {
		q = q.Where("assigned_owner_id = ?", ownerID)
	}
	if escalation != "" {
		q = q.Where("escalation_status = ?", escalation)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.NewMemberWorkflow
	err := q.Select("new_member_workflows.*, NULLIF(trim(concat_ws(' ', owners.first_name, owners.last_name)), '') AS assigned_owner_name").
		Joins("LEFT JOIN users owners ON owners.id = new_member_workflows.assigned_owner_id").
		Order("CASE WHEN new_member_workflows.next_action_at IS NULL THEN 1 ELSE 0 END, new_member_workflows.next_action_at ASC, new_member_workflows.created_at DESC").
		Offset((page - 1) * limit).Limit(limit).Scan(&rows).Error
	return rows, total, err
}

func (r *newMemberWorkflowRepository) Get(ctx context.Context, id string) (*models.NewMemberWorkflow, error) {
	var row models.NewMemberWorkflow
	if err := r.db.WithContext(ctx).Model(&models.NewMemberWorkflow{}).
		Select("new_member_workflows.*, NULLIF(trim(concat_ws(' ', owners.first_name, owners.last_name)), '') AS assigned_owner_name").
		Joins("LEFT JOIN users owners ON owners.id = new_member_workflows.assigned_owner_id").
		Where("new_member_workflows.id = ?", id).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (r *newMemberWorkflowRepository) Update(ctx context.Context, id string, updates map[string]interface{}, history *models.NewMemberWorkflowHistory) (*models.NewMemberWorkflow, error) {
	var out models.NewMemberWorkflow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.NewMemberWorkflow{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if history != nil {
			history.WorkflowID = id
			if err := tx.Create(history).Error; err != nil {
				return err
			}
		}
		return tx.First(&out, "id = ?", id).Error
	})
	return &out, err
}

func (r *newMemberWorkflowRepository) AddContact(ctx context.Context, contact *models.NewMemberContact, nextActionAt *time.Time, history *models.NewMemberWorkflowHistory) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(contact).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"last_contacted_at": contact.ContactedAt, "escalation_status": "none", "escalated_at": nil}
		if nextActionAt != nil {
			updates["next_action_at"] = nextActionAt
		}
		if err := tx.Model(&models.NewMemberWorkflow{}).Where("id = ?", contact.WorkflowID).Updates(updates).Error; err != nil {
			return err
		}
		if history != nil {
			history.WorkflowID = contact.WorkflowID
			return tx.Create(history).Error
		}
		return nil
	})
}

func (r *newMemberWorkflowRepository) ListContacts(ctx context.Context, workflowID string) ([]models.NewMemberContact, error) {
	var rows []models.NewMemberContact
	err := r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("contacted_at DESC").Find(&rows).Error
	return rows, err
}
func (r *newMemberWorkflowRepository) ListHistory(ctx context.Context, workflowID string) ([]models.NewMemberWorkflowHistory, error) {
	var rows []models.NewMemberWorkflowHistory
	err := r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("created_at DESC").Find(&rows).Error
	return rows, err
}
func (r *newMemberWorkflowRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]models.NewMemberWorkflow, error) {
	var rows []models.NewMemberWorkflow
	err := r.db.WithContext(ctx).
		Where("completed_at IS NULL AND next_action_at IS NOT NULL AND next_action_at <= ?", now).
		Where("last_reminder_at IS NULL OR (escalation_status = 'due' AND next_action_at <= ?)", now.Add(-48*time.Hour)).
		Where("escalation_status <> 'escalated'").
		Order("next_action_at ASC").Limit(limit).Find(&rows).Error
	return rows, err
}
func (r *newMemberWorkflowRepository) MarkReminder(ctx context.Context, workflowID string, now time.Time, escalate bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{"last_reminder_at": now, "escalation_status": "due"}
		event := "follow_up_due"
		if escalate {
			updates["escalation_status"] = "escalated"
			updates["escalated_at"] = now
			event = "follow_up_escalated"
		}
		if err := tx.Model(&models.NewMemberWorkflow{}).Where("id = ?", workflowID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&models.NewMemberWorkflowHistory{WorkflowID: workflowID, EventType: event, Details: []byte(`{"source":"scheduler"}`)}).Error
	})
}
