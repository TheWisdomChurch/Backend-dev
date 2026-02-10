package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type EmailTemplateRegistryHandler struct {
	svc service.EmailTemplateRegistryService
}

func NewEmailTemplateRegistryHandler(svc service.EmailTemplateRegistryService) *EmailTemplateRegistryHandler {
	return &EmailTemplateRegistryHandler{svc: svc}
}

func (h *EmailTemplateRegistryHandler) Create(c *gin.Context) {
	var req models.CreateEmailTemplateRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	item, err := h.svc.Create(&req, nil)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Email template created", item)
}

func (h *EmailTemplateRegistryHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateEmailTemplateRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	item, err := h.svc.Update(id, &req)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Email template not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Email template updated", item)
}

func (h *EmailTemplateRegistryHandler) Get(c *gin.Context) {
	id := c.Param("id")
	item, err := h.svc.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Email template not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email template loaded", item)
}

func (h *EmailTemplateRegistryHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)
	ownerType := strings.TrimSpace(c.Query("ownerType"))
	ownerID := strings.TrimSpace(c.Query("ownerId"))
	templateKey := strings.TrimSpace(c.Query("templateKey"))
	status := strings.TrimSpace(c.Query("status"))

	items, total, err := h.svc.List(page, limit, ownerType, ownerID, templateKey, status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load templates")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Email templates loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *EmailTemplateRegistryHandler) Activate(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "id is required")
		return
	}

	item, err := h.svc.Activate(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Email template not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Email template activated", item)
}
