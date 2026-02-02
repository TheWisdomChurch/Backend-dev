package handlers

import (
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

// ✅ updated constructor (inject bunny)
func NewEventHandler(repo *repository.EventRepository, bunny *service.BunnyUploader) *EventHandler {
	return &EventHandler{repo: repo, bunny: bunny}
}

func (h *EventHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	statusFilter := c.Query("status") // optional: upcoming|happening|past
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	items, total, err := h.repo.List(offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load events")
		return
	}

	now := time.Now().UTC()
	filtered := make([]models.Event, 0, len(items))
	for i := range items {
		items[i].Status = deriveStatus(items[i].Date, items[i].Time, now)
		// optional: persist if changed
		_ = h.repo.SetStatus(items[i].ID, items[i].Status)
		if statusFilter == "" || string(items[i].Status) == statusFilter {
			filtered = append(filtered, items[i])
		}
	}

	// adjust total if filter used
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
	item, err := h.repo.GetByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *EventHandler) Create(c *gin.Context) {
	var req models.Event
	if !validation.BindJSON(c, &req) {
		return
	}

	now := time.Now().UTC()
	req.Status = deriveStatus(req.Date, req.Time, now)

	if err := h.repo.Create(&req); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create event")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": req})
}

func (h *EventHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.repo.GetByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Event not found")
		return
	}

	var req models.Event
	if !validation.BindJSON(c, &req) {
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

	if err := h.repo.Update(existing); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update event")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": existing})
}

func (h *EventHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete event")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Event deleted"})
}

// =======================
// ✅ NEW: Upload endpoints
// =======================

func (h *EventHandler) UploadImage(c *gin.Context) {
	h.uploadEventAsset(c, "image")
}

func (h *EventHandler) UploadBanner(c *gin.Context) {
	h.uploadEventAsset(c, "banner")
}

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

	// Size limit: 10MB (adjust as needed)
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
	ext := ""
	switch ct {
	case "image/png":
		ext = "png"
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	default:
		utils.ErrorResponse(c, http.StatusBadRequest, "only png, jpg, webp allowed")
		return
	}

	objectKey, err := h.bunny.BuildEventAssetKey(eventID, kind, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	cdnURL, err := h.bunny.Upload(c.Request.Context(), objectKey, ct, src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to CDN failed")
		return
	}

	key := objectKey

	switch kind {
	case "image":
		if err := h.repo.SetImage(eventID, cdnURL, &key); err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to save image url")
			return
		}
	case "banner":
		if err := h.repo.SetBannerImage(eventID, cdnURL, &key); err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to save banner url")
			return
		}
	default:
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid asset type")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"eventId": eventID,
		"kind":    kind,
		"url":     cdnURL,
	})
}

// deriveStatus determines event status from date (YYYY-MM-DD) relative to now (UTC).
func deriveStatus(dateStr, timeStr string, now time.Time) models.EventStatus {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return models.EventStatusUpcoming
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return models.EventStatusUpcoming
	}

	// Compare by date only
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
