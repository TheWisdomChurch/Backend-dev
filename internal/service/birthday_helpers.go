package service

import (
	"errors"
	"strconv"
	"strings"
)

// parseBirthday accepts either month/day pointers or a DD/MM[/YYYY] string.
func parseBirthday(monthPtr, dayPtr *int, birthdayStr *string) (*int, *int, error) {
	return parseDayMonth(monthPtr, dayPtr, birthdayStr, "birthday")
}

func parseAnniversary(monthPtr, dayPtr *int, anniversaryStr *string) (*int, *int, error) {
	return parseDayMonth(monthPtr, dayPtr, anniversaryStr, "anniversary")
}

func parseDayMonth(monthPtr, dayPtr *int, rawPtr *string, field string) (*int, *int, error) {
	if strings.TrimSpace(field) == "" {
		field = "date"
	}

	monthField := field + "Month"
	dayField := field + "Day"

	normalize := func(m, d *int) (*int, *int, error) {
		if m == nil && d == nil {
			return nil, nil, nil
		}
		if m == nil || d == nil {
			return nil, nil, errors.New(monthField + " and " + dayField + " must both be provided")
		}
		mm := *m
		dd := *d
		if mm < 1 || mm > 12 {
			return nil, nil, errors.New(monthField + " must be 1-12")
		}
		if dd < 1 || dd > 31 {
			return nil, nil, errors.New(dayField + " must be 1-31")
		}
		return &mm, &dd, nil
	}

	if rawPtr != nil {
		raw := strings.TrimSpace(*rawPtr)
		if raw == "" {
			return nil, nil, nil
		}
		parts := strings.Split(raw, "/")
		if len(parts) != 2 && len(parts) != 3 {
			return nil, nil, errors.New(field + " must be in DD/MM or DD/MM/YYYY format")
		}
		day, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, nil, errors.New(field + " day must be numeric")
		}
		month, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, nil, errors.New(field + " month must be numeric")
		}
		return normalize(&month, &day)
	}

	return normalize(monthPtr, dayPtr)
}
