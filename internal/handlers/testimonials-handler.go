package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type TestimonialHandler struct {
	svc      service.TestimonialService
	userRepo repository.UserRepository
}

func NewTestimonialHandler(svc service.TestimonialService, userRepo repository.UserRepository) *TestimonialHandler {
	return &TestimonialHandler{svc: svc, userRepo: userRepo}
}

func (h *TestimonialHandler) currentUser(c *gin.Context) *models.User {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		return nil
	}
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		return nil
	}
	return user
}

func (h *TestimonialHandler) CreateTestimonial(c *gin.Context) {
	var req models.CreateTestimonialRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	testimonial, err := h.svc.CreateTestimonial(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create testimonial")
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Testimonial created successfully", testimonial)
}

func (h *TestimonialHandler) GetAllTestimonials(c *gin.Context) {
	testimonials, err := h.svc.GetAllTestimonials(true)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch testimonials")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Testimonials fetched successfully", testimonials)
}

func (h *TestimonialHandler) GetPaginatedTestimonials(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	testimonials, total, err := h.svc.GetPaginatedTestimonials(page, limit, true)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch testimonials")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Testimonials fetched successfully", gin.H{
		"data":       testimonials,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *TestimonialHandler) GetTestimonialByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
		return
	}

	testimonial, err := h.svc.GetTestimonialByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Testimonial not found")
		return
	}
	if !testimonial.IsApproved {
		utils.ErrorResponse(c, http.StatusNotFound, "Testimonial not found")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Testimonial fetched successfully", testimonial)
}

func (h *TestimonialHandler) UpdateTestimonial(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
		return
	}

	var req models.UpdateTestimonialRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	testimonial, err := h.svc.UpdateTestimonial(id, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update testimonial")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Testimonial updated successfully", testimonial)
}

func (h *TestimonialHandler) DeleteTestimonial(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
		return
	}

	if err := h.svc.DeleteTestimonial(id, h.currentUser(c)); err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Testimonial not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete testimonial")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Testimonial deleted successfully", nil)
}

func (h *TestimonialHandler) ApproveTestimonial(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
		return
	}

	testimonial, err := h.svc.ApproveTestimonial(id, h.currentUser(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to approve testimonial")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Testimonial approved successfully", testimonial)
}
