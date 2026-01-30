package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type EmailTemplateHandler struct {
	svc service.EmailTemplateService
}

func NewEmailTemplateHandler(svc service.EmailTemplateService) *EmailTemplateHandler {
	return &EmailTemplateHandler{svc: svc}
}

func (h *EmailTemplateHandler) SendTemplate(c *gin.Context) {
	var req models.SendTemplateEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	resp, err := h.svc.SendTemplateEmail(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Template email sent", resp)
}
