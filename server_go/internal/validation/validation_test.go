package validation_test

import (
	"strings"
	"testing"

	"benchmarking-platform/internal/validation"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"user@example.com", false},
		{"USER@EXAMPLE.COM", false},
		{"user+tag@sub.domain.io", false},
		{"", true},
		{"notanemail", true},
		{"@nodomain", true},
		{"noatsign.com", true},
		{strings.Repeat("a", 250) + "@x.com", true}, // > 254 chars
	}
	for _, tt := range tests {
		err := validation.ValidateEmail(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateEmail(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
	}
}

func TestValidateUserName(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"Alice", false},
		{"  Alice  ", false}, // trimmed
		{"", true},
		{"   ", true}, // blank after trim
		{strings.Repeat("a", 101), true},
		{strings.Repeat("a", 100), false},
	}
	for _, tt := range tests {
		err := validation.ValidateUserName(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateUserName(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"securepass", false},
		{"12345678", false},
		{"short", true},   // < 8 chars
		{"1234567", true}, // exactly 7 chars
		{strings.Repeat("a", 129), true},
		{strings.Repeat("a", 128), false},
	}
	for _, tt := range tests {
		err := validation.ValidatePassword(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidatePassword(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
	}
}

func TestValidateOrgName(t *testing.T) {
	if err := validation.ValidateOrgName("Acme Corp"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := validation.ValidateOrgName(""); err == nil {
		t.Error("expected error for empty org name")
	}
	if err := validation.ValidateOrgName(strings.Repeat("x", 101)); err == nil {
		t.Error("expected error for org name > 100 chars")
	}
}

func TestValidateWorkspaceName(t *testing.T) {
	if err := validation.ValidateWorkspaceName("main"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := validation.ValidateWorkspaceName(""); err == nil {
		t.Error("expected error for empty workspace name")
	}
}
