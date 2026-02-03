package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type EventHandler struct {
	repo  *repository.EventRepository
	bunny *service.BunnyUploader
}

func NewEventHandler(repo *repository.EventRepository, bunny *service.BunnyUploader) *EventHandler {
	return &EventHandler{repo: repo, bunny: bunny}
}

func (h *EventHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)
	offset := (page - 1) * limit

	statusFilter := strings.TrimSpace(c.Query("status")) // upcoming|happening|past

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	items, total, err := h.repo.ListWithContext(ctx, offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load events")
		return
	}

	now := time.Now().UTC()
	filtered := make([]models.Event, 0, len(items))
	for i := range items {
		items[i].Status = deriveStatus(items[i].Date, items[i].Time, now)
		if statusFilter == "" || string(items[i].Status) == statusFilter {
			filtered = append(filtered, items[i])
		}
	}

	if statusFilter != "" {
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
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	item, err := h.repo.GetByIDWithContext(ctx, id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Event retrieved", item)
}

func (h *EventHandler) Create(c *gin.Context) {
	var req models.Event
	if !validation.BindJSON(c, &req) {
		return
	}

	req.Status = deriveStatus(req.Date, req.Time, time.Now().UTC())

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := h.repo.CreateWithContext(ctx, &req); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create event")
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Event created", req)
}

func (h *EventHandler) Update(c *gin.Context) {
	id := c.Param("id")

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

	utils.SuccessResponse(c, http.StatusOK, "Event updated", existing)
}

func (h *EventHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := h.repo.DeleteWithContext(ctx, id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete event")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Event deleted", nil)
}

func (h *EventHandler) UploadImage(c *gin.Context)  { h.uploadEventAsset(c, "image") }
func (h *EventHandler) UploadBanner(c *gin.Context) { h.uploadEventAsset(c, "banner") }

func (h *EventHandler) uploadEventAsset(c *gin.Context, kind string) {
	if h.bunny == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "CDN uploader not configured")
		return
	}

	eventID := c.Param("id")

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

	objectKey, err := h.bunny.BuildEventAssetKey(eventID, kind, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	cdnURL, err := h.bunny.Upload(ctx, objectKey, ct, src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to CDN failed")
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
