package posting

import (
	"testing"
	"time"
)

func TestRenderTitle(t *testing.T) {
	// Fixed time for deterministic tests: 2026-03-20 14:30:00 UTC
	fixed := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)
	vars := TitleVars{
		Date:    fixed,
		Repo:    "owner/repo",
		Owner:   "owner",
		Command: "report",
	}

	tests := []struct {
		name     string
		template string
		vars     TitleVars
		want     string
	}{
		{
			name:     "date default format",
			template: "Velocity Update {{date}}",
			vars:     vars,
			want:     "Velocity Update 2026-03-20",
		},
		{
			name:     "date custom format",
			template: "Weekly {{date:Jan 2}}",
			vars:     vars,
			want:     "Weekly Mar 20",
		},
		{
			name:     "date year only",
			template: "Report {{date:2006}}",
			vars:     vars,
			want:     "Report 2026",
		},
		{
			name:     "date with time",
			template: "Run {{date:2006-01-02 15:04}}",
			vars:     vars,
			want:     "Run 2026-03-20 14:30",
		},
		{
			name:     "repo variable",
			template: "Metrics for {{repo}}",
			vars:     vars,
			want:     "Metrics for owner/repo",
		},
		{
			name:     "owner variable",
			template: "{{owner}} velocity",
			vars:     vars,
			want:     "owner velocity",
		},
		{
			name:     "command variable",
			template: "gh-velocity {{command}}",
			vars:     vars,
			want:     "gh-velocity report",
		},
		{
			name:     "multiple variables",
			template: "{{command}}: {{repo}} ({{date}})",
			vars:     vars,
			want:     "report: owner/repo (2026-03-20)",
		},
		{
			name:     "no placeholders",
			template: "Static Title",
			vars:     vars,
			want:     "Static Title",
		},
		{
			name:     "unknown variable preserved",
			template: "Title {{unknown}}",
			vars:     vars,
			want:     "Title {{unknown}}",
		},
		{
			name:     "whitespace in placeholder trimmed",
			template: "Update {{ date }}",
			vars:     vars,
			want:     "Update 2026-03-20",
		},
		{
			name:     "unknown qualified variable preserved",
			template: "Title {{foo:bar}}",
			vars:     vars,
			want:     "Title {{foo:bar}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderTitle(tt.template, tt.vars)
			if got != tt.want {
				t.Errorf("RenderTitle(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestValidateTitleTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid with date",
			template: "Velocity Update {{date}}",
		},
		{
			name:     "valid with date format",
			template: "Weekly {{date:Jan 2}}",
		},
		{
			name:     "valid with repo",
			template: "{{command}}: {{repo}}",
		},
		{
			name:     "valid no placeholders",
			template: "Static Title",
		},
		{
			name:     "empty",
			template: "",
			wantErr:  true,
			errMsg:   "non-empty",
		},
		{
			name:     "unclosed delimiter",
			template: "Title {{date",
			wantErr:  true,
			errMsg:   "unclosed",
		},
		{
			name:     "nested delimiters",
			template: "Title {{{{date}}}}",
			wantErr:  true,
			errMsg:   "nested",
		},
		{
			name:     "unmatched close",
			template: "Title date}}",
			wantErr:  true,
			errMsg:   "unmatched",
		},
		{
			name:     "unknown variable",
			template: "Title {{today}}",
			wantErr:  true,
			errMsg:   "unknown variable",
		},
		{
			name:     "unknown qualified variable",
			template: "Title {{foo:bar}}",
			wantErr:  true,
			errMsg:   "unknown variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTitleTemplate(tt.template)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// contains is defined in marker_test.go
