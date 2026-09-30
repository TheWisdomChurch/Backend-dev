package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
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

func (h *AuthHandler) ReconfigureTOTP(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	payload, err := h.service.ReconfigureTOTP(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator reconfiguration initialized", payload)
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

func (h *AuthHandler) GenerateRecoveryCodes(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	codes, err := h.service.GenerateRecoveryCodes(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Recovery codes generated", models.RecoveryCodesResponse{Codes: codes})
}

func (h *AuthHandler) RequestEmergencyMFAReset(c *gin.Context) {
	var req models.EmergencyResetRequestPayload
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.RequestEmergencyMFAReset(req.Email); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to send reset code")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "If an account with authenticator MFA exists for this email, a verification code has been sent.", nil)
}

func (h *AuthHandler) ConfirmEmergencyMFAReset(c *gin.Context) {
	var req models.EmergencyResetConfirmPayload
	if !validation.BindJSON(c, &req) {
		return
	}

	user, err := h.service.ConfirmEmergencyMFAReset(req.Email, req.Code, req.Password)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.issueAuthenticatedSession(c, user, req.RememberMe, "email_otp"); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to establish session after reset")
		return
	}

	accessStatus, accessCode, nextStep := deriveAccessStatus(user, "email_otp")
	responseData := authUserPayload(user)
	responseData["auth_method"] = "email_otp"

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Authenticator device reset successfully. Please register your new device.",
		"data": gin.H{
			"user": responseData,
		},
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
		},
	})
}
