package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type MinistryHandler struct {
	svc service.MinistryService
}

func NewMinistryHandler(svc service.MinistryService) *MinistryHandler {
	return &MinistryHandler{svc: svc}
}

func (h *MinistryHandler) Create(c *gin.Context) {
	var req models.CreateMinistryRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	m := models.Ministry{Name: strings.TrimSpace(req.Name)}
	if req.Description != nil {
		m.Description = strings.TrimSpace(*req.Description)
	}
	if req.Category != nil {
		m.Category = strings.TrimSpace(*req.Category)
	}
	m.CampusID = req.CampusID
	if err := h.svc.Create(c.Request.Context(), &m); err != nil {
		utils.Err(c, err)
		return
	}
	utils.Created(c, m)
}

func (h *MinistryHandler) Update(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var req models.UpdateMinistryRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Category != nil {
		updates["category"] = strings.TrimSpace(*req.Category)
	}
	if req.CampusID != nil {
		updates["campus_id"] = strings.TrimSpace(*req.CampusID)
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if len(updates) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "At least one ministry field is required")
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, updates); err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKMsg(c, "ministry updated", nil)
}

func (h *MinistryHandler) Structure(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "ministry id")
	if !ok {
		return
	}
	structure, err := h.svc.Structure(c.Request.Context(), id)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, structure)
}

func (h *MinistryHandler) AssignWorkforceMember(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "ministry id")
	if !ok {
		return
	}
	var req models.AssignMinistryWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.svc.AssignWorkforceMember(c.Request.Context(), id, req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.OKMsg(c, "workforce member assigned", nil)
}

func (h *MinistryHandler) UpdateWorkforceAssignment(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "ministry id")
	if !ok {
		return
	}
	workforceID, ok := parseUUIDParam(c, "workforce_id", "workforce member id")
	if !ok {
		return
	}
	var req models.AssignMinistryWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.svc.UpdateWorkforceAssignment(c.Request.Context(), id, workforceID, req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.OKMsg(c, "ministry assignment updated", nil)
}

func (h *MinistryHandler) RemoveWorkforceMember(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "ministry id")
	if !ok {
		return
	}
	workforceID, ok := parseUUIDParam(c, "workforce_id", "workforce member id")
	if !ok {
		return
	}
	if err := h.svc.RemoveWorkforceMember(c.Request.Context(), id, workforceID); err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKMsg(c, "workforce member removed", nil)
}

func (h *MinistryHandler) Get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	m, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, m)
}

func (h *MinistryHandler) List(c *gin.Context) {
	var campusID, category *string
	if v := c.Query("campus_id"); v != "" {
		campusID = &v
	}
	if v := c.Query("category"); v != "" {
		category = &v
	}
	activeOnly := c.Query("active") != "false"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	rows, total, err := h.svc.List(c.Request.Context(), campusID, category, activeOnly, limit, (page-1)*limit)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKPage(c, rows, utils.BuildPageMeta(page, limit, total))
}

func (h *MinistryHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKMsg(c, "ministry deleted", nil)
}

func (h *MinistryHandler) AddMember(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body struct {
		MemberID string `json:"member_id" binding:"required"`
		Role     string `json:"role,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.AddMember(c.Request.Context(), id, body.MemberID, body.Role); err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKMsg(c, "member added", nil)
}

func (h *MinistryHandler) RemoveMember(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	memberID := strings.TrimSpace(c.Param("member_id"))
	if err := h.svc.RemoveMember(c.Request.Context(), id, memberID); err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKMsg(c, "member removed", nil)
}

func (h *MinistryHandler) ListMembers(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	members, err := h.svc.ListMembers(c.Request.Context(), id)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, members)
}

func (h *MinistryHandler) MemberMinistries(c *gin.Context) {
	memberID := strings.TrimSpace(c.Param("member_id"))
	rows, err := h.svc.MemberMinistries(c.Request.Context(), memberID)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, rows)
}
