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

type AssetHandler struct {
	svc service.AssetService
}

func NewAssetHandler(svc service.AssetService) *AssetHandler {
	return &AssetHandler{svc: svc}
}

func (h *AssetHandler) Presign(c *gin.Context) {
	var req models.PresignAssetRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	resp, err := h.svc.PresignUpload(&req, nil)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Presign created", resp)
}

func (h *AssetHandler) Complete(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "asset id is required")
		return
	}

	asset, err := h.svc.CompleteUpload(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Asset not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Asset completed", asset)
}

func (h *AssetHandler) Get(c *gin.Context) {
	id := c.Param("id")
	asset, err := h.svc.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Asset not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Asset loaded", asset)
}

func (h *AssetHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)
	ownerType := strings.TrimSpace(c.Query("ownerType"))
	ownerID := strings.TrimSpace(c.Query("ownerId"))

	items, total, err := h.svc.List(page, limit, ownerType, ownerID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load assets")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Assets loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}
