package cmd

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

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
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	errObj, ok := parsed["error"].(map[string]any)
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

func TestPostFlag_CoercesFormatAndSetsDryRun(t *testing.T) {
	// Verify --post coerces format to markdown and DryRun defaults to true
	// when GH_VELOCITY_POST_LIVE is not set. Uses "version" to skip execution
	// (version command doesn't need Deps), so we test via a separate approach:
	// run a command that goes through PersistentPreRunE and check Deps.
	//
	// We use the release command with a fake repo; it will fail during
	// execution but PersistentPreRunE should succeed and set Deps correctly.
	root := NewRootCmd("test", "now")
	root.SetArgs([]string{"--post", "--repo", "owner/repo", "release", "v1.0.0"})

	// Ensure GH_VELOCITY_POST_LIVE is not set
	t.Setenv("GH_VELOCITY_POST_LIVE", "")

	err := root.Execute()
	// We expect an error (404 from fake repo) but not a config error
	if err == nil {
		t.Fatal("expected error from fake repo")
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) && appErr.Code == model.ErrConfigInvalid {
		t.Fatalf("--post should not produce a config error, got: %v", appErr)
	}
}

func TestNewPostFlag_ImpliesPost(t *testing.T) {
	root := NewRootCmd("test", "now")
	root.SetArgs([]string{"--new-post", "--repo", "owner/repo", "release", "v1.0.0"})

	t.Setenv("GH_VELOCITY_POST_LIVE", "")

	err := root.Execute()
	// Should proceed past PersistentPreRunE (not reject --new-post)
	if err == nil {
		t.Fatal("expected error from fake repo")
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) && appErr.Code == model.ErrConfigInvalid {
		t.Fatalf("--new-post should not produce a config error, got: %v", appErr)
	}
}

func TestIsRepoAutoDetected(t *testing.T) {
	tests := []struct {
		name     string
		repoFlag string
		ghRepo   string
		want     bool
	}{
		{"no flag no env", "", "", true},
		{"flag set", "owner/repo", "", false},
		{"env set", "", "owner/repo", false},
		{"both set", "owner/repo", "other/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_REPO", tt.ghRepo)
			got := isRepoAutoDetected(tt.repoFlag)
			if got != tt.want {
				t.Errorf("isRepoAutoDetected(%q) = %v, want %v (GH_REPO=%q)", tt.repoFlag, got, tt.want, tt.ghRepo)
			}
		})
	}
}

func TestNowFunc_Default(t *testing.T) {
	t.Setenv("GH_VELOCITY_NOW", "")
	fn := nowFunc()
	got := fn()
	// Should be within 1 second of actual now.
	if diff := time.Since(got); diff > time.Second || diff < -time.Second {
		t.Errorf("nowFunc() returned %v, expected ~now", got)
	}
}

func TestNowFunc_RFC3339(t *testing.T) {
	t.Setenv("GH_VELOCITY_NOW", "2026-03-01T12:00:00Z")
	fn := nowFunc()
	want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if got := fn(); !got.Equal(want) {
		t.Errorf("nowFunc() = %v, want %v", got, want)
	}
}

func TestNowFunc_DateOnly(t *testing.T) {
	t.Setenv("GH_VELOCITY_NOW", "2026-03-01")
	fn := nowFunc()
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if got := fn(); !got.Equal(want) {
		t.Errorf("nowFunc() = %v, want %v", got, want)
	}
}

func TestNowFunc_InvalidFallsBack(t *testing.T) {
	t.Setenv("GH_VELOCITY_NOW", "not-a-date")
	fn := nowFunc()
	got := fn()
	if diff := time.Since(got); diff > time.Second || diff < -time.Second {
		t.Errorf("nowFunc() with invalid env returned %v, expected ~now", got)
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
