package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type NewMemberWorkflowService interface {
	Reconcile(ctx context.Context) (int64, error)
	List(ctx context.Context, page, limit int, stage, ownerID, escalation string) ([]models.NewMemberWorkflow, int64, error)
	Get(ctx context.Context, id string) (*models.NewMemberWorkflow, []models.NewMemberContact, []models.NewMemberWorkflowHistory, error)
	Update(ctx context.Context, id, actorID string, req models.UpdateNewMemberWorkflowRequest) (*models.NewMemberWorkflow, error)
	AddContact(ctx context.Context, id, actorID string, req models.CreateNewMemberContactRequest) (*models.NewMemberContact, error)
	ProcessDue(ctx context.Context, now time.Time) (int, error)
}

type newMemberWorkflowService struct {
	repo          NewMemberWorkflowRepositoryAlias
	formRepo      repository.FormRepository
	notifications AdminNotificationService
}

// Alias keeps constructor signatures readable while retaining the repository contract.
type NewMemberWorkflowRepositoryAlias = repository.NewMemberWorkflowRepository

func NewNewMemberWorkflowService(repo repository.NewMemberWorkflowRepository, formRepo repository.FormRepository, notifications AdminNotificationService) NewMemberWorkflowService {
	return &newMemberWorkflowService{repo: repo, formRepo: formRepo, notifications: notifications}
}

var validNewMemberStages = map[string]bool{
	"new": true, "contact_attempted": true, "contacted": true, "orientation_scheduled": true,
	"orientation_completed": true, "integrated": true, "closed": true,
}

func (s *newMemberWorkflowService) Reconcile(ctx context.Context) (int64, error) {
	if s.formRepo == nil {
		return 0, errors.New("form repository not configured")
	}
	var created int64
	for page := 0; ; page++ {
		rows, total, err := s.formRepo.ListNewMemberSubmissions(page*500, 500, nil, nil)
		if err != nil {
			return created, err
		}
		n, err := s.repo.EnsureForSubmissions(ctx, rows)
		if err != nil {
			return created, err
		}
		created += n
		if int64((page+1)*500) >= total {
			break
		}
	}
	return created, nil
}

func (s *newMemberWorkflowService) List(ctx context.Context, page, limit int, stage, ownerID, escalation string) ([]models.NewMemberWorkflow, int64, error) {
	if _, err := s.Reconcile(ctx); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 25
	}
	stage = strings.TrimSpace(stage)
	if stage != "" && !validNewMemberStages[stage] {
		return nil, 0, errors.New("invalid workflow stage")
	}
	return s.repo.List(ctx, page, limit, stage, strings.TrimSpace(ownerID), strings.TrimSpace(escalation))
}

func (s *newMemberWorkflowService) Get(ctx context.Context, id string) (*models.NewMemberWorkflow, []models.NewMemberContact, []models.NewMemberWorkflowHistory, error) {
	row, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	contacts, err := s.repo.ListContacts(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	history, err := s.repo.ListHistory(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	return row, contacts, history, nil
}

func historyDetails(value interface{}) []byte { raw, _ := json.Marshal(value); return raw }

func (s *newMemberWorkflowService) Update(ctx context.Context, id, actorID string, req models.UpdateNewMemberWorkflowRequest) (*models.NewMemberWorkflow, error) {
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	event := "workflow_updated"
	var fromStage, toStage *string
	if req.Stage != nil {
		stage := strings.TrimSpace(*req.Stage)
		if !validNewMemberStages[stage] {
			return nil, errors.New("invalid workflow stage")
		}
		updates["stage"] = stage
		from := current.Stage
		fromStage = &from
		toStage = &stage
		event = "stage_changed"
		if stage == "integrated" || stage == "closed" {
			now := time.Now().UTC()
			updates["completed_at"] = now
			updates["next_action_at"] = nil
			updates["escalation_status"] = "resolved"
		} else {
			updates["completed_at"] = nil
		}
	}
	if req.AssignedOwnerID != nil {
		owner := strings.TrimSpace(*req.AssignedOwnerID)
		if owner == "" {
			updates["assigned_owner_id"] = nil
		} else {
			updates["assigned_owner_id"] = owner
		}
		event = "owner_assigned"
	}
	if req.NextActionAt != nil {
		updates["next_action_at"] = req.NextActionAt.UTC()
		updates["escalation_status"] = "none"
		updates["escalated_at"] = nil
		event = "next_action_scheduled"
	}
	if len(updates) == 0 {
		return nil, errors.New("at least one workflow field is required")
	}
	actor := strings.TrimSpace(actorID)
	history := &models.NewMemberWorkflowHistory{EventType: event, FromStage: fromStage, ToStage: toStage, Details: historyDetails(req)}
	if actor != "" {
		history.ActorID = &actor
	}
	return s.repo.Update(ctx, id, updates, history)
}

func (s *newMemberWorkflowService) AddContact(ctx context.Context, id, actorID string, req models.CreateNewMemberContactRequest) (*models.NewMemberContact, error) {
	channels := map[string]bool{"phone": true, "email": true, "sms": true, "whatsapp": true, "in_person": true, "other": true}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if !channels[channel] {
		return nil, errors.New("invalid contact channel")
	}
	outcome := strings.TrimSpace(req.Outcome)
	if outcome == "" {
		return nil, errors.New("outcome is required")
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	contactedAt := time.Now().UTC()
	if req.ContactedAt != nil {
		contactedAt = req.ContactedAt.UTC()
	}
	contact := &models.NewMemberContact{WorkflowID: id, Channel: channel, Outcome: outcome, Notes: req.Notes, ContactedAt: contactedAt, CreatedByID: actorID}
	history := &models.NewMemberWorkflowHistory{EventType: "contact_recorded", Details: historyDetails(map[string]interface{}{"channel": channel, "outcome": outcome})}
	if actorID != "" {
		history.ActorID = &actorID
	}
	if err := s.repo.AddContact(ctx, contact, req.NextActionAt, history); err != nil {
		return nil, err
	}
	return contact, nil
}

func (s *newMemberWorkflowService) ProcessDue(ctx context.Context, now time.Time) (int, error) {
	if _, err := s.Reconcile(ctx); err != nil {
		return 0, err
	}
	rows, err := s.repo.ListDue(ctx, now.UTC(), 500)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		escalate := row.NextActionAt != nil && now.UTC().Sub(*row.NextActionAt) >= 48*time.Hour
		if err := s.repo.MarkReminder(ctx, row.ID, now.UTC(), escalate); err != nil {
			return 0, err
		}
		if s.notifications != nil {
			title := "New-member follow-up due"
			if escalate {
				title = "New-member follow-up escalated"
			}
			entityType, entityID := "new_member_workflow", row.ID
			_ = s.notifications.NotifyRoles(AdminNotificationInput{Type: "new_member_follow_up", Title: title, Message: "A persisted new-member workflow requires action in the admin portal.", EntityType: &entityType, EntityID: &entityID, Roles: []string{"admin", "super_admin"}})
		}
	}
	return len(rows), nil
}
