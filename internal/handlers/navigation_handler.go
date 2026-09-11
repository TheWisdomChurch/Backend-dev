package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/apperror"
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
		if ae, ok := apperror.As(err); ok {
			utils.ErrorResponse(c, ae.HTTPStatus, ae.Message)
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to preview route")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Route preview computed", preview)
}
