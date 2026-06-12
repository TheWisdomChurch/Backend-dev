package repository

import (
	"context"
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type AttendanceRepository interface {
	// Service types
	ListServiceTypes(ctx context.Context, campusID *string) ([]models.ServiceType, error)
	CreateServiceType(ctx context.Context, st *models.ServiceType) error

	// Sessions
	CreateSession(ctx context.Context, s *models.AttendanceSession) error
	UpdateSession(ctx context.Context, id string, updates map[string]interface{}) error
	FindSession(ctx context.Context, id string) (*models.AttendanceSession, error)
	ListSessions(ctx context.Context, campusID *string, from, to *time.Time, limit, offset int) ([]models.AttendanceSession, int64, error)

	// Records
	CheckIn(ctx context.Context, r *models.AttendanceRecord) error
	ListRecords(ctx context.Context, sessionID string) ([]models.AttendanceRecord, error)
	MemberHistory(ctx context.Context, memberID string, limit int) ([]models.AttendanceRecord, error)
}

type attendanceRepository struct {
	db *database.Database
}

func NewAttendanceRepository(db *database.Database) AttendanceRepository {
	return &attendanceRepository{db: db}
}

func (r *attendanceRepository) ListServiceTypes(ctx context.Context, campusID *string) ([]models.ServiceType, error) {
	q := r.db.DB.WithContext(ctx).Where("is_active = true AND deleted_at IS NULL")
	if campusID != nil {
		q = q.Where("campus_id = ? OR campus_id IS NULL", *campusID)
	}
	var rows []models.ServiceType
	return rows, q.Order("name ASC").Find(&rows).Error
}

func (r *attendanceRepository) CreateServiceType(ctx context.Context, st *models.ServiceType) error {
	return r.db.DB.WithContext(ctx).Create(st).Error
}

func (r *attendanceRepository) CreateSession(ctx context.Context, s *models.AttendanceSession) error {
	return r.db.DB.WithContext(ctx).Create(s).Error
}

func (r *attendanceRepository) UpdateSession(ctx context.Context, id string, updates map[string]interface{}) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.AttendanceSession{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *attendanceRepository) FindSession(ctx context.Context, id string) (*models.AttendanceSession, error) {
	var s models.AttendanceSession
	err := r.db.DB.WithContext(ctx).
		Preload("ServiceType").
		Where("id = ? AND deleted_at IS NULL", id).
		First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *attendanceRepository) ListSessions(ctx context.Context, campusID *string, from, to *time.Time, limit, offset int) ([]models.AttendanceSession, int64, error) {
	q := r.db.DB.WithContext(ctx).Model(&models.AttendanceSession{}).
		Preload("ServiceType").
		Where("deleted_at IS NULL")
	if campusID != nil {
		q = q.Where("campus_id = ?", *campusID)
	}
	if from != nil {
		q = q.Where("date >= ?", *from)
	}
	if to != nil {
		q = q.Where("date <= ?", *to)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.AttendanceSession
	err := q.Order("date DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *attendanceRepository) CheckIn(ctx context.Context, rec *models.AttendanceRecord) error {
	return r.db.DB.WithContext(ctx).Create(rec).Error
}

func (r *attendanceRepository) ListRecords(ctx context.Context, sessionID string) ([]models.AttendanceRecord, error) {
	var rows []models.AttendanceRecord
	err := r.db.DB.WithContext(ctx).
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
		Order("checked_in_at ASC").
		Find(&rows).Error
	return rows, err
}

func (r *attendanceRepository) MemberHistory(ctx context.Context, memberID string, limit int) ([]models.AttendanceRecord, error) {
	var rows []models.AttendanceRecord
	err := r.db.DB.WithContext(ctx).
		Where("member_id = ? AND deleted_at IS NULL", memberID).
		Order("checked_in_at DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}
