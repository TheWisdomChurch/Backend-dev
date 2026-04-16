package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/pkg/utils"
)

const (
	siteContentHomepageAdKey = "homepage_ad"
	siteContentConfessionKey = "confession_popup"
)

type SiteContentHandler struct {
	db *database.Database
}

func NewSiteContentHandler(db *database.Database) *SiteContentHandler {
	return &SiteContentHandler{db: db}
}

type HomepageAdPayload struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Headline    string `json:"headline"`
	Description string `json:"description"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt"`
	Time        string `json:"time"`
	Location    string `json:"location"`
	Image       string `json:"image"`
	RegisterURL string `json:"registerUrl"`
	CTALabel    string `json:"ctaLabel"`
	Note        string `json:"note"`
}

type ConfessionPopupPayload struct {
	WelcomeTitle   string `json:"welcomeTitle"`
	WelcomeMessage string `json:"welcomeMessage"`
	ConfessionText string `json:"confessionText"`
	Motto          string `json:"motto"`
}

var defaultHomepageAd = HomepageAdPayload{
	ID:          "wpc-2026",
	Title:       "Wisdom Power Conference 2026",
	Headline:    "Have you registered for WPC 2026?",
	Description: "Join three days of worship, impartation, and encounters designed to refresh your spirit and strengthen your walk.",
	StartAt:     "2026-03-20T18:00:00Z",
	EndAt:       "2026-03-22T20:00:00Z",
	Time:        "Morning Session • Evening Session",
	Location:    "Honor Gardens opposite Dominion City, Alasia Bus stop",
	Image:       "/HEADER.png",
	RegisterURL: "https://admin.wisdomchurchhq.org/forms/wpc26",
	CTALabel:    "Register now",
	Note:        "You will be returned to the main website after you finish.",
}

var defaultConfession = ConfessionPopupPayload{
	WelcomeTitle:   "Welcome Home",
	WelcomeMessage: "You are in a place of worship, truth, and transformation. Before you continue, take a moment with our confession and align your words with faith.",
	ConfessionText: "We begin to prosper, we continue to prosper, until we become very prosperous.",
	Motto:          "We begin to prosper, we continue to prosper, until we become very prosperous.",
}

func (h *SiteContentHandler) GetHomepageAd(c *gin.Context) {
	var payload HomepageAdPayload
	if err := h.loadContent(siteContentHomepageAdKey, &payload); err != nil {
		payload = defaultHomepageAd
	}
	utils.SuccessResponse(c, http.StatusOK, "Homepage ad content loaded", payload)
}

func (h *SiteContentHandler) GetConfessionPopup(c *gin.Context) {
	var payload ConfessionPopupPayload
	if err := h.loadContent(siteContentConfessionKey, &payload); err != nil {
		payload = defaultConfession
	}
	utils.SuccessResponse(c, http.StatusOK, "Confession popup content loaded", payload)
}

func (h *SiteContentHandler) GetAdminHomepageAd(c *gin.Context) {
	var payload HomepageAdPayload
	if err := h.loadContent(siteContentHomepageAdKey, &payload); err != nil {
		payload = defaultHomepageAd
	}
	utils.SuccessResponse(c, http.StatusOK, "Homepage ad content loaded", payload)
}

func (h *SiteContentHandler) UpdateAdminHomepageAd(c *gin.Context) {
	var payload HomepageAdPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid homepage ad payload")
		return
	}

	if payload.Title == "" || payload.Headline == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "title and headline are required")
		return
	}

	if err := h.saveContent(siteContentHomepageAdKey, payload, c.GetString("email")); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to save homepage ad content")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Homepage ad content updated", payload)
}

func (h *SiteContentHandler) GetAdminConfessionPopup(c *gin.Context) {
	var payload ConfessionPopupPayload
	if err := h.loadContent(siteContentConfessionKey, &payload); err != nil {
		payload = defaultConfession
	}
	utils.SuccessResponse(c, http.StatusOK, "Confession popup content loaded", payload)
}

func (h *SiteContentHandler) UpdateAdminConfessionPopup(c *gin.Context) {
	var payload ConfessionPopupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid confession popup payload")
		return
	}

	if payload.WelcomeTitle == "" || payload.ConfessionText == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "welcomeTitle and confessionText are required")
		return
	}

	if err := h.saveContent(siteContentConfessionKey, payload, c.GetString("email")); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to save confession popup content")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Confession popup content updated", payload)
}

func (h *SiteContentHandler) loadContent(key string, out interface{}) error {
	ctx, cancel := contextWithTimeout()
	defer cancel()

	var row models.SiteContent
	if err := h.db.WithContext(ctx).Where("key = ?", key).First(&row).Error; err != nil {
		return err
	}
	return json.Unmarshal(row.Payload, out)
}

func (h *SiteContentHandler) saveContent(key string, payload interface{}, updatedBy string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var updatedByPtr *string
	if updatedBy != "" {
		updatedByCopy := updatedBy
		updatedByPtr = &updatedByCopy
	}

	ctx, cancel := contextWithTimeout()
	defer cancel()

	row := models.SiteContent{
		Key:       key,
		Payload:   raw,
		UpdatedBy: updatedByPtr,
	}

	return h.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_by", "updated_at"}),
	}).Create(&row).Error
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
