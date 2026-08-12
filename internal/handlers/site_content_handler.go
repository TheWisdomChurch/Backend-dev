package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/pkg/utils"
)

const (
	siteContentHomepageAdKey = "homepage_ad"
	siteContentConfessionKey = "confession_popup"
	siteContentAboutKey      = "about_page"
)

type SiteContentHandler struct {
	repo repository.SiteContentRepository
}

func NewSiteContentHandler(db *database.Database) *SiteContentHandler {
	return &SiteContentHandler{repo: repository.NewSiteContentRepository(db)}
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

type AboutPillarPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type AboutPagePayload struct {
	Eyebrow      string               `json:"eyebrow"`
	Title        string               `json:"title"`
	Subtitle     string               `json:"subtitle"`
	StoryTitle   string               `json:"storyTitle"`
	StoryBody    string               `json:"storyBody"`
	StoryImage   string               `json:"storyImage"`
	CultureTitle string               `json:"cultureTitle"`
	Pillars      []AboutPillarPayload `json:"pillars"`
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

var defaultAboutPage = AboutPagePayload{
	Eyebrow: "About Wisdom Church", Title: "Raising complete believers.",
	Subtitle:   "A Spirit-filled community built on Word, worship, and intentional discipleship.",
	StoryTitle: "A church where people grow in Christ.",
	StoryBody:  "The Wisdom Church is a trans-generational community in Lagos, committed to forming complete believers through sound teaching, worshipful community, and faithful pastoral care.",
	Pillars: []AboutPillarPayload{
		{Title: "Presence-driven worship", Body: "We gather to host the presence of God with reverence, expectation, and joy."},
		{Title: "Word-shaped discipleship", Body: "Teaching is practical and biblical, aimed at forming complete believers."},
		{Title: "People-first community", Body: "Hospitality, accountability, and care are central to how we build family."},
		{Title: "Excellence with integrity", Body: "We steward people and service moments with clarity, order, and consistency."},
	},
}

func (h *SiteContentHandler) GetHomepageAd(c *gin.Context) {
	var payload HomepageAdPayload
	if err := h.loadContent(c, siteContentHomepageAdKey, &payload); err != nil {
		payload = defaultHomepageAd
	}
	utils.SuccessResponse(c, http.StatusOK, "Homepage ad content loaded", payload)
}

func (h *SiteContentHandler) GetConfessionPopup(c *gin.Context) {
	var payload ConfessionPopupPayload
	if err := h.loadContent(c, siteContentConfessionKey, &payload); err != nil {
		payload = defaultConfession
	}
	utils.SuccessResponse(c, http.StatusOK, "Confession popup content loaded", payload)
}

func (h *SiteContentHandler) GetAboutPage(c *gin.Context) {
	payload := defaultAboutPage
	if err := h.loadContent(c, siteContentAboutKey, &payload); err != nil {
		payload = defaultAboutPage
	}
	utils.SuccessResponse(c, http.StatusOK, "About page content loaded", payload)
}

func (h *SiteContentHandler) GetAdminAboutPage(c *gin.Context) {
	h.GetAboutPage(c)
}

func (h *SiteContentHandler) UpdateAdminAboutPage(c *gin.Context) {
	var payload AboutPagePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid about page payload")
		return
	}
	if payload.Title == "" || payload.StoryTitle == "" || len(payload.Pillars) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "title, storyTitle, and pillars are required")
		return
	}
	if err := h.saveContent(c, siteContentAboutKey, payload, c.GetString("email")); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to save about page content")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "About page content updated", payload)
}

func (h *SiteContentHandler) GetAdminHomepageAd(c *gin.Context) {
	var payload HomepageAdPayload
	if err := h.loadContent(c, siteContentHomepageAdKey, &payload); err != nil {
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
	if err := h.saveContent(c, siteContentHomepageAdKey, payload, c.GetString("email")); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to save homepage ad content")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Homepage ad content updated", payload)
}

func (h *SiteContentHandler) GetAdminConfessionPopup(c *gin.Context) {
	var payload ConfessionPopupPayload
	if err := h.loadContent(c, siteContentConfessionKey, &payload); err != nil {
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
	if err := h.saveContent(c, siteContentConfessionKey, payload, c.GetString("email")); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to save confession popup content")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Confession popup content updated", payload)
}

func (h *SiteContentHandler) loadContent(c *gin.Context, key string, out interface{}) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	row, err := h.repo.GetByKey(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(row.Payload, out)
}

func (h *SiteContentHandler) saveContent(c *gin.Context, key string, payload interface{}, updatedBy string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var updatedByPtr *string
	if updatedBy != "" {
		cp := updatedBy
		updatedByPtr = &cp
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	return h.repo.Upsert(ctx, &models.SiteContent{
		Key:       key,
		Payload:   raw,
		UpdatedBy: updatedByPtr,
	})
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
