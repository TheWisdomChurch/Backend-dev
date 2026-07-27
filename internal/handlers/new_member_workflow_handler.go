package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type NewMemberWorkflowHandler struct {
	svc service.NewMemberWorkflowService
}

func NewNewMemberWorkflowHandler(svc service.NewMemberWorkflowService) *NewMemberWorkflowHandler {
	return &NewMemberWorkflowHandler{svc: svc}
}

func (h *NewMemberWorkflowHandler) List(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 25, 200)
	if !ok {
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), page, limit, c.Query("stage"), c.Query("ownerId"), c.Query("escalationStatus"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "New-member workflows loaded", gin.H{"data": items, "total": total, "page": page, "limit": limit, "totalPages": (total + int64(limit) - 1) / int64(limit)})
}

func (h *NewMemberWorkflowHandler) Get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workflow id")
	if !ok {
		return
	}
	row, contacts, history, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "New-member workflow not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "New-member workflow retrieved", gin.H{"workflow": row, "contacts": contacts, "history": history})
}

func (h *NewMemberWorkflowHandler) Update(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workflow id")
	if !ok {
		return
	}
	var req models.UpdateNewMemberWorkflowRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, _ := middleware.GetUserIDFromContext(c)
	row, err := h.svc.Update(c.Request.Context(), id, strings.TrimSpace(actor), req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "New-member workflow updated", row)
}

func (h *NewMemberWorkflowHandler) AddContact(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workflow id")
	if !ok {
		return
	}
	var req models.CreateNewMemberContactRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Authentication required")
		return
	}
	row, err := h.svc.AddContact(c.Request.Context(), id, actor, req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Contact history recorded", row)
}

func (h *NewMemberWorkflowHandler) Reconcile(c *gin.Context) {
	created, err := h.svc.Reconcile(c.Request.Context())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Workflow reconciliation failed")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Workflow reconciliation completed", gin.H{"created": created})
}
