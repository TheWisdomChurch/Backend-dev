package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type EventHandler struct {
	repo        *repository.EventRepository
	storage     service.AssetUploader
	userRepo    repository.UserRepository
	approvalSvc service.ApprovalService
	notifySvc   service.AdminNotificationService
}

func NewEventHandler(
	repo *repository.EventRepository,
	storage service.AssetUploader,
	userRepo repository.UserRepository,
	approvalSvc service.ApprovalService,
	notifySvc service.AdminNotificationService,
) *EventHandler {
	return &EventHandler{
		repo:        repo,
		storage:     storage,
		userRepo:    userRepo,
		approvalSvc: approvalSvc,
		notifySvc:   notifySvc,
	}
}

func (h *EventHandler) currentUser(c *gin.Context) *models.User {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		return nil
	}
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		return nil
	}
	return user
}

func normalizedRole(c *gin.Context) string {
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	return role
}

func isAdminOrSuper(role string) bool {
	return role == "admin" || role == "super_admin"
}

func (h *EventHandler) List(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 10, 300)
	if !ok {
		return
	}
	offset := (page - 1) * limit

	statusQuery, ok := parseEnumQuery(c, "status", "status", []string{"upcoming", "happening", "past", "ongoing", "completed"})
	if !ok {
		return
	}
	statusFilter := normalizeIncomingEventStatus(statusQuery) // upcoming|happening|past (+ aliases)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	items, total, err := h.repo.ListWithContext(ctx, offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load events")
		return
	}

	now := time.Now().UTC()
	filtered := make([]models.Event, 0, len(items))
	role := normalizedRole(c)
	allowPending := isAdminOrSuper(role)
	for i := range items {
		items[i].Status = deriveStatus(items[i].Date, items[i].Time, now)
		if !allowPending && !items[i].IsApproved {
			continue
		}
		if statusFilter == "" || string(items[i].Status) == statusFilter {
			normalizeOutgoingEventStatus(&items[i])
			filtered = append(filtered, items[i])
		}
	}

	if statusFilter != "" || !allowPending {
		total = int64(len(filtered))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       filtered,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *EventHandler) Get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "event id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	item, err := h.repo.GetByIDWithContext(ctx, id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}
	role := normalizedRole(c)
	allowPending := isAdminOrSuper(role)
	if !allowPending && !item.IsApproved {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}
	normalizeOutgoingEventStatus(item)
	utils.SuccessResponse(c, http.StatusOK, "Event retrieved", item)
}

func (h *EventHandler) Create(c *gin.Context) {
	var req models.Event
	if !validation.BindJSON(c, &req) {
		return
	}

	req.Status = deriveStatus(req.Date, req.Time, time.Now().UTC())
	role := normalizedRole(c)
	approver := h.currentUser(c)
	if role == "super_admin" && approver != nil {
		req.IsApproved = true
		req.ApprovedByID = &approver.ID
		name := strings.TrimSpace(strings.Join([]string{approver.FirstName, approver.LastName}, " "))
		if name != "" {
			req.ApprovedByName = &name
		}
		if approver.Email != "" {
			email := approver.Email
			req.ApprovedByEmail = &email
		}
		now := time.Now().UTC()
		req.ApprovedAt = &now
	} else {
		req.IsApproved = false
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := h.repo.CreateWithContext(ctx, &req); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create event")
		return
	}

	if !req.IsApproved && h.approvalSvc != nil {
		entityID := req.ID
		label := strings.TrimSpace(req.Title)
		if label == "" {
			label = "Event"
		}
		approvalReq, err := h.approvalSvc.CreateRequest(service.CreateApprovalRequest{
			Type:        models.ApprovalTypeEvent,
			EntityID:    &entityID,
			EntityLabel: &label,
			RequestedByID: func() *string {
				if approver == nil {
					return nil
				}
				return &approver.ID
			}(),
			RequestedByName: func() *string {
				if approver == nil {
					return nil
				}
				name := strings.TrimSpace(strings.Join([]string{approver.FirstName, approver.LastName}, " "))
				if name == "" {
					return nil
				}
				return &name
			}(),
			RequestedByEmail: func() *string {
				if approver == nil || approver.Email == "" {
					return nil
				}
				email := approver.Email
				return &email
			}(),
		})
		if err == nil && h.notifySvc != nil {
			title := "New event approval request"
			message := fmt.Sprintf("A new event is awaiting approval. Ticket %s.", approvalReq.TicketCode)
			_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
				Type:       "event_request",
				Title:      title,
				Message:    message,
				TicketCode: &approvalReq.TicketCode,
				EntityType: func() *string { t := "event"; return &t }(),
				EntityID:   &entityID,
				Roles:      []string{"admin", "super_admin"},
			})
		}
	}

	normalizeOutgoingEventStatus(&req)
	utils.SuccessResponse(c, http.StatusCreated, "Event created", req)
}

func (h *EventHandler) Update(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "event id")
	if !ok {
		return
	}

	var req models.Event
	if !validation.BindJSON(c, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 7*time.Second)
	defer cancel()

	existing, err := h.repo.GetByIDWithContext(ctx, id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}

	existing.Title = req.Title
	existing.ShortDescription = req.ShortDescription
	existing.Description = req.Description
	existing.Date = req.Date
	existing.Time = req.Time
	existing.Location = req.Location
	existing.Category = req.Category
	existing.Status = deriveStatus(req.Date, req.Time, time.Now().UTC())
	existing.IsFeatured = req.IsFeatured
	existing.Tags = req.Tags
	existing.RegisterLink = req.RegisterLink
	existing.Speaker = req.Speaker
	existing.ContactPhone = req.ContactPhone
	existing.Image = req.Image
	existing.BannerImage = req.BannerImage

	if err := h.repo.UpdateWithContext(ctx, existing); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update event")
		return
	}

	normalizeOutgoingEventStatus(existing)
	utils.SuccessResponse(c, http.StatusOK, "Event updated", existing)
}

func (h *EventHandler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "event id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	existing, err := h.repo.GetByIDWithContext(ctx, id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}

	approver := h.currentUser(c)
	if approver != nil {
		existing.IsApproved = true
		existing.ApprovedByID = &approver.ID
		name := strings.TrimSpace(strings.Join([]string{approver.FirstName, approver.LastName}, " "))
		if name != "" {
			existing.ApprovedByName = &name
		}
		if approver.Email != "" {
			email := approver.Email
			existing.ApprovedByEmail = &email
		}
		now := time.Now().UTC()
		existing.ApprovedAt = &now
	} else {
		existing.IsApproved = true
	}

	if err := h.repo.UpdateWithContext(ctx, existing); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to approve event")
		return
	}

	if h.approvalSvc != nil {
		_, _ = h.approvalSvc.CompleteRequest(models.ApprovalTypeEvent, id, models.ApprovalStatusApproved, approver)
	}
	if h.notifySvc != nil {
		title := "Event approved"
		message := "An event has been approved and published."
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "event_approved",
			Title:      title,
			Message:    message,
			EntityType: func() *string { t := "event"; return &t }(),
			EntityID:   &id,
			Roles:      []string{"admin", "super_admin"},
		})
	}

	normalizeOutgoingEventStatus(existing)
	utils.SuccessResponse(c, http.StatusOK, "Event approved", existing)
}

// Delete does not remove the event directly — any admin can ask for an event
// to come down, but only a super admin can actually take it off the live
// site. This creates a pending approval_requests ticket (type event_delete)
// carrying the requester's stated reason; ApproveDeleteEvent performs the
// actual deletion once a super admin signs off.
func (h *EventHandler) Delete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "event id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason" binding:"required"`
	}
	if !validation.BindJSON(c, &body) {
		return
	}

	if h.approvalSvc == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Approval service not configured")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	event, err := h.repo.GetByIDWithContext(ctx, id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}

	requester := h.currentUser(c)
	var requestedByID, requestedByName, requestedByEmail *string
	if requester != nil {
		requestedByID = &requester.ID
		name := strings.TrimSpace(strings.Join([]string{requester.FirstName, requester.LastName}, " "))
		if name != "" {
			requestedByName = &name
		}
		if requester.Email != "" {
			email := requester.Email
			requestedByEmail = &email
		}
	}

	label := strings.TrimSpace(event.Title)
	if label == "" {
		label = id
	}
	reason := strings.TrimSpace(body.Reason)

	req, err := h.approvalSvc.CreateRequest(service.CreateApprovalRequest{
		Type:             models.ApprovalTypeEventDelete,
		EntityID:         &id,
		EntityLabel:      &label,
		Reason:           &reason,
		RequestedByID:    requestedByID,
		RequestedByName:  requestedByName,
		RequestedByEmail: requestedByEmail,
	})
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if h.notifySvc != nil {
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "event_delete_request",
			Title:      "Event delete approval required",
			Message:    fmt.Sprintf("%q was marked for removal: %s", label, reason),
			TicketCode: &req.TicketCode,
			EntityType: func() *string { t := "event_delete"; return &t }(),
			EntityID:   &id,
			Roles:      []string{"super_admin"},
		})
	}

	utils.SuccessResponse(c, http.StatusAccepted, "Delete request sent for super admin approval", req)
}

// ApproveDeleteEvent performs the actual removal once a super admin has
// approved the pending event_delete request. Accepts either the event's own
// id or the approval request's id, matching the workforce/leadership
// delete-approve pattern.
func (h *EventHandler) ApproveDeleteEvent(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "event id")
	if !ok {
		return
	}

	if h.approvalSvc == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Approval service not configured")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	approver := h.currentUser(c)
	entityID := id

	event, err := h.repo.GetByIDWithContext(ctx, entityID)
	if err != nil {
		// id might be the approval request's own id rather than the event's.
		req, reqErr := h.approvalSvc.GetRequest(id)
		if reqErr != nil || req.Type != models.ApprovalTypeEventDelete || req.EntityID == nil {
			utils.ErrorResponse(c, http.StatusNotFound, "Event or delete request not found")
			return
		}
		entityID = strings.TrimSpace(*req.EntityID)
		event, err = h.repo.GetByIDWithContext(ctx, entityID)
		if err != nil {
			// Event already gone — still resolve the ticket so it doesn't
			// sit pending forever.
			_, _ = h.approvalSvc.CompleteRequestByID(req.ID, models.ApprovalStatusApproved, approver)
			utils.SuccessResponse(c, http.StatusOK, "Event already removed", nil)
			return
		}
	}

	label := strings.TrimSpace(event.Title)
	if label == "" {
		label = entityID
	}

	req, err := h.approvalSvc.CompleteRequest(models.ApprovalTypeEventDelete, entityID, models.ApprovalStatusApproved, approver)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repo.DeleteWithContext(ctx, entityID); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete event")
		return
	}

	if h.notifySvc != nil {
		ticket := ""
		if req != nil {
			ticket = req.TicketCode
		}
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "event_delete_approved",
			Title:      "Event removed",
			Message:    fmt.Sprintf("%q has been approved for removal and taken off the site.", label),
			TicketCode: &ticket,
			EntityType: func() *string { t := "event_delete"; return &t }(),
			EntityID:   &entityID,
			Roles:      []string{"admin", "super_admin"},
		})
	}

	utils.SuccessResponse(c, http.StatusOK, "Event deleted", nil)
}

func (h *EventHandler) UploadImage(c *gin.Context)  { h.uploadEventAsset(c, "image") }
func (h *EventHandler) UploadBanner(c *gin.Context) { h.uploadEventAsset(c, "banner") }

func (h *EventHandler) uploadEventAsset(c *gin.Context, kind string) {
	if h.storage == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Storage uploader not configured")
		return
	}

	eventID, ok := parseUUIDParam(c, "id", "event id")
	if !ok {
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "file is required")
		return
	}

	const maxBytes = 10 << 20
	if fh.Size > maxBytes {
		utils.ErrorResponse(c, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	src, err := fh.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer src.Close()

	ct := fh.Header.Get("Content-Type")
	ext, ok := allowedImageExt(ct)
	if !ok {
		utils.ErrorResponse(c, http.StatusBadRequest, "only png, jpg, webp allowed")
		return
	}

	objectKey, err := h.storage.BuildEventAssetKey(eventID, kind, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	cdnURL, err := h.storage.Upload(ctx, objectKey, ct, src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to storage failed")
		return
	}

	key := objectKey
	switch kind {
	case "image":
		if err := h.repo.SetImageWithContext(ctx, eventID, cdnURL, &key); err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to save image url")
			return
		}
	case "banner":
		if err := h.repo.SetBannerImageWithContext(ctx, eventID, cdnURL, &key); err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to save banner url")
			return
		}
	default:
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid asset type")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Upload successful", gin.H{
		"eventId": eventID,
		"kind":    kind,
		"url":     cdnURL,
		"key":     objectKey,
	})
}

func allowedImageExt(contentType string) (string, bool) {
	switch contentType {
	case "image/png":
		return "png", true
	case "image/jpeg":
		return "jpg", true
	case "image/jpg":
		return "jpg", true
	case "image/webp":
		return "webp", true
	default:
		return "", false
	}
}

func deriveStatus(dateStr, timeStr string, now time.Time) models.EventStatus {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return models.EventStatusUpcoming
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return models.EventStatusUpcoming
	}

	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()

	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return models.EventStatusHappening
	case t.After(time.Date(y2, m2, d2, 23, 59, 59, 0, time.UTC)):
		return models.EventStatusUpcoming
	default:
		return models.EventStatusPast
	}
}

func normalizeIncomingEventStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ongoing":
		return "happening"
	case "completed":
		return "past"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func normalizeOutgoingEventStatus(event *models.Event) {
	if event == nil {
		return
	}
	switch event.Status {
	case models.EventStatusHappening:
		event.Status = models.EventStatus("ongoing")
	case models.EventStatusPast:
		event.Status = models.EventStatus("completed")
	}
}

func parseIntClamp(s string, min, max int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
