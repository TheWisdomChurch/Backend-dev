package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type AdminEmailScheduleRepository interface {
	Create(*models.AdminEmailSchedule) error
	Update(*models.AdminEmailSchedule) error
	Get(string) (*models.AdminEmailSchedule, error)
	List(offset, limit int, status string) ([]models.AdminEmailSchedule, int64, error)
	Delete(string) error
	ClaimDue(context.Context, time.Time, string, int) ([]models.AdminEmailSchedule, error)
	ReleaseClaim(string) error
	CreateRun(*models.AdminEmailScheduleRun) error
	CompleteRun(*models.AdminEmailScheduleRun, *models.AdminEmailSchedule) error
	ListRuns(string, int) ([]models.AdminEmailScheduleRun, error)
}

type adminEmailScheduleRepository struct{ db *database.Database }

func NewAdminEmailScheduleRepository(db *database.Database) AdminEmailScheduleRepository {
	return &adminEmailScheduleRepository{db: db}
}

func (r *adminEmailScheduleRepository) Create(v *models.AdminEmailSchedule) error {
	return r.db.Create(v).Error
}
func (r *adminEmailScheduleRepository) Update(v *models.AdminEmailSchedule) error {
	return r.db.Save(v).Error
}
func (r *adminEmailScheduleRepository) Get(id string) (*models.AdminEmailSchedule, error) {
	var v models.AdminEmailSchedule
	if err := r.db.First(&v, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}
func (r *adminEmailScheduleRepository) List(offset, limit int, status string) ([]models.AdminEmailSchedule, int64, error) {
	var rows []models.AdminEmailSchedule
	var total int64
	q := r.db.Model(&models.AdminEmailSchedule{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("next_run_at ASC NULLS LAST, created_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
func (r *adminEmailScheduleRepository) Delete(id string) error {
	result := r.db.Delete(&models.AdminEmailSchedule{}, "id = ? AND status IN ?", id, []string{"draft", "paused", "completed", "failed"})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("schedule is active or does not exist")
	}
	return nil
}

// ClaimDue uses row locks and a lease so concurrent API replicas cannot send
// the same occurrence. Stale leases are recoverable after ten minutes.
func (r *adminEmailScheduleRepository) ClaimDue(ctx context.Context, now time.Time, worker string, limit int) ([]models.AdminEmailSchedule, error) {
	var claimed []models.AdminEmailSchedule
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []models.AdminEmailSchedule
		stale := now.Add(-10 * time.Minute)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_run_at <= ? AND (claimed_at IS NULL OR claimed_at < ?)", models.AdminEmailScheduleActive, now, stale).
			Order("next_run_at ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := tx.Model(&rows[i]).Updates(map[string]any{"claimed_at": now, "claimed_by": worker}).Error; err != nil {
				return err
			}
		}
		claimed = rows
		return nil
	})
	return claimed, err
}
func (r *adminEmailScheduleRepository) ReleaseClaim(id string) error {
	return r.db.Model(&models.AdminEmailSchedule{}).Where("id = ?", id).Updates(map[string]any{"claimed_at": nil, "claimed_by": nil}).Error
}
func (r *adminEmailScheduleRepository) CreateRun(v *models.AdminEmailScheduleRun) error {
	// A worker may die after the provider accepted messages but before the DB
	// transaction completes. Reclaiming the same occurrence gives at-least-once
	// delivery without leaving the schedule permanently wedged on its unique key.
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "schedule_id"}, {Name: "scheduled_for"}},
		DoUpdates: clause.Assignments(map[string]any{
			"status": "running", "started_at": v.StartedAt, "completed_at": nil, "error": nil,
		}),
	}).Create(v).Error
}
func (r *adminEmailScheduleRepository) CompleteRun(run *models.AdminEmailScheduleRun, schedule *models.AdminEmailSchedule) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(run).Error; err != nil {
			return err
		}
		return tx.Save(schedule).Error
	})
}
func (r *adminEmailScheduleRepository) ListRuns(id string, limit int) ([]models.AdminEmailScheduleRun, error) {
	var rows []models.AdminEmailScheduleRun
	err := r.db.Where("schedule_id = ?", id).Order("scheduled_for DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
