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

type CelebrationCandidate struct {
	Source    string
	SourceID  string
	FirstName string
	LastName  string
	Email     string
	Kind      string
}

type CelebrationAutomationRepository interface {
	GetConfig() (*models.CelebrationAutomationConfig, error)
	UpdateConfig(*models.CelebrationAutomationConfig) error
	EnsureRun(context.Context, string, string, string, []byte) (*models.CelebrationAutomationRun, error)
	ClaimRun(context.Context, string, string, time.Time) (*models.CelebrationAutomationRun, error)
	CompleteRun(context.Context, *models.CelebrationAutomationRun, string) error
	ListCandidates(context.Context, int, int, bool, bool) ([]CelebrationCandidate, error)
	UpsertDelivery(context.Context, *models.CelebrationDelivery) (*models.CelebrationDelivery, error)
	UpdateDelivery(context.Context, *models.CelebrationDelivery) error
	ListRuns(int, int) ([]models.CelebrationAutomationRun, int64, error)
	ListDeliveries(string, int, int) ([]models.CelebrationDelivery, int64, error)
	GetRunByDate(string) (*models.CelebrationAutomationRun, error)
}

type celebrationAutomationRepository struct{ db *database.Database }

func NewCelebrationAutomationRepository(db *database.Database) CelebrationAutomationRepository {
	return &celebrationAutomationRepository{db: db}
}
func (r *celebrationAutomationRepository) GetConfig() (*models.CelebrationAutomationConfig, error) {
	var v models.CelebrationAutomationConfig
	if err := r.db.First(&v, "id = ?", "default").Error; err != nil {
		return nil, err
	}
	return &v, nil
}
func (r *celebrationAutomationRepository) UpdateConfig(v *models.CelebrationAutomationConfig) error {
	return r.db.Model(&models.CelebrationAutomationConfig{}).Where("id = ?", "default").Select("enabled", "birthday_enabled", "anniversary_enabled", "timezone", "send_time", "feb29_policy", "max_attempts", "retry_minutes", "birthday_subject", "anniversary_subject", "birthday_template_key", "anniversary_template_key", "updated_by_user_id", "updated_by_email", "updated_at").Updates(v).Error
}
func (r *celebrationAutomationRepository) EnsureRun(ctx context.Context, date, timezone, trigger string, snapshot []byte) (*models.CelebrationAutomationRun, error) {
	v := &models.CelebrationAutomationRun{RunDate: date, Timezone: timezone, Status: "pending", Attempt: 0, Trigger: trigger, ConfigSnapshot: snapshot}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "run_date"}}, DoNothing: true}).Create(v).Error; err != nil {
		return nil, err
	}
	return r.GetRunByDate(date)
}
func (r *celebrationAutomationRepository) GetRunByDate(date string) (*models.CelebrationAutomationRun, error) {
	var v models.CelebrationAutomationRun
	if err := r.db.First(&v, "run_date = ?", date).Error; err != nil {
		return nil, err
	}
	return &v, nil
}
func (r *celebrationAutomationRepository) ClaimRun(ctx context.Context, id, worker string, now time.Time) (*models.CelebrationAutomationRun, error) {
	var claimed models.CelebrationAutomationRun
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var v models.CelebrationAutomationRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).First(&v, "id = ?", id).Error; err != nil {
			return err
		}
		stale := now.Add(-5 * time.Minute)
		eligible := v.Status == "pending" || (v.Status == "partial" && (v.NextAttemptAt == nil || !v.NextAttemptAt.After(now))) || (v.Status == "running" && v.ClaimedAt != nil && v.ClaimedAt.Before(stale))
		if !eligible {
			return gorm.ErrRecordNotFound
		}
		v.Status = "running"
		v.ClaimedAt = &now
		v.ClaimedBy = &worker
		v.Attempt++
		v.StartedAt = &now
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		claimed = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &claimed, nil
}
func (r *celebrationAutomationRepository) CompleteRun(ctx context.Context, v *models.CelebrationAutomationRun, worker string) error {
	if v == nil {
		return errors.New("run is required")
	}
	result := r.db.WithContext(ctx).Model(&models.CelebrationAutomationRun{}).Where("id = ? AND claimed_by = ?", v.ID, worker).Updates(map[string]any{"status": v.Status, "targeted": v.Targeted, "sent": v.Sent, "suppressed": v.Suppressed, "skipped": v.Skipped, "failed": v.Failed, "last_error": v.LastError, "next_attempt_at": v.NextAttemptAt, "completed_at": v.CompletedAt, "claimed_at": nil, "claimed_by": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("celebration run claim was lost")
	}
	return nil
}
func (r *celebrationAutomationRepository) ListCandidates(ctx context.Context, month, day int, birthdays, anniversaries bool) ([]CelebrationCandidate, error) {
	rows := []CelebrationCandidate{}
	if birthdays {
		queries := []struct {
			sql  string
			args []any
		}{
			{`SELECT 'member' source,id::text source_id,first_name,last_name,email,'birthday' kind FROM members WHERE is_active=true AND birthday_month=? AND birthday_day=?`, []any{month, day}},
			{`SELECT 'workforce' source,id::text source_id,first_name,last_name,email,'birthday' kind FROM workforce_members WHERE status='serving' AND birthday_month=? AND birthday_day=?`, []any{month, day}},
			{`SELECT 'leadership' source,id::text source_id,first_name,last_name,email,'birthday' kind FROM leadership_members WHERE status='approved' AND birthday_month=? AND birthday_day=?`, []any{month, day}},
		}
		for _, q := range queries {
			var part []CelebrationCandidate
			if err := r.db.WithContext(ctx).Raw(q.sql, q.args...).Scan(&part).Error; err != nil {
				return nil, err
			}
			rows = append(rows, part...)
		}
	}
	if anniversaries {
		queries := []string{
			`SELECT 'workforce' source,id::text source_id,first_name,last_name,email,'anniversary' kind FROM workforce_members WHERE status='serving' AND anniversary_month=? AND anniversary_day=?`,
			`SELECT 'leadership' source,id::text source_id,first_name,last_name,email,'anniversary' kind FROM leadership_members WHERE status='approved' AND anniversary_month=? AND anniversary_day=?`,
		}
		for _, q := range queries {
			var part []CelebrationCandidate
			if err := r.db.WithContext(ctx).Raw(q, month, day).Scan(&part).Error; err != nil {
				return nil, err
			}
			rows = append(rows, part...)
		}
	}
	return rows, nil
}
func (r *celebrationAutomationRepository) UpsertDelivery(ctx context.Context, v *models.CelebrationDelivery) (*models.CelebrationDelivery, error) {
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "run_id"}, {Name: "kind"}, {Name: "email_hash"}}, DoNothing: true}).Create(v).Error; err != nil {
		return nil, err
	}
	var existing models.CelebrationDelivery
	if err := r.db.WithContext(ctx).First(&existing, "run_id = ? AND kind = ? AND email_hash = ?", v.RunID, v.Kind, v.EmailHash).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}
func (r *celebrationAutomationRepository) UpdateDelivery(ctx context.Context, v *models.CelebrationDelivery) error {
	return r.db.WithContext(ctx).Model(v).Select("status", "attempt", "error", "sent_at", "sources", "recipient_name").Updates(v).Error
}
func (r *celebrationAutomationRepository) ListRuns(offset, limit int) ([]models.CelebrationAutomationRun, int64, error) {
	var rows []models.CelebrationAutomationRun
	var total int64
	q := r.db.Model(&models.CelebrationAutomationRun{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("run_date DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}
func (r *celebrationAutomationRepository) ListDeliveries(runID string, offset, limit int) ([]models.CelebrationDelivery, int64, error) {
	var rows []models.CelebrationDelivery
	var total int64
	q := r.db.Model(&models.CelebrationDelivery{}).Where("run_id = ?", runID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("kind,recipient_email").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}
