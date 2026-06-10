package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

// ImprovedVerifyLoginOTP - Enhanced version with better error handling
// This replaces the existing VerifyLoginOTP function
func (h *AuthHandler) ImprovedVerifyLoginOTP(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required"`
		Code       string `json:"code" binding:"required"`
		Purpose    string `json:"purpose"`
		Method     string `json:"method"`
		RememberMe bool   `json:"rememberMe"`
	}

	// Bind request
	if err := c.BindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Validate and normalize email
	req.Email = strings.TrimSpace(req.Email)
	req.Email = strings.ToLower(req.Email)

	if !isValidEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid email format",
			"errors": gin.H{
				"email": "Please provide a valid email address",
			},
		})
		return
	}

	// Validate OTP code
	req.Code = strings.TrimSpace(req.Code)
	if !isValidOTPCode(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid OTP code format",
			"errors": gin.H{
				"code": "OTP code must be exactly 6 digits (0-9)",
			},
		})
		return
	}

	// Check rate limiting
	if h.isOTPRateLimited(c, req.Email) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"status":  "error",
			"message": "Too many failed verification attempts",
			"meta": gin.H{
				"retry_after": 900, // 15 minutes in seconds
				"help":        "Please wait 15 minutes before trying again",
			},
		})
		return
	}

	// Attempt verification
	user, authMethod, err := h.service.VerifyLoginMFA(
		req.Email,
		req.Code,
		req.Purpose,
		req.Method,
		h.loginMetadata(c),
	)

	if err != nil {
		// Record failed attempt
		h.recordOTPAttempt(c, req.Email, false)

		// Return specific error messages
		if errors.Is(err, service.ErrInvalidOTP) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Incorrect OTP code",
				"errors": gin.H{
					"code": "The OTP code you entered is incorrect. Please check and try again.",
				},
				"data": gin.H{
					"attempts_remaining": 5 - h.getOTPAttempts(c, req.Email),
				},
			})
			return
		}

		if errors.Is(err, service.ErrOTPExpired) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "OTP code has expired",
				"errors": gin.H{
					"code": "Your OTP code has expired. Please request a new code.",
				},
			})
			return
		}

		if errors.Is(err, service.ErrUserNotFound) {
			// Don't reveal if user exists (security)
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Verification failed",
				"errors": gin.H{
					"email": "Unable to verify with the provided email and code",
				},
			})
			return
		}

		// Generic error for unknown issues
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Verification failed",
			"errors": gin.H{
				"general": "An error occurred during verification. Please try again.",
			},
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid verification response",
		})
		return
	}

	// Check if admin account is approved
	if isAdminAccessRole(user.Role) && !user.AdminApproved {
		h.clearAuthCookie(c)
		writeAuthBlockedResponse(c, http.StatusForbidden,
			"Your admin account is awaiting super-admin approval.",
			"admin_approval_pending",
			"/pending-approval",
		)
		return
	}

	// Issue authenticated session
	if err := h.issueAuthenticatedSession(c, user, req.RememberMe, authMethod); err != nil {
		if errors.Is(err, service.ErrAdminPending) {
			writeAuthBlockedResponse(c, http.StatusForbidden,
				"Your admin account is awaiting super-admin approval.",
				"admin_approval_pending",
				"/pending-approval",
			)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to generate authentication token",
		})
		return
	}

	// Clear OTP attempts on success
	h.clearOTPAttempts(c, req.Email)

	// Derive access status
	accessStatus, accessCode, nextStep := deriveAccessStatus(user, authMethod)
	responseData := authUserPayload(user)
	responseData["auth_method"] = strings.TrimSpace(authMethod)

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login verified successfully",
		"data": gin.H{
			"user": responseData,
		},
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// Validation helpers

func isValidEmail(email string) bool {
	if email == "" {
		return false
	}
	// Simple email validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func isValidOTPCode(code string) bool {
	if code == "" {
		return false
	}
	if len(code) != 6 {
		return false
	}
	// Must be all digits
	codeRegex := regexp.MustCompile(`^\d{6}$`)
	return codeRegex.MatchString(code)
}

// Rate limiting helpers

const (
	maxOTPAttempts    = 5
	otpAttemptWindow  = 15 * time.Minute
	otpRateLimitKey   = "otp_attempts:%s"
)

func (h *AuthHandler) isOTPRateLimited(c *gin.Context, email string) bool {
	attempts := h.getOTPAttempts(c, email)
	return attempts >= maxOTPAttempts
}

func (h *AuthHandler) recordOTPAttempt(c *gin.Context, email string, success bool) {
	// In production, use Redis or database
	// This is a simplified version
	key := fmt.Sprintf(otpRateLimitKey, email)
	_ = key
	// TODO: Implement with cache layer
}

func (h *AuthHandler) getOTPAttempts(c *gin.Context, email string) int {
	// In production, use Redis or database
	// This is a simplified version
	_ = email
	return 0 // TODO: Implement with cache layer
}

func (h *AuthHandler) clearOTPAttempts(c *gin.Context, email string) {
	// In production, use Redis or database
	_ = email
	// TODO: Implement with cache layer
}

// Helper to check if user has admin role
func isAdminAccessRole(role string) bool {
	adminRoles := map[string]bool{
		"admin":      true,
		"superadmin": true,
		"superadmin_pending": true,
	}
	return adminRoles[strings.ToLower(role)]
}
