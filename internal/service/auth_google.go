package service

import (
	"errors"
	"strings"
	"time"
)

func (s *authServiceImpl) CompleteGoogleLogin(googleSubject, emailAddr, firstName, lastName string, meta LoginMetadata) (*LoginResult, error) {
	subject := strings.TrimSpace(googleSubject)
	emailAddr = normalizeEmail(emailAddr)
	if subject == "" || emailAddr == "" {
		return nil, errors.New("invalid Google account details")
	}

	user, err := s.userRepo.FindByFederatedAccount("google", subject)
	if err != nil {
		return nil, errors.New("failed to resolve Google account")
	}

	if user == nil {
		user, err = s.userRepo.FindByEmail(emailAddr)
		if err != nil || user == nil {
			return nil, errors.New("no approved account exists for this Google email")
		}

		if user.FederatedProvider != nil && strings.TrimSpace(*user.FederatedProvider) != "" &&
			(!strings.EqualFold(strings.TrimSpace(*user.FederatedProvider), "google") ||
				(user.FederatedSubject != nil && strings.TrimSpace(*user.FederatedSubject) != "" && strings.TrimSpace(*user.FederatedSubject) != subject)) {
			return nil, errors.New("this account is already linked to another external identity")
		}

		provider := "google"
		linkedAt := time.Now().UTC()
		user.FederatedProvider = &provider
		user.FederatedSubject = &subject
		user.FederatedLinkedAt = &linkedAt
		if strings.TrimSpace(user.FirstName) == "" && strings.TrimSpace(firstName) != "" {
			user.FirstName = strings.TrimSpace(firstName)
		}
		if strings.TrimSpace(user.LastName) == "" && strings.TrimSpace(lastName) != "" {
			user.LastName = strings.TrimSpace(lastName)
		}
		if err := s.userRepo.Update(user); err != nil {
			return nil, errors.New("failed to link Google account")
		}
	}

	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if isAdminRole(user.Role) && !user.AdminApproved {
		return nil, ErrAdminPending
	}

	return s.completePrimaryLogin(user, meta, "google")
}
