package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type SermonHandler struct {
	svc service.SermonService
}

func NewSermonHandler(svc service.SermonService) *SermonHandler {
	return &SermonHandler{svc: svc}
}

func (h *SermonHandler) List(c *gin.Context) {
	limit := parseIntClamp(c.DefaultQuery("limit", "50"), 1, 50)
	items, err := h.svc.List(service.SermonFilter{
		Search:   c.Query("search"),
		Series:   c.Query("series"),
		Preacher: c.Query("preacher"),
		Sort:     c.DefaultQuery("sort", "newest"),
		Limit:    limit,
	})
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "Failed to load sermons")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Sermons loaded", items)
}
