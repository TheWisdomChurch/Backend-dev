package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type CellGroupHandler struct {
	svc service.CellGroupService
}

func NewCellGroupHandler(svc service.CellGroupService) *CellGroupHandler {
	return &CellGroupHandler{svc: svc}
}

func (h *CellGroupHandler) Create(c *gin.Context) {
	var g models.CellGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.Create(c.Request.Context(), &g); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Cell group created", g)
}

func (h *CellGroupHandler) Update(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, body); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "group updated", nil)
}

func (h *CellGroupHandler) Get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	g, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Cell group not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load cell group")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Cell group retrieved", g)
}

func (h *CellGroupHandler) List(c *gin.Context) {
	var campusID *string
	if v := c.Query("campus_id"); v != "" {
		campusID = &v
	}
	activeOnly := c.Query("active") != "false"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	groups, total, err := h.svc.List(c.Request.Context(), campusID, activeOnly, limit, (page-1)*limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load cell groups")
		return
	}
	utils.PaginatedSuccessResponse(c, http.StatusOK, groups, page, limit, int(total))
}

func (h *CellGroupHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete cell group")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "group deleted", nil)
}

func (h *CellGroupHandler) AddMember(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	var body struct {
		MemberID string `json:"member_id" binding:"required"`
		Role     string `json:"role,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	member, err := h.svc.AddMember(c.Request.Context(), groupID, body.MemberID, body.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "member added", member)
}

func (h *CellGroupHandler) RemoveMember(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	memberID := strings.TrimSpace(c.Param("member_id"))
	if err := h.svc.RemoveMember(c.Request.Context(), groupID, memberID); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to remove member")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "member removed", nil)
}

func (h *CellGroupHandler) ListMembers(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	members, err := h.svc.ListMembers(c.Request.Context(), groupID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load cell group members")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Cell group members retrieved", members)
}

func (h *CellGroupHandler) MemberGroups(c *gin.Context) {
	memberID := strings.TrimSpace(c.Param("member_id"))
	groups, err := h.svc.MemberGroups(c.Request.Context(), memberID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load member's cell groups")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Member's cell groups retrieved", groups)
}

func (h *CellGroupHandler) CreateMeeting(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	var m models.CellGroupMeeting
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	m.GroupID = groupID
	if err := h.svc.CreateMeeting(c.Request.Context(), &m); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Cell group meeting created", m)
}

func (h *CellGroupHandler) ListMeetings(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	meetings, total, err := h.svc.ListMeetings(c.Request.Context(), groupID, limit, (page-1)*limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load cell group meetings")
		return
	}
	utils.PaginatedSuccessResponse(c, http.StatusOK, meetings, page, limit, int(total))
}
