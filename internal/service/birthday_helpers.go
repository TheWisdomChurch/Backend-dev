package service

import (
	"errors"
	"strconv"
	"strings"
)

type dayMonthParseMode int

const (
	dayMonthNoYear dayMonthParseMode = iota
	dayMonthOptionalYear
	dayMonthRequireYear
)

// parseBirthday accepts either month/day pointers or a DD/MM string.
func parseBirthday(monthPtr, dayPtr *int, birthdayStr *string) (*int, *int, error) {
	return parseDayMonth(monthPtr, dayPtr, birthdayStr, "birthday", dayMonthNoYear)
}

// parseAnniversary accepts month/day pointers or a DD/MM/YYYY string.
func parseAnniversary(monthPtr, dayPtr *int, anniversaryStr *string) (*int, *int, error) {
	return parseDayMonth(monthPtr, dayPtr, anniversaryStr, "anniversary", dayMonthRequireYear)
}

func parseDayMonth(monthPtr, dayPtr *int, rawPtr *string, field string, mode dayMonthParseMode) (*int, *int, error) {
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
		switch mode {
		case dayMonthNoYear:
			if len(parts) != 2 {
				return nil, nil, errors.New(field + " must be in DD/MM format")
			}
		case dayMonthRequireYear:
			if len(parts) != 3 {
				return nil, nil, errors.New(field + " must be in DD/MM/YYYY format")
			}
		case dayMonthOptionalYear:
			if len(parts) != 2 && len(parts) != 3 {
				return nil, nil, errors.New(field + " must be in DD/MM or DD/MM/YYYY format")
			}
		default:
			if len(parts) != 2 {
				return nil, nil, errors.New(field + " must be in DD/MM format")
			}
		}
		day, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, nil, errors.New(field + " day must be numeric")
		}
		month, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, nil, errors.New(field + " month must be numeric")
		}
		if len(parts) == 3 {
			year, err := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err != nil {
				return nil, nil, errors.New(field + " year must be numeric")
			}
			if year < 1 {
				return nil, nil, errors.New(field + " year must be valid")
			}
		}
		return normalize(&month, &day)
	}

	return normalize(monthPtr, dayPtr)
}
