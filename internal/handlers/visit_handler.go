package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/sanitize"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

const (
	visitTimezone = "Africa/Lagos"
	visitHour     = 9
)

var visitStatuses = map[string]bool{
	"new": true, "confirmed": true, "contacted": true, "arrived": true,
	"no_show": true, "completed": true, "cancelled": true,
}

var visitStatusTransitions = map[string]map[string]bool{
	"new":       {"confirmed": true, "contacted": true, "cancelled": true},
	"confirmed": {"contacted": true, "arrived": true, "no_show": true, "cancelled": true},
	"contacted": {"arrived": true, "no_show": true, "completed": true, "cancelled": true},
	"arrived":   {"completed": true},
	"no_show":   {"contacted": true, "completed": true},
	"completed": {},
	"cancelled": {},
}

type CreateVisitRequest struct {
	FirstName      string `json:"firstName" binding:"required"`
	LastName       string `json:"lastName" binding:"required"`
	Email          string `json:"email" binding:"required,email"`
	Phone          string `json:"phone"`
	ServiceDate    string `json:"serviceDate" binding:"required"`
	Attendance     int    `json:"attendance" binding:"required,min=1,max=20"`
	Notes          string `json:"notes"`
	ReminderOptIn  bool   `json:"reminderOptIn"`
	SourceChannel  string `json:"sourceChannel"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type UpdateVisitRequest struct {
	Status         *string `json:"status"`
	AssignedTo     *string `json:"assignedTo"`
	NextFollowUpAt *string `json:"nextFollowUpAt"`
	Contacted      bool    `json:"contacted"`
	ActivityNotes  string  `json:"activityNotes"`
}

type VisitPublicResponse struct {
	ID                 string    `json:"id"`
	ServiceDate        time.Time `json:"serviceDate"`
	ServiceAt          time.Time `json:"serviceAt"`
	ServiceType        string    `json:"serviceType"`
	Attendance         int       `json:"attendance"`
	Status             string    `json:"status"`
	ReminderOptIn      bool      `json:"reminderOptIn"`
	ConfirmationQueued bool      `json:"confirmationQueued"`
}

type VisitServiceOption struct {
	Date        string    `json:"date"`
	ServiceAt   time.Time `json:"serviceAt"`
	ServiceType string    `json:"serviceType"`
}

func publicVisit(visit *models.VisitRequest) VisitPublicResponse {
	return VisitPublicResponse{
		ID: visit.ID, ServiceDate: visit.ServiceDate, ServiceAt: visit.ServiceAt,
		ServiceType: visit.ServiceType, Attendance: visit.Attendance, Status: visit.Status,
		ReminderOptIn: visit.ReminderOptIn, ConfirmationQueued: visit.ConfirmationSentAt != nil,
	}
}

func lagosLocation() *time.Location {
	loc, err := time.LoadLocation(visitTimezone)
	if err != nil {
		return time.FixedZone("WAT", 60*60)
	}
	return loc
}

// classifySunday is authoritative. First Sunday is Celebration & Communion,
// last Sunday is Supernatural, and every Sunday between them is Gaining Wisdom.
// This naturally yields three Gaining Wisdom services in a five-Sunday month.
func classifySunday(date time.Time) string {
	if date.Day() <= 7 {
		return "Celebration & Communion Service"
	}
	nextWeek := date.AddDate(0, 0, 7)
	if nextWeek.Month() != date.Month() {
		return "Supernatural Service"
	}
	return "Gaining Wisdom Service"
}

func parseVisitServiceDate(raw string) (time.Time, time.Time, string, error) {
	loc := lagosLocation()
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("serviceDate must be YYYY-MM-DD")
	}
	if date.Weekday() != time.Sunday {
		return time.Time{}, time.Time{}, "", fmt.Errorf("selected service date must be a Sunday")
	}
	serviceAt := time.Date(date.Year(), date.Month(), date.Day(), visitHour, 0, 0, 0, loc)
	now := time.Now().In(loc)
	if serviceAt.Before(now.Add(-15 * time.Minute)) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("selected service has already started")
	}
	if serviceAt.After(now.AddDate(0, 6, 0)) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("selected service is too far in the future")
	}
	return date, serviceAt.UTC(), classifySunday(date), nil
}

func visitIdempotencyKey(req CreateVisitRequest) string {
	// The server owns this key. Trusting a caller-provided key would let a
	// malicious request collide with and retrieve another visitor's record.
	raw := strings.ToLower(strings.TrimSpace(req.Email)) + "|" + strings.TrimSpace(req.ServiceDate)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (h *EngagementHandler) ListVisitServices(c *gin.Context) {
	count := parseIntClamp(c.DefaultQuery("count", "8"), 1, 16)
	loc := lagosLocation()
	cursor := time.Now().In(loc)
	cursor = time.Date(cursor.Year(), cursor.Month(), cursor.Day(), visitHour, 0, 0, 0, loc)
	daysUntilSunday := (int(time.Sunday) - int(cursor.Weekday()) + 7) % 7
	cursor = cursor.AddDate(0, 0, daysUntilSunday)
	if !cursor.After(time.Now().In(loc)) {
		cursor = cursor.AddDate(0, 0, 7)
	}
	items := make([]VisitServiceOption, 0, count)
	for index := 0; index < count; index++ {
		date := cursor.AddDate(0, 0, index*7)
		items = append(items, VisitServiceOption{Date: date.Format("2006-01-02"), ServiceAt: date.UTC(), ServiceType: classifySunday(date)})
	}
	utils.SuccessResponse(c, http.StatusOK, "Upcoming Sunday services loaded", items)
}

func (h *EngagementHandler) CreateVisit(c *gin.Context) {
	var req CreateVisitRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if len(strings.TrimSpace(req.FirstName)) > 120 || len(strings.TrimSpace(req.LastName)) > 120 || len(strings.TrimSpace(req.Email)) > 255 || len(strings.TrimSpace(req.Phone)) > 60 || len(strings.TrimSpace(req.Notes)) > 2000 {
		utils.ErrorResponse(c, http.StatusBadRequest, "One or more visit fields exceed the allowed length")
		return
	}
	serviceDate, serviceAt, serviceType, err := parseVisitServiceDate(req.ServiceDate)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	key := visitIdempotencyKey(req)
	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	if existing, findErr := h.repo.GetVisitByIdempotencyKey(ctx, key); findErr == nil {
		utils.SuccessResponse(c, http.StatusOK, "Visit already planned", publicVisit(existing))
		return
	} else if findErr != nil && findErr != gorm.ErrRecordNotFound {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Unable to verify visit request")
		return
	}

	source := strings.TrimSpace(req.SourceChannel)
	if source == "" {
		source = "frontend:web:plan-visit"
	}
	visit := models.VisitRequest{
		FirstName: sanitize.Text(strings.TrimSpace(req.FirstName)), LastName: sanitize.Text(strings.TrimSpace(req.LastName)),
		Email: strings.ToLower(strings.TrimSpace(req.Email)), Phone: strings.TrimSpace(req.Phone),
		ServiceDate: serviceDate, ServiceAt: serviceAt, ServiceType: serviceType,
		Attendance: req.Attendance, Notes: sanitize.Text(strings.TrimSpace(req.Notes)), Status: "new",
		ReminderOptIn: req.ReminderOptIn, SourceChannel: source, IdempotencyKey: key,
	}
	followUpAt := serviceAt.Add(4 * time.Hour)
	visit.NextFollowUpAt = &followUpAt
	if err := h.repo.CreateVisitRequest(ctx, &visit); err != nil {
		// Close the lookup/insert race safely. The unique key is server-derived,
		// and the public DTO never exposes another visitor's PII.
		if existing, findErr := h.repo.GetVisitByIdempotencyKey(ctx, key); findErr == nil {
			utils.SuccessResponse(c, http.StatusOK, "Visit already planned", publicVisit(existing))
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Unable to save visit request")
		return
	}
	_ = h.repo.CreateVisitActivity(ctx, &models.VisitActivity{VisitID: visit.ID, EventType: "created", ToStatus: visit.Status, Notes: "Public plan-a-visit submission"})

	if h.notifySvc != nil {
		entityType, entityID := "visit_request", visit.ID
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type: "visit_request", Title: "New planned visit",
			Message:    fmt.Sprintf("%s %s plans to attend %s on %s.", visit.FirstName, visit.LastName, visit.ServiceType, visit.ServiceDate.Format("2 Jan 2006")),
			EntityType: &entityType, EntityID: &entityID, Roles: []string{"admin", "super_admin"},
		})
	}
	if h.sendVisitEmail(visit, false) {
		at := time.Now().UTC()
		visit.ConfirmationSentAt = &at
		_, _ = h.repo.UpdateVisitRequest(ctx, visit.ID, map[string]any{"confirmation_sent_at": at})
	}
	utils.SuccessResponse(c, http.StatusCreated, "Visit planned", publicVisit(&visit))
}

func (h *EngagementHandler) ListVisits(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100)
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !visitStatuses[status] {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid visit status")
		return
	}
	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	items, total, err := h.repo.ListVisitRequests(ctx, (page-1)*limit, limit, status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Unable to load visits")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Visits loaded", gin.H{"data": items, "total": total, "page": page, "limit": limit, "totalPages": (total + int64(limit) - 1) / int64(limit)})
}

func (h *EngagementHandler) UpdateVisit(c *gin.Context) {
	var req UpdateVisitRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	updates := map[string]any{}
	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	current, err := h.repo.GetVisitRequest(ctx, c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
		}
		utils.ErrorResponse(c, status, "Visit not found")
		return
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if !visitStatuses[status] {
			utils.ErrorResponse(c, http.StatusBadRequest, "Invalid visit status")
			return
		}
		if status != current.Status && !visitStatusTransitions[current.Status][status] {
			utils.ErrorResponse(c, http.StatusConflict, fmt.Sprintf("Visit cannot move from %s to %s", current.Status, status))
			return
		}
		now := time.Now().UTC()
		if status == "arrived" && now.Before(current.ServiceAt.Add(-90*time.Minute)) {
			utils.ErrorResponse(c, http.StatusConflict, "Visitor cannot be checked in before the arrival window")
			return
		}
		if status == "no_show" && now.Before(current.ServiceAt.Add(2*time.Hour)) {
			utils.ErrorResponse(c, http.StatusConflict, "No-show can only be recorded after service")
			return
		}
		updates["status"] = status
		if status == "arrived" {
			updates["checked_in_at"] = time.Now().UTC()
		}
		if status == "completed" || status == "cancelled" {
			updates["next_follow_up_at"] = nil
		}
	}
	if req.AssignedTo != nil {
		assignedTo := strings.TrimSpace(*req.AssignedTo)
		if assignedTo == "" {
			updates["assigned_to"] = nil
		} else if _, err := uuid.Parse(assignedTo); err != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "assignedTo must be a valid user id")
			return
		} else {
			updates["assigned_to"] = assignedTo
		}
	}
	if req.NextFollowUpAt != nil {
		updates["follow_up_notified_at"] = nil
		if strings.TrimSpace(*req.NextFollowUpAt) == "" {
			updates["next_follow_up_at"] = nil
		} else if parsed, err := time.Parse(time.RFC3339, *req.NextFollowUpAt); err == nil {
			updates["next_follow_up_at"] = parsed.UTC()
		} else {
			utils.ErrorResponse(c, http.StatusBadRequest, "nextFollowUpAt must be RFC3339")
			return
		}
	}
	if req.Contacted {
		updates["last_contact_at"] = time.Now().UTC()
	}
	if len(updates) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "No visit updates provided")
		return
	}
	visit, err := h.repo.UpdateVisitRequest(ctx, c.Param("id"), updates)
	if err != nil {
		status := http.StatusInternalServerError
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
		}
		utils.ErrorResponse(c, status, "Unable to update visit")
		return
	}
	actor, _ := middleware.GetUserIDFromContext(c)
	var actorID *string
	if strings.TrimSpace(actor) != "" {
		actorID = &actor
	}
	eventType := "updated"
	if visit.Status != current.Status {
		eventType = "status_changed"
	}
	_ = h.repo.CreateVisitActivity(ctx, &models.VisitActivity{
		VisitID: visit.ID, EventType: eventType, FromStatus: current.Status,
		ToStatus: visit.Status, ActorID: actorID, Notes: sanitize.Text(strings.TrimSpace(req.ActivityNotes)),
	})
	utils.SuccessResponse(c, http.StatusOK, "Visit updated", visit)
}

func (h *EngagementHandler) ListVisitActivities(c *gin.Context) {
	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	if _, err := h.repo.GetVisitRequest(ctx, c.Param("id")); err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Visit not found")
		return
	}
	items, err := h.repo.ListVisitActivities(ctx, c.Param("id"), 100)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Unable to load visit history")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Visit history loaded", items)
}

func (h *EngagementHandler) sendVisitEmail(visit models.VisitRequest, reminder bool) bool {
	if h.sender == nil || strings.TrimSpace(visit.Email) == "" {
		return false
	}
	subject, heading := "Your Sunday visit is confirmed", "We’re ready to welcome you"
	if reminder {
		subject, heading = "Your visit is tomorrow", "We’ll see you tomorrow"
	}
	body := fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:620px;margin:auto;padding:32px;color:#1b1712"><p style="color:#a87900;font-weight:700;letter-spacing:.12em;text-transform:uppercase">The Wisdom Church</p><h1>%s</h1><p>Hello %s,</p><p>Your place is set for <strong>%s</strong>.</p><div style="background:#f6f1e6;padding:20px;border-radius:16px"><strong>%s</strong><br>%s WAT<br>Honor Gardens, opposite Dominion City, Alasia, Lekki-Epe Expressway, Lagos</div><p>Reference: <strong>%s</strong></p><p>Our welcome team will be expecting %d %s.</p></div>`, html.EscapeString(heading), html.EscapeString(visit.FirstName), html.EscapeString(visit.ServiceType), visit.ServiceDate.Format("Sunday, 2 January 2006"), "9:00 AM", html.EscapeString(visit.ID), visit.Attendance, map[bool]string{true: "people", false: "person"}[visit.Attendance != 1])
	if err := h.sender.SendHTML(visit.Email, subject, body); err != nil {
		applog.L().Warn("visit email failed", "visit_id", visit.ID, "reminder", reminder, "error", err)
		return false
	}
	return true
}

func (h *EngagementHandler) ProcessVisitReminders(ctx context.Context, now time.Time) (int, error) {
	pending, err := h.repo.ListVisitConfirmationsPending(ctx, now.Add(-2*time.Minute), 200)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, visit := range pending {
		claimedAt := time.Now().UTC()
		claimed, claimErr := h.repo.ClaimVisitDelivery(ctx, visit.ID, "confirmation_sent_at", claimedAt)
		if claimErr != nil || !claimed {
			continue
		}
		if !h.sendVisitEmail(visit, false) {
			_ = h.repo.ReleaseVisitDelivery(ctx, visit.ID, "confirmation_sent_at")
			continue
		}
		if err := h.repo.CompleteVisitDelivery(ctx, visit.ID, "confirmation_sent_at", time.Now().UTC()); err != nil {
			_ = h.repo.ReleaseVisitDelivery(ctx, visit.ID, "confirmation_sent_at")
			continue
		}
		processed++
	}

	items, err := h.repo.ListVisitRemindersDue(ctx, now, now.Add(25*time.Hour), 200)
	if err != nil {
		return processed, err
	}
	for _, visit := range items {
		claimedAt := time.Now().UTC()
		claimed, claimErr := h.repo.ClaimVisitDelivery(ctx, visit.ID, "reminder_sent_at", claimedAt)
		if claimErr != nil || !claimed {
			continue
		}
		if !h.sendVisitEmail(visit, true) {
			_ = h.repo.ReleaseVisitDelivery(ctx, visit.ID, "reminder_sent_at")
			continue
		}
		if err := h.repo.CompleteVisitDelivery(ctx, visit.ID, "reminder_sent_at", time.Now().UTC()); err != nil {
			_ = h.repo.ReleaseVisitDelivery(ctx, visit.ID, "reminder_sent_at")
			continue
		}
		updates := map[string]any{}
		if visit.Status == "new" {
			updates["status"] = "confirmed"
		}
		if len(updates) > 0 {
			_, _ = h.repo.UpdateVisitRequest(ctx, visit.ID, updates)
		}
		processed++
	}
	followUps, err := h.repo.ListVisitFollowUpsDue(ctx, now, 200)
	if err != nil {
		return processed, err
	}
	for _, visit := range followUps {
		if h.notifySvc == nil {
			continue
		}
		claimedAt := time.Now().UTC()
		claimed, claimErr := h.repo.ClaimVisitDelivery(ctx, visit.ID, "follow_up_notified_at", claimedAt)
		if claimErr != nil || !claimed {
			continue
		}
		entityType, entityID := "visit_request", visit.ID
		err := h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type: "visit_follow_up_due", Title: "Visitor follow-up is due",
			Message:    fmt.Sprintf("Follow up with %s %s after %s.", visit.FirstName, visit.LastName, visit.ServiceType),
			EntityType: &entityType, EntityID: &entityID, Roles: []string{"admin", "super_admin"},
		})
		if err != nil {
			_ = h.repo.ReleaseVisitDelivery(ctx, visit.ID, "follow_up_notified_at")
			continue
		}
		if err := h.repo.CompleteVisitDelivery(ctx, visit.ID, "follow_up_notified_at", time.Now().UTC()); err != nil {
			_ = h.repo.ReleaseVisitDelivery(ctx, visit.ID, "follow_up_notified_at")
			continue
		}
		processed++
	}
	return processed, nil
}
