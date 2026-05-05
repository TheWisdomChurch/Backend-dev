package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type dayMonthParseMode int

const (
	dayMonthNoYear dayMonthParseMode = iota
	dayMonthAllowYear
	dayMonthRequireYear
)

var (
	dayMonthInputRe = regexp.MustCompile(`^(\d{1,2})[\/\-.](\d{1,2})(?:[\/\-.](\d{2,4}))?$`)
	isoDateInputRe  = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
)

func parseBirthday(monthPtr, dayPtr *int, birthdayStr *string) (*int, *int, error) {
	return parseDayMonth(monthPtr, dayPtr, birthdayStr, "birthday", dayMonthAllowYear)
}

func parseAnniversary(monthPtr, dayPtr *int, anniversaryStr *string) (*int, *int, error) {
	return parseDayMonth(monthPtr, dayPtr, anniversaryStr, "anniversary", dayMonthAllowYear)
}

func parseDayMonth(monthPtr, dayPtr *int, rawPtr *string, field string, mode dayMonthParseMode) (*int, *int, error) {
	if monthPtr != nil || dayPtr != nil {
		if monthPtr == nil || dayPtr == nil {
			return nil, nil, fmt.Errorf("%s month and day must be provided together", field)
		}

		month := *monthPtr
		day := *dayPtr

		if err := validateMonthDay(field, month, day); err != nil {
			return nil, nil, err
		}

		return &month, &day, nil
	}

	if rawPtr == nil || strings.TrimSpace(*rawPtr) == "" {
		return nil, nil, nil
	}

	raw := strings.TrimSpace(*rawPtr)

	if match := isoDateInputRe.FindStringSubmatch(raw); match != nil {
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])

		if err := validateMonthDay(field, month, day); err != nil {
			return nil, nil, err
		}

		return &month, &day, nil
	}

	match := dayMonthInputRe.FindStringSubmatch(raw)
	if match == nil {
		return nil, nil, formatDayMonthError(field, mode)
	}

	day, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	year := strings.TrimSpace(match[3])

	if mode == dayMonthRequireYear && year == "" {
		return nil, nil, formatDayMonthError(field, mode)
	}

	if err := validateMonthDay(field, month, day); err != nil {
		return nil, nil, err
	}

	return &month, &day, nil
}

func formatDayMonthError(field string, mode dayMonthParseMode) error {
	switch mode {
	case dayMonthRequireYear:
		return fmt.Errorf("%s must be in DD/MM/YYYY format", field)
	case dayMonthAllowYear:
		return fmt.Errorf("%s must be in DD/MM or DD/MM/YYYY format", field)
	default:
		return fmt.Errorf("%s must be in DD/MM format", field)
	}
}

func validateMonthDay(field string, month int, day int) error {
	if month < 1 || month > 12 {
		return fmt.Errorf("%s month must be between 1 and 12", field)
	}

	maxDay := daysInMonthForBirthday(month)
	if day < 1 || day > maxDay {
		return fmt.Errorf("%s day must be between 1 and %d", field, maxDay)
	}

	return nil
}

func daysInMonthForBirthday(month int) int {
	switch month {
	case 2:
		return 29
	case 4, 6, 9, 11:
		return 30
	default:
		return 31
	}
}
