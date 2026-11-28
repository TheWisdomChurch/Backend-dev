package service

import (
	"context"
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

type workflowRepositoryStub struct {
	current *models.NewMemberWorkflow
	updates map[string]interface{}
	history *models.NewMemberWorkflowHistory
}

func (r *workflowRepositoryStub) EnsureForSubmissions(context.Context, []models.NewMemberSubmission) (int64, error) {
	return 0, nil
}
func (r *workflowRepositoryStub) List(context.Context, int, int, string, string, string) ([]models.NewMemberWorkflow, int64, error) {
	return nil, 0, nil
}
func (r *workflowRepositoryStub) Get(context.Context, string) (*models.NewMemberWorkflow, error) {
	return r.current, nil
}
func (r *workflowRepositoryStub) Update(_ context.Context, _ string, updates map[string]interface{}, history *models.NewMemberWorkflowHistory) (*models.NewMemberWorkflow, error) {
	r.updates = updates
	r.history = history
	return r.current, nil
}
func (r *workflowRepositoryStub) AddContact(context.Context, *models.NewMemberContact, *time.Time, *models.NewMemberWorkflowHistory) error {
	return nil
}
func (r *workflowRepositoryStub) ListContacts(context.Context, string) ([]models.NewMemberContact, error) {
	return nil, nil
}
func (r *workflowRepositoryStub) ListHistory(context.Context, string) ([]models.NewMemberWorkflowHistory, error) {
	return nil, nil
}
func (r *workflowRepositoryStub) ListDue(context.Context, time.Time, int) ([]models.NewMemberWorkflow, error) {
	return nil, nil
}
func (r *workflowRepositoryStub) MarkReminder(context.Context, string, time.Time, bool) error {
	return nil
}

func TestUpdateCanClearNextAction(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	repo := &workflowRepositoryStub{current: &models.NewMemberWorkflow{ID: "workflow-id", Stage: models.NewMemberStageContacted, NextActionAt: &next}}
	svc := &newMemberWorkflowService{repo: repo}

	_, err := svc.Update(context.Background(), "workflow-id", "admin-id", models.UpdateNewMemberWorkflowRequest{ClearNextActionAt: true})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if value, exists := repo.updates["next_action_at"]; !exists || value != nil {
		t.Fatalf("next_action_at update = %#v, want explicit nil clear", repo.updates["next_action_at"])
	}
	if repo.history == nil || repo.history.EventType != "next_action_cleared" {
		t.Fatalf("history event = %#v, want next_action_cleared", repo.history)
	}
}

func TestUpdateTerminalStageClearsAnySubmittedNextAction(t *testing.T) {
	repo := &workflowRepositoryStub{current: &models.NewMemberWorkflow{ID: "workflow-id", Stage: models.NewMemberStageContacted}}
	svc := &newMemberWorkflowService{repo: repo}
	stage := models.NewMemberStageClosed
	next := time.Now().UTC().Add(24 * time.Hour)

	_, err := svc.Update(context.Background(), "workflow-id", "admin-id", models.UpdateNewMemberWorkflowRequest{Stage: &stage, NextActionAt: &next})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if value, exists := repo.updates["next_action_at"]; !exists || value != nil {
		t.Fatalf("next_action_at update = %#v, want terminal-stage clear", repo.updates["next_action_at"])
	}
	if got := repo.updates["escalation_status"]; got != "resolved" {
		t.Fatalf("escalation_status = %#v, want resolved", got)
	}
}
