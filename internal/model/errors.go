package model

import (
	"encoding/json"
	"fmt"
)

// Error code constants for machine-parseable error classification.
const (
	ErrNotFound        = "NOT_FOUND"
	ErrAuthMissingScope = "AUTH_MISSING_SCOPE"
	ErrConfigInvalid   = "CONFIG_INVALID"
	ErrRateLimited     = "RATE_LIMITED"
	ErrNoTags          = "NO_TAGS"
	ErrNotGitRepo      = "NOT_GIT_REPO"
)

// exitCodes maps error codes to process exit codes.
var exitCodes = map[string]int{
	ErrNotFound:        4,
	ErrAuthMissingScope: 3,
	ErrConfigInvalid:   2,
	ErrRateLimited:     1,
	ErrNoTags:          1,
	ErrNotGitRepo:      1,
}

// AppError is a structured error that carries a machine-readable code,
// a human-readable message, and optional details for agent consumption.
type AppError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ExitCode returns the process exit code for this error.
// Unknown codes default to 1.
func (e *AppError) ExitCode() int {
	if code, ok := exitCodes[e.Code]; ok {
		return code
	}
	return 1
}

// ErrorEnvelope is the top-level JSON wrapper emitted when --format json
// encounters an error.
type ErrorEnvelope struct {
	Error *AppError `json:"error"`
}

// JSON marshals the envelope to a JSON byte slice.
func (env *ErrorEnvelope) JSON() ([]byte, error) {
	return json.Marshal(env)
}
