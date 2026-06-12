package service

import (
	"context"
	"fmt"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type CreateSessionRequest struct {
	CampusID      *string   `json:"campus_id,omitempty"`
	ServiceTypeID string    `json:"service_type_id" binding:"required"`
	Date          time.Time `json:"date" binding:"required"`
	HeadCount     int       `json:"head_count"`
	Notes         string    `json:"notes,omitempty"`
}

type CheckInRequest struct {
	SessionID    string  `json:"session_id" binding:"required"`
	MemberID     *string `json:"member_id,omitempty"`
	GuestName    string  `json:"guest_name,omitempty"`
	CheckedInVia string  `json:"checked_in_via,omitempty"` // manual, qr, app
}

type AttendanceService interface {
	ListServiceTypes(ctx context.Context, campusID *string) ([]models.ServiceType, error)
	CreateServiceType(ctx context.Context, st *models.ServiceType) error
	CreateSession(ctx context.Context, req CreateSessionRequest, createdByID *string) (*models.AttendanceSession, error)
	UpdateSession(ctx context.Context, id string, updates map[string]interface{}) error
	GetSession(ctx context.Context, id string) (*models.AttendanceSession, error)
	ListSessions(ctx context.Context, campusID *string, from, to *time.Time, limit, offset int) ([]models.AttendanceSession, int64, error)
	CheckIn(ctx context.Context, req CheckInRequest) (*models.AttendanceRecord, error)
	ListRecords(ctx context.Context, sessionID string) ([]models.AttendanceRecord, error)
	MemberHistory(ctx context.Context, memberID string, limit int) ([]models.AttendanceRecord, error)
}

type attendanceService struct {
	repo repository.AttendanceRepository
}

func NewAttendanceService(repo repository.AttendanceRepository) AttendanceService {
	return &attendanceService{repo: repo}
}

func (s *attendanceService) ListServiceTypes(ctx context.Context, campusID *string) ([]models.ServiceType, error) {
	return s.repo.ListServiceTypes(ctx, campusID)
}

func (s *attendanceService) CreateServiceType(ctx context.Context, st *models.ServiceType) error {
	return s.repo.CreateServiceType(ctx, st)
}

func (s *attendanceService) CreateSession(ctx context.Context, req CreateSessionRequest, createdByID *string) (*models.AttendanceSession, error) {
	if req.HeadCount < 0 {
		return nil, fmt.Errorf("head count cannot be negative")
	}
	session := &models.AttendanceSession{
		CampusID:      req.CampusID,
		ServiceTypeID: req.ServiceTypeID,
		Date:          req.Date,
		HeadCount:     req.HeadCount,
		Notes:         req.Notes,
		CreatedByID:   createdByID,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("attendance: create session: %w", err)
	}
	return session, nil
}

func (s *attendanceService) UpdateSession(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.repo.UpdateSession(ctx, id, updates)
}

func (s *attendanceService) GetSession(ctx context.Context, id string) (*models.AttendanceSession, error) {
	return s.repo.FindSession(ctx, id)
}

func (s *attendanceService) ListSessions(ctx context.Context, campusID *string, from, to *time.Time, limit, offset int) ([]models.AttendanceSession, int64, error) {
	return s.repo.ListSessions(ctx, campusID, from, to, limit, offset)
}

func (s *attendanceService) CheckIn(ctx context.Context, req CheckInRequest) (*models.AttendanceRecord, error) {
	if req.MemberID == nil && req.GuestName == "" {
		return nil, fmt.Errorf("either member_id or guest_name is required")
	}
	via := req.CheckedInVia
	if via == "" {
		via = "manual"
	}
	rec := &models.AttendanceRecord{
		SessionID:    req.SessionID,
		MemberID:     req.MemberID,
		GuestName:    req.GuestName,
		CheckedInVia: via,
	}
	if err := s.repo.CheckIn(ctx, rec); err != nil {
		return nil, fmt.Errorf("attendance: check-in: %w", err)
	}
	return rec, nil
}

func (s *attendanceService) ListRecords(ctx context.Context, sessionID string) ([]models.AttendanceRecord, error) {
	return s.repo.ListRecords(ctx, sessionID)
}

func (s *attendanceService) MemberHistory(ctx context.Context, memberID string, limit int) ([]models.AttendanceRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.MemberHistory(ctx, memberID, limit)
}
