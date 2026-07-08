package handlers

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type GivingPaymentsHandler struct {
	svc service.GivingService
}

func NewGivingPaymentsHandler(svc service.GivingService) *GivingPaymentsHandler {
	return &GivingPaymentsHandler{svc: svc}
}

// ListCategories godoc
// @Summary List giving categories
// @Tags Giving
// @Produce json
// @Success 200 {object} utils.APIResponse
// @Router /giving/categories [get]
func (h *GivingPaymentsHandler) ListCategories(c *gin.Context) {
	cats, err := h.svc.ListCategories(c.Request.Context())
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, cats)
}

// Initiate godoc
// @Summary Start a giving/payment transaction
// @Tags Giving
// @Accept json
// @Produce json
// @Param provider path string true "Payment provider (paystack|stripe)"
// @Param body body service.InitiateGivingRequest true "Giving request"
// @Success 200 {object} utils.APIResponse
// @Router /giving/initiate/{provider} [post]
func (h *GivingPaymentsHandler) Initiate(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	var req service.InitiateGivingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	resp, err := h.svc.Initiate(c.Request.Context(), provider, req)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, resp)
}

// Verify godoc
// @Summary Verify a payment reference and update transaction status
// @Tags Giving
// @Produce json
// @Param provider   path string true "Payment provider"
// @Param reference  path string true "Payment reference"
// @Success 200 {object} utils.APIResponse
// @Router /giving/verify/{provider}/{reference} [get]
func (h *GivingPaymentsHandler) Verify(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	reference := strings.TrimSpace(c.Param("reference"))
	tx, err := h.svc.VerifyAndRecord(c.Request.Context(), provider, reference)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, tx)
}

// Webhook godoc
// @Summary Receive a payment provider webhook
// @Tags Giving
// @Accept json
// @Produce json
// @Param provider path string true "Payment provider"
// @Success 200
// @Router /giving/webhook/{provider} [post]
func (h *GivingPaymentsHandler) Webhook(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))

	// Read raw body BEFORE any binding — required for webhook signature verification.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "cannot read body"})
		return
	}

	signature := c.GetHeader("X-Paystack-Signature")
	if provider == "stripe" {
		signature = c.GetHeader("Stripe-Signature")
	}

	if err := h.svc.HandleWebhook(c.Request.Context(), provider, signature, rawBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// List godoc
// @Summary List giving transactions (admin)
// @Tags Giving
// @Produce json
// @Param category_id query string false "Filter by category"
// @Param member_id   query string false "Filter by member"
// @Param campus_id   query string false "Filter by campus"
// @Param status      query string false "Filter by status (pending|success|failed|reversed)"
// @Param from        query string false "From date (RFC3339)"
// @Param to          query string false "To date (RFC3339)"
// @Param page        query int    false "Page number (default 1)"
// @Param limit       query int    false "Page size (default 20, max 100)"
// @Success 200 {object} utils.APIResponse
// @Router /admin/giving [get]
func (h *GivingPaymentsHandler) List(c *gin.Context) {
	filter := repository.GivingFilter{}
	if v := c.Query("category_id"); v != "" {
		filter.CategoryID = &v
	}
	if v := c.Query("member_id"); v != "" {
		filter.MemberID = &v
	}
	if v := c.Query("campus_id"); v != "" {
		filter.CampusID = &v
	}
	if v := c.Query("status"); v != "" {
		filter.Status = &v
	}
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = &t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = &t
		}
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}

	txs, total, err := h.svc.List(c.Request.Context(), filter, limit, (page-1)*limit)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKPage(c, txs, utils.BuildPageMeta(page, limit, total))
}

// MonthlySummary godoc
// @Summary Giving monthly summary (admin)
// @Tags Giving
// @Produce json
// @Param year      query int    false "Year (e.g. 2025)"
// @Param month     query int    false "Month 1-12"
// @Param campus_id query string false "Campus filter"
// @Success 200 {object} utils.APIResponse
// @Router /admin/giving/summary [get]
func (h *GivingPaymentsHandler) MonthlySummary(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	month, _ := strconv.Atoi(c.Query("month"))
	var campusID *string
	if v := c.Query("campus_id"); v != "" {
		campusID = &v
	}
	rows, err := h.svc.MonthlySummary(c.Request.Context(), year, month, campusID)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, rows)
}
