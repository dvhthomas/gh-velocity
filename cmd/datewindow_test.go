package cmd

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestParseDateWindow(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	t.Run("absolute dates", func(t *testing.T) {
		since, until, err := parseDateWindow("2026-03-01", "2026-03-10", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if since.Day() != 1 {
			t.Errorf("since day = %d, want 1", since.Day())
		}
		if until.Day() != 10 {
			t.Errorf("until day = %d, want 10", until.Day())
		}
	})

	t.Run("relative since with empty until defaults to now", func(t *testing.T) {
		since, until, err := parseDateWindow("30d", "", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if until != now {
			t.Errorf("until = %v, want %v (now)", until, now)
		}
		// 30 days before now
		expected := now.AddDate(0, 0, -30)
		if since != expected {
			t.Errorf("since = %v, want %v", since, expected)
		}
	})

	t.Run("invalid since returns AppError", func(t *testing.T) {
		_, _, err := parseDateWindow("not-a-date", "", now)
		if err == nil {
			t.Fatal("expected error for invalid since")
		}
		appErr, ok := err.(*model.AppError)
		if !ok {
			t.Fatalf("expected *model.AppError, got %T", err)
		}
		if appErr.Code != model.ErrConfigInvalid {
			t.Errorf("error code = %q, want %q", appErr.Code, model.ErrConfigInvalid)
		}
	})

	t.Run("invalid until returns AppError", func(t *testing.T) {
		_, _, err := parseDateWindow("30d", "not-a-date", now)
		if err == nil {
			t.Fatal("expected error for invalid until")
		}
		appErr, ok := err.(*model.AppError)
		if !ok {
			t.Fatalf("expected *model.AppError, got %T", err)
		}
		if appErr.Code != model.ErrConfigInvalid {
			t.Errorf("error code = %q, want %q", appErr.Code, model.ErrConfigInvalid)
		}
	})

	t.Run("since after until returns AppError", func(t *testing.T) {
		_, _, err := parseDateWindow("2026-03-20", "2026-03-10", now)
		if err == nil {
			t.Fatal("expected error for since > until")
		}
		appErr, ok := err.(*model.AppError)
		if !ok {
			t.Fatalf("expected *model.AppError, got %T", err)
		}
		if appErr.Code != model.ErrConfigInvalid {
			t.Errorf("error code = %q, want %q", appErr.Code, model.ErrConfigInvalid)
		}
	})
}
