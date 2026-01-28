package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
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
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid payload")
		return
	}

	resp, err := h.svc.SendOTP(&req)
	if err != nil {
		// Log for debugging SMTP/config issues
		log.Printf("OTP send failed for %s (purpose=%s): %v", req.Email, req.Purpose, err)
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP sent", resp)
}

func (h *OTPHandler) VerifyOTP(c *gin.Context) {
	var req models.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid payload")
		return
	}

	resp, err := h.svc.VerifyOTP(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP verified", resp)
}
