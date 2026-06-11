package authutil

import (
	"fmt"
	"strings"
	"unicode"
)

// PasswordPolicy enforces strength requirements on plaintext passwords.
type PasswordPolicy struct {
	MinLength      int
	RequireUpper   bool
	RequireLower   bool
	RequireDigit   bool
	RequireSpecial bool
}

// DefaultPasswordPolicy returns the recommended production policy.
func DefaultPasswordPolicy() *PasswordPolicy {
	return &PasswordPolicy{
		MinLength:      12,
		RequireUpper:   true,
		RequireLower:   true,
		RequireDigit:   true,
		RequireSpecial: true,
	}
}

// PolicyFromConfig builds a policy from config values.
func PolicyFromConfig(minLength int) *PasswordPolicy {
	if minLength <= 0 {
		minLength = 12
	}
	p := DefaultPasswordPolicy()
	p.MinLength = minLength
	return p
}

// Validate returns a human-readable error when the password violates the policy,
// or nil when it passes. Never logs or stores the plaintext password.
func (p *PasswordPolicy) Validate(password string) error {
	if p == nil {
		return nil
	}

	if len(password) < p.MinLength {
		return fmt.Errorf("password must be at least %d characters long", p.MinLength)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	var missing []string
	if p.RequireUpper && !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if p.RequireLower && !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if p.RequireDigit && !hasDigit {
		missing = append(missing, "digit")
	}
	if p.RequireSpecial && !hasSpecial {
		missing = append(missing, "special character")
	}

	if len(missing) > 0 {
		return fmt.Errorf("password must contain at least one %s", strings.Join(missing, ", "))
	}

	return nil
}
