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
	RenewClaim(string, string, time.Time) (bool, error)
	ReleaseClaim(string, string) error
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
	if v == nil {
		return errors.New("schedule is required")
	}
	currentVersion := v.Version
	result := r.db.Model(&models.AdminEmailSchedule{}).
		Where("id = ? AND version = ? AND claimed_at IS NULL", v.ID, currentVersion).
		Select("name", "description", "status", "recurrence", "timezone", "send_time", "start_date", "end_date", "weekdays", "month_days", "start_at", "end_at", "next_run_at", "compose_payload", "subject", "audience_label", "last_error", "consecutive_errors", "version").
		Updates(map[string]any{
			"name": v.Name, "description": v.Description, "status": v.Status, "recurrence": v.Recurrence, "timezone": v.Timezone, "send_time": v.SendTime,
			"start_date": v.StartDate, "end_date": v.EndDate, "weekdays": v.Weekdays, "month_days": v.MonthDays, "start_at": v.StartAt, "end_at": v.EndAt,
			"next_run_at": v.NextRunAt, "compose_payload": v.ComposePayload, "subject": v.Subject, "audience_label": v.AudienceLabel,
			"last_error": v.LastError, "consecutive_errors": v.ConsecutiveErrors, "version": gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("schedule changed or is currently being processed; reload and retry")
	}
	v.Version = currentVersion + 1
	return nil
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
		stale := now.Add(-5 * time.Minute)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_run_at <= ? AND (claimed_at IS NULL OR claimed_at < ?)", models.AdminEmailScheduleActive, now, stale).
			Order("next_run_at ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := tx.Model(&rows[i]).Updates(map[string]any{"claimed_at": now, "claimed_by": worker}).Error; err != nil {
				return err
			}
			rows[i].ClaimedAt = &now
			rows[i].ClaimedBy = &worker
		}
		claimed = rows
		return nil
	})
	return claimed, err
}
func (r *adminEmailScheduleRepository) RenewClaim(id, worker string, now time.Time) (bool, error) {
	result := r.db.Model(&models.AdminEmailSchedule{}).Where("id = ? AND claimed_by = ? AND status = ?", id, worker, models.AdminEmailScheduleActive).Update("claimed_at", now)
	return result.RowsAffected == 1, result.Error
}
func (r *adminEmailScheduleRepository) ReleaseClaim(id, worker string) error {
	return r.db.Model(&models.AdminEmailSchedule{}).Where("id = ? AND claimed_by = ?", id, worker).Updates(map[string]any{"claimed_at": nil, "claimed_by": nil}).Error
}
func (r *adminEmailScheduleRepository) CreateRun(v *models.AdminEmailScheduleRun) error {
	// A worker may die after the provider accepted messages but before the DB
	// transaction completes. Reclaiming the same occurrence gives at-least-once
	// delivery without leaving the schedule permanently wedged on its unique key.
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "schedule_id"}, {Name: "scheduled_for"}},
		DoUpdates: clause.Assignments(map[string]any{
			"status": "running", "started_at": v.StartedAt, "completed_at": nil, "error": nil, "attempt": gorm.Expr("admin_email_schedule_runs.attempt + 1"),
		}),
	}).Create(v).Error
}
func (r *adminEmailScheduleRepository) CompleteRun(run *models.AdminEmailScheduleRun, schedule *models.AdminEmailSchedule) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.AdminEmailScheduleRun{}).Where("schedule_id = ? AND scheduled_for = ?", run.ScheduleID, run.ScheduledFor).Updates(map[string]any{
			"status": run.Status, "delivery_id": run.DeliveryID, "sent": run.Sent, "failed": run.Failed, "error": run.Error, "completed_at": run.CompletedAt,
		}).Error; err != nil {
			return err
		}
		worker := ""
		if schedule.ClaimedBy != nil {
			worker = *schedule.ClaimedBy
		}
		result := tx.Model(&models.AdminEmailSchedule{}).Where("id = ? AND claimed_by = ?", schedule.ID, worker).Updates(map[string]any{
			"status": schedule.Status, "next_run_at": schedule.NextRunAt, "pending_occurrence_at": schedule.PendingOccurrenceAt, "last_run_at": schedule.LastRunAt,
			"run_count": schedule.RunCount, "consecutive_errors": schedule.ConsecutiveErrors, "last_error": schedule.LastError,
			"claimed_at": nil, "claimed_by": nil, "version": gorm.Expr("version + 1"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("schedule claim was lost before completion")
		}
		return nil
	})
}
func (r *adminEmailScheduleRepository) ListRuns(id string, limit int) ([]models.AdminEmailScheduleRun, error) {
	var rows []models.AdminEmailScheduleRun
	err := r.db.Where("schedule_id = ?", id).Order("scheduled_for DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
