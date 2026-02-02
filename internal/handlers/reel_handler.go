package handlers

import (
	"net/http"
	"strconv"

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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	items, total, err := h.repo.List(offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load reels")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *ReelHandler) Create(c *gin.Context) {
	// For now JSON. If you want multipart upload, we can add it next.
	var req models.Reel
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.repo.Create(&req); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create reel")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": req})
}

func (h *ReelHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete reel")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Reel deleted"})
}
