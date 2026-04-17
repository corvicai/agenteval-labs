// Package validation provides lightweight, reusable input validators for
// handler-level checks across the API. Validators trim whitespace before
// every check and return descriptive errors that are safe to surface to
// callers over the WebSocket / REST layer.
package validation

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

const (
	MaxNameLen      = 100
	MaxEmailLen     = 254
	MaxWorkspaceLen = 100
	MaxOrgLen       = 100
	MaxPasswordLen  = 128
	MinPasswordLen  = 8
)

// ValidateName checks that a generic name field is non-empty and within the
// given maximum character limit. field is the label used in the error message
// (e.g. "name", "workspace name").
func ValidateName(name, field string, maxLen int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len([]rune(name)) > maxLen {
		return fmt.Errorf("%s must be at most %d characters", field, maxLen)
	}
	return nil
}

// ValidateEmail checks that an email address is non-empty, syntactically
// valid (RFC 5322 via net/mail), and within the maximum allowed length.
func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New("email is required")
	}
	if len(email) > MaxEmailLen {
		return fmt.Errorf("email must be at most %d characters", MaxEmailLen)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return errors.New("email is not valid")
	}
	return nil
}

// ValidatePassword checks that a password meets minimum and maximum length
// requirements. It does NOT hash; callers are expected to hash afterwards.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLen {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	}
	if len(password) > MaxPasswordLen {
		return fmt.Errorf("password must be at most %d characters", MaxPasswordLen)
	}
	return nil
}

// ValidateUserName is a convenience wrapper for user display names.
func ValidateUserName(name string) error {
	return ValidateName(name, "name", MaxNameLen)
}

// ValidateOrgName is a convenience wrapper for organisation names.
func ValidateOrgName(name string) error {
	return ValidateName(name, "organization name", MaxOrgLen)
}

// ValidateWorkspaceName is a convenience wrapper for workspace names.
func ValidateWorkspaceName(name string) error {
	return ValidateName(name, "workspace name", MaxWorkspaceLen)
}
