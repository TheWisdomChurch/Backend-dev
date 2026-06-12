package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type MinistryHandler struct {
	svc service.MinistryService
}

func NewMinistryHandler(svc service.MinistryService) *MinistryHandler {
	return &MinistryHandler{svc: svc}
}

func (h *MinistryHandler) Create(c *gin.Context) {
	var m models.Ministry
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.Create(c.Request.Context(), &m); err != nil {
		utils.Err(c, err)
		return
	}
	utils.Created(c, m)
}

func (h *MinistryHandler) Update(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, body); err != nil {
		utils.Err(c, err)
		return
	}
	utils.OKMsg(c, "ministry updated", nil)
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
