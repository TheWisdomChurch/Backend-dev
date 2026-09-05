package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type ApprovalService interface {
	CreateRequest(input CreateApprovalRequest) (*models.ApprovalRequest, error)
	GetRequest(id string) (*models.ApprovalRequest, error)
	CompleteRequest(t models.ApprovalRequestType, entityID string, status models.ApprovalRequestStatus, approver *models.User) (*models.ApprovalRequest, error)
	CompleteRequestByID(id string, status models.ApprovalRequestStatus, approver *models.User) (*models.ApprovalRequest, error)
	ListRequests(types []models.ApprovalRequestType, statuses []models.ApprovalRequestStatus, start, end *time.Time, limit int) ([]models.ApprovalRequest, error)
	CountRequests(ctx context.Context, statuses []models.ApprovalRequestStatus) (int64, error)
}

type CreateApprovalRequest struct {
	Type             models.ApprovalRequestType
	EntityID         *string
	EntityLabel      *string
	Reason           *string
	RequestedByID    *string
	RequestedByName  *string
	RequestedByEmail *string
}

type approvalService struct {
	repo         *repository.ApprovalRequestRepository
	sequenceRepo *repository.TicketSequenceRepository
}

func NewApprovalService(repo *repository.ApprovalRequestRepository, seq *repository.TicketSequenceRepository) ApprovalService {
	return &approvalService{repo: repo, sequenceRepo: seq}
}

func (s *approvalService) CreateRequest(input CreateApprovalRequest) (*models.ApprovalRequest, error) {
	if s == nil || s.repo == nil || s.sequenceRepo == nil {
		return nil, errors.New("approval service is not configured")
	}
	if strings.TrimSpace(string(input.Type)) == "" {
		return nil, errors.New("approval request type is required")
	}

	entityID := ""
	if input.EntityID != nil {
		entityID = strings.TrimSpace(*input.EntityID)
	}

	// Keep approval requests idempotent for the same pending entity.
	// This prevents duplicate super-admin tickets if registration is retried.
	if entityID != "" {
		if existing, err := s.repo.FindByEntity(input.Type, entityID); err == nil && existing != nil {
			if existing.Status == models.ApprovalStatusPending {
				return existing, nil
			}
		}
	}

	prefix := s.ticketPrefix(input.Type, time.Now().UTC())
	seq, err := s.sequenceRepo.Next(prefix)
	if err != nil {
		return nil, err
	}

	code := fmt.Sprintf("%s-%04d", prefix, seq)
	req := &models.ApprovalRequest{
		TicketCode:       code,
		Type:             input.Type,
		Status:           models.ApprovalStatusPending,
		EntityID:         input.EntityID,
		EntityLabel:      input.EntityLabel,
		Reason:           input.Reason,
		RequestedByID:    input.RequestedByID,
		RequestedByName:  input.RequestedByName,
		RequestedByEmail: input.RequestedByEmail,
	}

	if err := s.repo.Create(req); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *approvalService) GetRequest(id string) (*models.ApprovalRequest, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("approval repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("approval request id is required")
	}
	return s.repo.FindByID(id)
}

func (s *approvalService) CompleteRequestByID(
	id string,
	status models.ApprovalRequestStatus,
	approver *models.User,
) (*models.ApprovalRequest, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("approval repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("approval request id is required")
	}
	req, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	return s.complete(req, status, approver)
}

func (s *approvalService) CompleteRequest(
	t models.ApprovalRequestType,
	entityID string,
	status models.ApprovalRequestStatus,
	approver *models.User,
) (*models.ApprovalRequest, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("approval repository is not configured")
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return nil, errors.New("approval entity id is required")
	}
	req, err := s.repo.FindByEntity(t, entityID)
	if err != nil {
		return nil, err
	}
	return s.complete(req, status, approver)
}

func (s *approvalService) complete(
	req *models.ApprovalRequest,
	status models.ApprovalRequestStatus,
	approver *models.User,
) (*models.ApprovalRequest, error) {
	if req == nil {
		return nil, errors.New("approval request not found")
	}
	if status == "" {
		return nil, errors.New("approval status is required")
	}

	now := time.Now().UTC()
	req.Status = status
	if approver != nil {
		req.ApprovedByID = &approver.ID
		name := strings.TrimSpace(strings.Join([]string{approver.FirstName, approver.LastName}, " "))
		if name != "" {
			req.ApprovedByName = &name
		}
		if strings.TrimSpace(approver.Email) != "" {
			email := strings.TrimSpace(approver.Email)
			req.ApprovedByEmail = &email
		}
	}
	req.ApprovedAt = &now

	if err := s.repo.Update(req); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *approvalService) ListRequests(
	types []models.ApprovalRequestType,
	statuses []models.ApprovalRequestStatus,
	start, end *time.Time,
	limit int,
) ([]models.ApprovalRequest, error) {
	if s == nil || s.repo == nil {
		return []models.ApprovalRequest{}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return s.repo.List(types, statuses, start, end, limit)
}

func (s *approvalService) CountRequests(ctx context.Context, statuses []models.ApprovalRequestStatus) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("approval repository is not configured")
	}
	return s.repo.Count(ctx, statuses)
}

func (s *approvalService) ticketPrefix(t models.ApprovalRequestType, now time.Time) string {
	date := now.Format("060102")
	label := "Request"
	switch t {
	case models.ApprovalTypeTestimonial:
		label = "Testimonials"
	case models.ApprovalTypeTestimonialDelete:
		label = "TestimonialDelete"
	case models.ApprovalTypeEvent:
		label = "Events"
	case models.ApprovalTypeEventDelete:
		label = "EventDelete"
	case models.ApprovalTypeAdminUser:
		label = "Admins"
	case models.ApprovalTypeLeadershipDelete:
		label = "LeadershipDelete"
	case models.ApprovalTypeWorkforceDelete:
		label = "WorkforceDelete"
	case models.ApprovalTypeWorkforceRegistration:
		label = "WorkforceJoin"
	}
	return fmt.Sprintf("%s-%s", date, label)
}
