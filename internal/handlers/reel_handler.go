package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type ReelHandler struct {
	repo *repository.ReelRepository
}

func NewReelHandler(repo *repository.ReelRepository) *ReelHandler {
	return &ReelHandler{repo: repo}
}

func (h *ReelHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)
	offset := (page - 1) * limit

	items, total, err := h.repo.List(offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load reels")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Reels loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *ReelHandler) Create(c *gin.Context) {
	var req models.Reel
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.repo.Create(&req); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create reel")
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Reel created", req)
}

func (h *ReelHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete reel")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Reel deleted", nil)
}
