package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type WorkforceHandler struct {
	svc service.WorkforceService
}

func NewWorkforceHandler(svc service.WorkforceService) *WorkforceHandler {
	return &WorkforceHandler{svc: svc}
}

func (h *WorkforceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	department := c.Query("department")
	status := c.Query("status")

	items, total, err := h.svc.List(page, limit, department, status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load workforce")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *WorkforceHandler) Create(c *gin.Context) {
	var req models.CreateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Create(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Member created", member)
}

// Apply is a public endpoint for workers to submit interest; always starts as pending.
func (h *WorkforceHandler) Apply(c *gin.Context) {
	var req models.CreateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Create(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Application submitted", member)
}

func (h *WorkforceHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Update(id, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Member updated", member)
}

// Approve can only be called by super admins; moves member to serving and sends welcome email.
func (h *WorkforceHandler) Approve(c *gin.Context) {
	id := c.Param("id")
	member, err := h.svc.Approve(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce request approved", member)
}

func (h *WorkforceHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load workforce stats")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce stats retrieved", stats)
}
