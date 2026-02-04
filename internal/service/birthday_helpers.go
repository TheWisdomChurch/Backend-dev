package service

import (
	"errors"
	"strconv"
	"strings"
)

// normalizeBirthday validates optional month/day (1-12, 1-31). Returns nil pointers when absent.
func normalizeBirthday(monthPtr, dayPtr *int) (*int, *int, error) {
	if monthPtr == nil && dayPtr == nil {
		return nil, nil, nil
	}
	if monthPtr == nil || dayPtr == nil {
		return nil, nil, errors.New("birthdayMonth and birthdayDay must both be provided")
	}
	m := *monthPtr
	d := *dayPtr
	if m < 1 || m > 12 {
		return nil, nil, errors.New("birthdayMonth must be 1-12")
	}
	if d < 1 || d > 31 {
		return nil, nil, errors.New("birthdayDay must be 1-31")
	}
	return &m, &d, nil
}

// parseBirthday accepts either month/day pointers or a DD/MM string.
func parseBirthday(monthPtr, dayPtr *int, birthdayStr *string) (*int, *int, error) {
	if birthdayStr != nil {
		raw := strings.TrimSpace(*birthdayStr)
		if raw == "" {
			return nil, nil, nil
		}
		parts := strings.Split(raw, "/")
		if len(parts) != 2 {
			return nil, nil, errors.New("birthday must be in DD/MM format")
		}
		day, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, nil, errors.New("birthday day must be numeric")
		}
		month, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, nil, errors.New("birthday month must be numeric")
		}
		return normalizeBirthday(&month, &day)
	}

	return normalizeBirthday(monthPtr, dayPtr)
}
