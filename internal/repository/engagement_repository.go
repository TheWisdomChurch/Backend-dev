package repository

import (
	"context"
	"fmt"
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

// EngagementRepository handles persistence for pastoral care requests,
// giving intents, and contact messages.
type EngagementRepository interface {
	CreatePastoralCareRequest(ctx context.Context, req *models.PastoralCareRequest) error
	ListPastoralCareRequests(ctx context.Context, offset, limit int) ([]models.PastoralCareRequest, int64, error)

	CreateGivingIntent(ctx context.Context, intent *models.GivingIntent) error
	ListGivingIntents(ctx context.Context, offset, limit int) ([]models.GivingIntent, int64, error)

	CreateContactMessage(ctx context.Context, msg *models.ContactMessage) error
	ListContactMessages(ctx context.Context, offset, limit int) ([]models.ContactMessage, int64, error)

	CreateVisitRequest(ctx context.Context, visit *models.VisitRequest) error
	GetVisitByIdempotencyKey(ctx context.Context, key string) (*models.VisitRequest, error)
	ListVisitRequests(ctx context.Context, offset, limit int, status string) ([]models.VisitRequest, int64, error)
	GetVisitRequest(ctx context.Context, id string) (*models.VisitRequest, error)
	UpdateVisitRequest(ctx context.Context, id string, updates map[string]any) (*models.VisitRequest, error)
	ListVisitRemindersDue(ctx context.Context, from, through time.Time, limit int) ([]models.VisitRequest, error)
	ListVisitConfirmationsPending(ctx context.Context, through time.Time, limit int) ([]models.VisitRequest, error)
	ListVisitFollowUpsDue(ctx context.Context, through time.Time, limit int) ([]models.VisitRequest, error)
	CreateVisitActivity(ctx context.Context, activity *models.VisitActivity) error
	ListVisitActivities(ctx context.Context, visitID string, limit int) ([]models.VisitActivity, error)
	ClaimVisitDelivery(ctx context.Context, id, column string, at time.Time) (bool, error)
	CompleteVisitDelivery(ctx context.Context, id, column string, at time.Time) error
	ReleaseVisitDelivery(ctx context.Context, id, column string) error
}

type engagementRepository struct {
	db *database.Database
}

func NewEngagementRepository(db *database.Database) EngagementRepository {
	return &engagementRepository{db: db}
}

func (r *engagementRepository) CreatePastoralCareRequest(ctx context.Context, req *models.PastoralCareRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

func (r *engagementRepository) ListPastoralCareRequests(ctx context.Context, offset, limit int) ([]models.PastoralCareRequest, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.PastoralCareRequest{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.PastoralCareRequest
	if err := r.db.WithContext(ctx).Model(&models.PastoralCareRequest{}).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *engagementRepository) CreateGivingIntent(ctx context.Context, intent *models.GivingIntent) error {
	return r.db.WithContext(ctx).Create(intent).Error
}

func (r *engagementRepository) ListGivingIntents(ctx context.Context, offset, limit int) ([]models.GivingIntent, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.GivingIntent{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.GivingIntent
	if err := r.db.WithContext(ctx).Model(&models.GivingIntent{}).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *engagementRepository) CreateContactMessage(ctx context.Context, msg *models.ContactMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *engagementRepository) ListContactMessages(ctx context.Context, offset, limit int) ([]models.ContactMessage, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.ContactMessage{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.ContactMessage
	if err := r.db.WithContext(ctx).Model(&models.ContactMessage{}).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *engagementRepository) CreateVisitRequest(ctx context.Context, visit *models.VisitRequest) error {
	return r.db.WithContext(ctx).Create(visit).Error
}

func (r *engagementRepository) GetVisitByIdempotencyKey(ctx context.Context, key string) (*models.VisitRequest, error) {
	var visit models.VisitRequest
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&visit).Error; err != nil {
		return nil, err
	}
	return &visit, nil
}

func (r *engagementRepository) ListVisitRequests(ctx context.Context, offset, limit int, status string) ([]models.VisitRequest, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.VisitRequest{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.VisitRequest
	if err := query.Order("service_at ASC, created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *engagementRepository) GetVisitRequest(ctx context.Context, id string) (*models.VisitRequest, error) {
	var visit models.VisitRequest
	if err := r.db.WithContext(ctx).First(&visit, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &visit, nil
}

func (r *engagementRepository) UpdateVisitRequest(ctx context.Context, id string, updates map[string]any) (*models.VisitRequest, error) {
	if err := r.db.WithContext(ctx).Model(&models.VisitRequest{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetVisitRequest(ctx, id)
}

func (r *engagementRepository) ListVisitRemindersDue(ctx context.Context, from, through time.Time, limit int) ([]models.VisitRequest, error) {
	var items []models.VisitRequest
	err := r.db.WithContext(ctx).
		Where("service_at > ? AND service_at <= ? AND reminder_sent_at IS NULL AND reminder_opt_in = TRUE AND status NOT IN ?", from, through, []string{"cancelled", "completed", "no_show"}).
		Order("service_at ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (r *engagementRepository) ListVisitConfirmationsPending(ctx context.Context, through time.Time, limit int) ([]models.VisitRequest, error) {
	var items []models.VisitRequest
	err := r.db.WithContext(ctx).
		Where("created_at <= ? AND confirmation_sent_at IS NULL AND service_at > ? AND status != ?", through, time.Now().UTC(), "cancelled").
		Order("created_at ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (r *engagementRepository) ListVisitFollowUpsDue(ctx context.Context, through time.Time, limit int) ([]models.VisitRequest, error) {
	var items []models.VisitRequest
	err := r.db.WithContext(ctx).
		Where("next_follow_up_at IS NOT NULL AND next_follow_up_at <= ? AND follow_up_notified_at IS NULL AND status NOT IN ?", through, []string{"completed", "cancelled"}).
		Order("next_follow_up_at ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (r *engagementRepository) CreateVisitActivity(ctx context.Context, activity *models.VisitActivity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

func (r *engagementRepository) ListVisitActivities(ctx context.Context, visitID string, limit int) ([]models.VisitActivity, error) {
	var items []models.VisitActivity
	err := r.db.WithContext(ctx).Where("visit_id = ?", visitID).Order("created_at DESC").Limit(limit).Find(&items).Error
	return items, err
}

func visitDeliveryClaimColumn(column string) (string, bool) {
	columns := map[string]string{
		"confirmation_sent_at":  "confirmation_claimed_at",
		"reminder_sent_at":      "reminder_claimed_at",
		"follow_up_notified_at": "follow_up_claimed_at",
	}
	claim, ok := columns[column]
	return claim, ok
}

func (r *engagementRepository) ClaimVisitDelivery(ctx context.Context, id, column string, at time.Time) (bool, error) {
	claimColumn, ok := visitDeliveryClaimColumn(column)
	if !ok {
		return false, fmt.Errorf("invalid visit delivery column")
	}
	result := r.db.WithContext(ctx).Model(&models.VisitRequest{}).
		Where("id = ? AND "+column+" IS NULL AND ("+claimColumn+" IS NULL OR "+claimColumn+" < ?)", id, at.Add(-15*time.Minute)).
		Update(claimColumn, at)
	return result.RowsAffected == 1, result.Error
}

func (r *engagementRepository) CompleteVisitDelivery(ctx context.Context, id, column string, at time.Time) error {
	claimColumn, ok := visitDeliveryClaimColumn(column)
	if !ok {
		return fmt.Errorf("invalid visit delivery column")
	}
	return r.db.WithContext(ctx).Model(&models.VisitRequest{}).Where("id = ?", id).Updates(map[string]any{column: at, claimColumn: nil}).Error
}

func (r *engagementRepository) ReleaseVisitDelivery(ctx context.Context, id, column string) error {
	claimColumn, ok := visitDeliveryClaimColumn(column)
	if !ok {
		return fmt.Errorf("invalid visit delivery column")
	}
	return r.db.WithContext(ctx).Model(&models.VisitRequest{}).Where("id = ?", id).Update(claimColumn, nil).Error
}
