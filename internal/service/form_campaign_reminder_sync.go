package service

import (
	"log"
	"strings"

	"wisdomHouse-backend/internal/models"
)

func (s *formService) syncCampaignReminder(
	existing *models.FormCalendarReminder,
	form *models.Form,
	event *models.Event,
	submission *models.FormSubmission,
	emailAddr string,
	recipientName string,
	registrationCode string,
) {
	if s.reminderRepo == nil || existing == nil || form == nil || event == nil || submission == nil {
		return
	}
	if form.Slug == nil || strings.TrimSpace(*form.Slug) == "" {
		return
	}

	startAt, endAt, err := parseEventSchedule(strings.TrimSpace(event.Date), strings.TrimSpace(event.Time))
	if err != nil {
		return
	}

	if strings.TrimSpace(existing.CalendarToken) == "" {
		token, tokenErr := generateSecureToken(24)
		if tokenErr != nil {
			return
		}
		existing.CalendarToken = token
	}

	var recipientPtr *string
	if strings.TrimSpace(recipientName) != "" {
		name := strings.TrimSpace(recipientName)
		recipientPtr = &name
	}
	var codePtr *string
	if strings.TrimSpace(registrationCode) != "" {
		code := strings.TrimSpace(registrationCode)
		codePtr = &code
	}
	var locationPtr *string
	location := campaignEventLocation(event)
	if location != "" {
		loc := location
		locationPtr = &loc
	}

	endCopy := endAt
	existing.FormID = form.ID
	existing.SubmissionID = submission.ID
	existing.Slug = strings.TrimSpace(*form.Slug)
	existing.Email = strings.TrimSpace(emailAddr)
	existing.RecipientName = recipientPtr
	existing.RegistrationCode = codePtr
	existing.EventTitle = campaignEventTitle(form, event)
	existing.EventLocation = locationPtr
	existing.EventDate = campaignEventDate(event)
	existing.EventTime = campaignEventTime(event)
	existing.EventStartsAt = startAt.UTC()
	existing.EventEndsAt = &endCopy

	if err := s.reminderRepo.Update(existing); err != nil {
		log.Printf("⚠️ failed to sync form campaign reminder (formID=%s, submissionID=%s): %v", form.ID, submission.ID, err)
	}
}

func campaignEventTitle(form *models.Form, event *models.Event) string {
	if event != nil {
		if title := strings.TrimSpace(event.Title); title != "" {
			return title
		}
	}
	if form != nil {
		if title := strings.TrimSpace(form.Title); title != "" {
			return title
		}
	}
	return "Church Event"
}

func campaignEventDate(event *models.Event) string {
	if event == nil {
		return ""
	}
	return strings.TrimSpace(event.Date)
}

func campaignEventTime(event *models.Event) string {
	if event == nil {
		return ""
	}
	return strings.TrimSpace(event.Time)
}

func campaignEventLocation(event *models.Event) string {
	if event == nil {
		return ""
	}
	return strings.TrimSpace(event.Location)
}
