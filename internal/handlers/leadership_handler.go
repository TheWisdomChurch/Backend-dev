package handlers

import (
	"context"
	"log"
	"net/http"
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

type LeadershipHandler struct {
	svc      service.LeadershipService
	uploader service.AssetUploader
	userRepo repository.UserRepository
}

func NewLeadershipHandler(svc service.LeadershipService, uploader service.AssetUploader, userRepo repository.UserRepository) *LeadershipHandler {
	return &LeadershipHandler{
		svc:      svc,
		uploader: uploader,
		userRepo: userRepo,
	}
}

func (h *LeadershipHandler) currentUser(c *gin.Context) *models.User {
	if h.userRepo == nil {
		return nil
	}
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

// Public: list approved leadership members
func (h *LeadershipHandler) ListPublic(c *gin.Context) {
	role := c.Query("role")
	items, err := h.svc.ListApproved(role)
	if err != nil {
		log.Printf("leadership public list failed role=%q: %v", role, err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load leadership")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Leadership loaded", items)
}

// Public: apply for leadership
func (h *LeadershipHandler) Apply(c *gin.Context) {
	var req models.CreateLeadershipRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Apply(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Application submitted", member)
}

// Public: upload leadership profile image (max 5MB)
func (h *LeadershipHandler) UploadImage(c *gin.Context) {
	if h.uploader == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Storage uploader not configured")
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "file is required")
		return
	}

	const maxBytes = 5 << 20
	if fh.Size > maxBytes {
		utils.ErrorResponse(c, http.StatusBadRequest, "file too large (max 5MB)")
		return
	}

	src, err := fh.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer src.Close()

	contentType := fh.Header.Get("Content-Type")
	ext, ok := allowedImageExt(contentType)
	if !ok {
		utils.ErrorResponse(c, http.StatusBadRequest, "only png, jpg, webp allowed")
		return
	}

	objectKey, err := h.uploader.BuildGenericAssetKey("leadership/profiles", ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	url, err := h.uploader.Upload(ctx, objectKey, contentType, src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to storage failed")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Upload successful", gin.H{
		"url": url,
		"key": objectKey,
	})
}

// Admin: list leadership applications/members
func (h *LeadershipHandler) List(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 10, 300)
	if !ok {
		return
	}

	role, ok := parseEnumQuery(c, "role", "role", []string{
		"senior_pastor",
		"associate_pastor",
		"deacon",
		"deaconess",
		"reverend",
	})
	if !ok {
		return
	}
	status, ok := parseEnumQuery(c, "status", "status", []string{"pending", "awaiting_super_admin_approval", "approved", "declined"})
	if !ok {
		return
	}

	items, total, err := h.svc.List(page, limit, role, status)
	if err != nil {
		log.Printf("leadership admin list failed page=%d limit=%d role=%q status=%q: %v", page, limit, role, status, err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load leadership")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Leadership loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

// Admin: create leadership member
func (h *LeadershipHandler) Create(c *gin.Context) {
	var req models.CreateLeadershipRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Create(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Leadership member created", member)
}

// Admin: update leadership member
func (h *LeadershipHandler) Update(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "leadership member id")
	if !ok {
		return
	}

	var req models.UpdateLeadershipRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Update(id, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Leadership member updated", member)
}

// Super-admin: approve leadership member
func (h *LeadershipHandler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "leadership member id")
	if !ok {
		return
	}

	member, err := h.svc.Approve(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Leadership request approved", member)
}

// Super-admin: decline leadership member
func (h *LeadershipHandler) Decline(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "leadership member id")
	if !ok {
		return
	}

	member, err := h.svc.Decline(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Leadership request declined", member)
}

// Admin: request leadership deletion. Super admin approval performs the actual delete.
func (h *LeadershipHandler) Delete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "leadership member id")
	if !ok {
		return
	}
	req, err := h.svc.RequestDelete(id, h.currentUser(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusAccepted, "Leadership delete request sent for super admin approval", req)
}

// Super-admin: approve leadership deletion
func (h *LeadershipHandler) ApproveDelete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "leadership member id or approval request id")
	if !ok {
		return
	}

	if err := h.svc.ApproveDelete(id, h.currentUser(c)); err != nil {
		message := strings.TrimSpace(err.Error())
		if message == "" {
			message = "Failed to approve leadership delete request"
		}

		lowerMessage := strings.ToLower(message)

		switch {
		case strings.Contains(lowerMessage, "approval service"):
			utils.ErrorResponse(c, http.StatusInternalServerError, message)

		case strings.Contains(lowerMessage, "not found"):
			utils.ErrorResponse(c, http.StatusNotFound, message)

		default:
			utils.ErrorResponse(c, http.StatusBadRequest, message)
		}

		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Leadership delete request approved", gin.H{
		"deleted": true,
	})
}

func (h *LeadershipHandler) BirthdayStats(c *gin.Context) {
	stats, err := h.svc.BirthdayStats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load birthday stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthday stats retrieved", stats)
}

func (h *LeadershipHandler) BirthdaysByMonth(c *gin.Context) {
	month, ok := parseMonthPathParam(c, "month")
	if !ok {
		return
	}
	items, err := h.svc.BirthdaysByMonth(month)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "month must be 1-12") {
			utils.ErrorResponse(c, http.StatusBadRequest, "month must be 1-12")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load birthdays")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthdays retrieved", gin.H{
		"month": month,
		"data":  items,
	})
}

func (h *LeadershipHandler) BirthdaysToday(c *gin.Context) {
	items, err := h.svc.BirthdaysToday(time.Now())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load today's birthdays")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's birthdays retrieved", gin.H{
		"data": items,
	})
}

func (h *LeadershipHandler) SendBirthdaysToday(c *gin.Context) {
	now := time.Now()
	result, err := h.svc.SendBirthdayGreetings(int(now.Month()), now.Day())
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthday emails queued/sent", result)
}

func (h *LeadershipHandler) AnniversaryStats(c *gin.Context) {
	stats, err := h.svc.AnniversaryStats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load anniversary stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Anniversary stats retrieved", stats)
}

func (h *LeadershipHandler) AnniversariesByMonth(c *gin.Context) {
	month, ok := parseMonthPathParam(c, "month")
	if !ok {
		return
	}
	items, err := h.svc.AnniversariesByMonth(month)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "month must be 1-12") {
			utils.ErrorResponse(c, http.StatusBadRequest, "month must be 1-12")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load anniversaries")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Anniversaries retrieved", gin.H{
		"month": month,
		"data":  items,
	})
}

func (h *LeadershipHandler) AnniversariesToday(c *gin.Context) {
	items, err := h.svc.AnniversariesToday(time.Now())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load today's anniversaries")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's anniversaries retrieved", gin.H{
		"data": items,
	})
}

func (h *LeadershipHandler) SendAnniversariesToday(c *gin.Context) {
	now := time.Now()
	result, err := h.svc.SendAnniversaryGreetings(int(now.Month()), now.Day())
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Anniversary emails queued/sent", result)
}
