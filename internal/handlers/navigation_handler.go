package handlers

import (
	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type NavigationHandler struct {
	service service.NavigationService
}

func NewNavigationHandler(navigationService service.NavigationService) *NavigationHandler {
	return &NavigationHandler{service: navigationService}
}

func (h *NavigationHandler) PreviewRoute(c *gin.Context) {
	var request struct {
		Origin *service.Coordinates `json:"origin" binding:"required"`
	}
	if !validation.BindJSON(c, &request) {
		return
	}

	preview, err := h.service.PreviewRoute(c.Request.Context(), *request.Origin)
	if err != nil {
		utils.Err(c, err)
		return
	}
	utils.OK(c, preview)
}
