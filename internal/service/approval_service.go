package service

import (
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type ApprovalService interface {
	CreateRequest(input CreateApprovalRequest) (*models.ApprovalRequest, error)
	CompleteRequest(t models.ApprovalRequestType, entityID string, status models.ApprovalRequestStatus, approver *models.User) (*models.ApprovalRequest, error)
	ListRequests(types []models.ApprovalRequestType, statuses []models.ApprovalRequestStatus, start, end *time.Time, limit int) ([]models.ApprovalRequest, error)
}

type CreateApprovalRequest struct {
	Type             models.ApprovalRequestType
	EntityID         *string
	EntityLabel      *string
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
		RequestedByID:    input.RequestedByID,
		RequestedByName:  input.RequestedByName,
		RequestedByEmail: input.RequestedByEmail,
	}

	if err := s.repo.Create(req); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *approvalService) CompleteRequest(
	t models.ApprovalRequestType,
	entityID string,
	status models.ApprovalRequestStatus,
	approver *models.User,
) (*models.ApprovalRequest, error) {
	req, err := s.repo.FindByEntity(t, entityID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	req.Status = status
	if approver != nil {
		req.ApprovedByID = &approver.ID
		name := strings.TrimSpace(strings.Join([]string{approver.FirstName, approver.LastName}, " "))
		if name != "" {
			req.ApprovedByName = &name
		}
		if approver.Email != "" {
			email := approver.Email
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
	return s.repo.List(types, statuses, start, end, limit)
}

func (s *approvalService) ticketPrefix(t models.ApprovalRequestType, now time.Time) string {
	date := now.Format("060102")
	label := "Request"
	switch t {
	case models.ApprovalTypeTestimonial:
		label = "Testimonials"
	case models.ApprovalTypeEvent:
		label = "Events"
	case models.ApprovalTypeAdminUser:
		label = "Admins"
	case models.ApprovalTypeLeadershipDelete:
		label = "LeadershipDelete"
	case models.ApprovalTypeWorkforceDelete:
		label = "WorkforceDelete"
	}
	return fmt.Sprintf("%s-%s", date, label)
}
