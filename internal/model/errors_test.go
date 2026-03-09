package model

import (
	"encoding/json"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	e := &AppError{Code: ErrNotFound, Message: "issue #42 not found"}
	want := "NOT_FOUND: issue #42 not found"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAppError_ExitCode(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{ErrNotFound, 4},
		{ErrAuthMissingScope, 3},
		{ErrConfigInvalid, 2},
		{ErrRateLimited, 1},
		{ErrNoTags, 1},
		{ErrNotGitRepo, 1},
		{"UNKNOWN_CODE", 1},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			e := &AppError{Code: tt.code, Message: "test"}
			if got := e.ExitCode(); got != tt.want {
				t.Errorf("ExitCode() for %s = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestErrorEnvelope_JSON(t *testing.T) {
	env := &ErrorEnvelope{
		Error: &AppError{
			Code:    ErrNotFound,
			Message: "issue #42 not found in owner/repo",
			Details: map[string]interface{}{
				"resource": "issue",
				"number":   float64(42),
			},
		},
	}

	b, err := env.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var got ErrorEnvelope
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Error.Code != ErrNotFound {
		t.Errorf("code = %q, want %q", got.Error.Code, ErrNotFound)
	}
	if got.Error.Message != env.Error.Message {
		t.Errorf("message = %q, want %q", got.Error.Message, env.Error.Message)
	}
	if got.Error.Details["resource"] != "issue" {
		t.Errorf("details.resource = %v, want %q", got.Error.Details["resource"], "issue")
	}
}

func TestErrorEnvelope_JSON_OmitsEmptyDetails(t *testing.T) {
	env := &ErrorEnvelope{
		Error: &AppError{
			Code:    ErrNoTags,
			Message: "no tags found",
		},
	}

	b, err := env.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	// Verify "details" key is absent when nil.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	var errObj map[string]json.RawMessage
	if err := json.Unmarshal(raw["error"], &errObj); err != nil {
		t.Fatalf("Unmarshal error obj: %v", err)
	}
	if _, exists := errObj["details"]; exists {
		t.Error("expected details to be omitted from JSON when nil")
	}
}
