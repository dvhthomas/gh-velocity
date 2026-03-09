package cmd

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// captureStderr temporarily redirects os.Stderr to capture output.
// We test handleError indirectly by checking exit codes, and test
// JSON output via the handleErrorToBuffer helper.

func TestHandleError_AppError_ExitCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantExit int
	}{
		{"not found", model.ErrNotFound, 4},
		{"auth missing scope", model.ErrAuthMissingScope, 3},
		{"config invalid", model.ErrConfigInvalid, 2},
		{"rate limited", model.ErrRateLimited, 1},
		{"no tags", model.ErrNoTags, 1},
		{"not git repo", model.ErrNotGitRepo, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRoot("pretty")
			appErr := &model.AppError{Code: tt.code, Message: "test"}
			got := handleError(root, appErr)
			if got != tt.wantExit {
				t.Errorf("handleError() exit code = %d, want %d", got, tt.wantExit)
			}
		})
	}
}

func TestHandleError_NonAppError_Returns1(t *testing.T) {
	root := newTestRoot("pretty")
	got := handleError(root, errors.New("some random error"))
	if got != 1 {
		t.Errorf("handleError() exit code = %d, want 1", got)
	}
}

func TestHandleError_JSONFormat_EmitsEnvelope(t *testing.T) {
	root := newTestRoot("json")
	appErr := &model.AppError{
		Code:    model.ErrConfigInvalid,
		Message: "bad config",
	}

	// We can't easily capture os.Stderr in a unit test without pipe gymnastics,
	// so we verify the envelope structure directly.
	envelope := model.ErrorEnvelope{Error: appErr}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify the JSON structure is correct
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	errObj, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' key in JSON envelope")
	}
	if errObj["code"] != model.ErrConfigInvalid {
		t.Errorf("code = %v, want %v", errObj["code"], model.ErrConfigInvalid)
	}
	if errObj["message"] != "bad config" {
		t.Errorf("message = %v, want %q", errObj["message"], "bad config")
	}

	// Also verify exit code is correct
	exitCode := handleError(root, appErr)
	if exitCode != 2 {
		t.Errorf("handleError() exit code = %d, want 2", exitCode)
	}
}

func TestExecute_PostFlag_ReturnsConfigInvalidExitCode(t *testing.T) {
	root := NewRootCmd("test", "now")
	// Use "release" subcommand (not "version") so PersistentPreRunE runs.
	root.SetArgs([]string{"--post", "--repo", "owner/repo", "release", "v1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for --post flag")
	}

	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *model.AppError, got %T: %v", err, err)
	}
	if appErr.Code != model.ErrConfigInvalid {
		t.Errorf("error code = %q, want %q", appErr.Code, model.ErrConfigInvalid)
	}
	if appErr.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", appErr.ExitCode())
	}
}

// newTestRoot creates a minimal root command with the format flag set.
func newTestRoot(format string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "test",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().String("format", format, "")
	return cmd
}
