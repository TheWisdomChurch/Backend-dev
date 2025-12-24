package service

import (
    "fmt"

    "github.com/google/uuid"
    "wisdomHouse-backend/internal/models"        
    "wisdomHouse-backend/internal/repository"   
)

type TestimonialService interface {
    CreateTestimonial(req *models.CreateTestimonialRequest) (*models.Testimonial, error)
    GetAllTestimonials(approved bool) ([]models.Testimonial, error)
    GetTestimonialByID(id uuid.UUID) (*models.Testimonial, error)
    UpdateTestimonial(id uuid.UUID, req *models.UpdateTestimonialRequest) (*models.Testimonial, error)
    DeleteTestimonial(id uuid.UUID) error
    GetPaginatedTestimonials(page, limit int, approved bool) ([]models.Testimonial, int64, error)
    ApproveTestimonial(id uuid.UUID) (*models.Testimonial, error)
}

type testimonialService struct {
    repo repository.TestimonialRepository
}

func NewTestimonialService(repo repository.TestimonialRepository) TestimonialService {
    return &testimonialService{repo: repo}
}

func (s *testimonialService) CreateTestimonial(req *models.CreateTestimonialRequest) (*models.Testimonial, error) {
    testimonial := &models.Testimonial{
        FirstName:   req.FirstName,
        LastName:    req.LastName,
        FullName:    fmt.Sprintf("%s %s", req.FirstName, req.LastName),
        ImageURL:    req.ImageURL,
        Testimony:   req.Testimony,
        IsAnonymous: req.IsAnonymous,
        IsApproved:  false, // New testimonials need approval
    }
    
    if err := s.repo.Create(testimonial); err != nil {
        return nil, err
    }
    
    return testimonial, nil
}

func (s *testimonialService) GetAllTestimonials(approved bool) ([]models.Testimonial, error) {
    return s.repo.GetAll(approved)
}

func (s *testimonialService) GetTestimonialByID(id uuid.UUID) (*models.Testimonial, error) {
    return s.repo.GetByID(id)
}

func (s *testimonialService) UpdateTestimonial(id uuid.UUID, req *models.UpdateTestimonialRequest) (*models.Testimonial, error) {
    testimonial, err := s.repo.GetByID(id)
    if err != nil {
        return nil, err
    }
    
    // Update fields if provided
    if req.FirstName != nil {
        testimonial.FirstName = *req.FirstName
    }
    if req.LastName != nil {
        testimonial.LastName = *req.LastName
    }
    if req.FirstName != nil || req.LastName != nil {
        testimonial.FullName = fmt.Sprintf("%s %s", testimonial.FirstName, testimonial.LastName)
    }
    if req.ImageURL != nil {
        testimonial.ImageURL = *req.ImageURL
    }
    if req.Testimony != nil {
        testimonial.Testimony = *req.Testimony
    }
    if req.IsAnonymous != nil {
        testimonial.IsAnonymous = *req.IsAnonymous
    }
    if req.IsApproved != nil {
        testimonial.IsApproved = *req.IsApproved
    }
    
    if err := s.repo.Update(testimonial); err != nil {
        return nil, err
    }
    
    return testimonial, nil
}

func (s *testimonialService) DeleteTestimonial(id uuid.UUID) error {
    return s.repo.Delete(id)
}

func (s *testimonialService) GetPaginatedTestimonials(page, limit int, approved bool) ([]models.Testimonial, int64, error) {
    if page < 1 {
        page = 1
    }
    if limit < 1 || limit > 100 {
        limit = 10
    }
    
    return s.repo.GetPaginated(page, limit, approved)
}

func (s *testimonialService) ApproveTestimonial(id uuid.UUID) (*models.Testimonial, error) {
    testimonial, err := s.repo.GetByID(id)
    if err != nil {
        return nil, err
    }
    
    testimonial.IsApproved = true
    
    if err := s.repo.Update(testimonial); err != nil {
        return nil, err
    }
    
    return testimonial, nil
}