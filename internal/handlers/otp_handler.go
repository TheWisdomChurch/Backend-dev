package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type OTPHandler struct {
	svc service.OTPService
}

func NewOTPHandler(svc service.OTPService) *OTPHandler {
	return &OTPHandler{svc: svc}
}

func (h *OTPHandler) SendOTP(c *gin.Context) {
	var req models.SendOTPRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	resp, err := h.svc.SendOTP(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP sent", resp)
}

func (h *OTPHandler) VerifyOTP(c *gin.Context) {
	var req models.VerifyOTPRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	purpose := strings.TrimSpace(req.Purpose)
	// For password reset, verify without consuming the OTP.
	// The final reset step will consume it.
	var resp *models.VerifyOTPResponse
	var err error
	if strings.HasPrefix(purpose, "password_reset") {
		resp, err = h.svc.VerifyOTPNoConsume(&req)
	} else {
		resp, err = h.svc.VerifyOTP(&req)
	}
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP verified", resp)
}
