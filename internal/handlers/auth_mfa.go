package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

/* ============================================================================

   MFA Settings

============================================================================ */

func (h *AuthHandler) GetMFASecurityProfile(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	profile, err := h.service.GetSecurityProfile(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Security profile loaded", profile)
}

func (h *AuthHandler) BeginTOTPSetup(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	payload, err := h.service.BeginTOTPSetup(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator setup initialized", payload)
}

func (h *AuthHandler) EnableTOTP(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		Code string `json:"code" binding:"required,len=6"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.EnableTOTP(userID, req.Code)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator app enabled", profile)
}

func (h *AuthHandler) DisableTOTP(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		Code string `json:"code" binding:"required,len=6"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.DisableTOTP(userID, req.Code)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator app disabled", profile)
}

func (h *AuthHandler) SetPreferredMFAMethod(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		Method string `json:"method" binding:"required"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.SetPreferredMFAMethod(userID, req.Method)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "MFA preference updated", profile)
}
