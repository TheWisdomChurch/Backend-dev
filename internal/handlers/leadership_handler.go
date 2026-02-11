package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type LeadershipHandler struct {
	svc service.LeadershipService
}

func NewLeadershipHandler(svc service.LeadershipService) *LeadershipHandler {
	return &LeadershipHandler{svc: svc}
}

// Public: list approved leadership members
func (h *LeadershipHandler) ListPublic(c *gin.Context) {
	role := c.Query("role")
	items, err := h.svc.ListApproved(role)
	if err != nil {
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

// Admin: list leadership applications/members
func (h *LeadershipHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	role := c.Query("role")
	status := c.Query("status")

	items, total, err := h.svc.List(page, limit, role, status)
	if err != nil {
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
	id := c.Param("id")

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
	id := c.Param("id")

	member, err := h.svc.Approve(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Leadership request approved", member)
}

// Admin: delete leadership member
func (h *LeadershipHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.Delete(id); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Leadership member deleted", nil)
}
