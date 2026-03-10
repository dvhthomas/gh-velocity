package dateutil

import (
	"testing"
	"time"
)

// Fixed "now" for deterministic tests: 2026-03-10T14:30:00Z
var testNow = time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		// YYYY-MM-DD
		{
			name:  "date only",
			input: "2026-01-15",
			want:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "date only leap year valid",
			input: "2024-02-29",
			want:  time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "date only leap year invalid",
			input:   "2026-02-29",
			wantErr: true,
		},
		{
			name:    "invalid month",
			input:   "2026-13-01",
			wantErr: true,
		},
		{
			name:    "invalid day",
			input:   "2026-01-32",
			wantErr: true,
		},

		// RFC3339
		{
			name:  "RFC3339 UTC",
			input: "2026-01-15T14:30:00Z",
			want:  time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339 with offset converted to UTC",
			input: "2026-01-15T14:30:00+05:00",
			want:  time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
		},
		{
			name:    "RFC3339 malformed",
			input:   "2026-01-15T25:00:00Z",
			wantErr: true,
		},

		// Relative
		{
			name:  "30d relative",
			input: "30d",
			want:  testNow.AddDate(0, 0, -30),
		},
		{
			name:  "7d relative",
			input: "7d",
			want:  testNow.AddDate(0, 0, -7),
		},
		{
			name:  "0d is start of today UTC",
			input: "0d",
			want:  time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "1d relative",
			input: "1d",
			want:  testNow.AddDate(0, 0, -1),
		},

		// Invalid
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "garbage",
			input:   "foo",
			wantErr: true,
		},
		{
			name:    "negative relative",
			input:   "-5d",
			wantErr: true,
		},
		{
			name:    "float relative",
			input:   "3.5d",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input, testNow)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("Parse(%q) location = %v, want UTC", tt.input, got.Location())
			}
		})
	}
}

func TestValidateWindow(t *testing.T) {
	tests := []struct {
		name    string
		since   time.Time
		until   time.Time
		wantErr bool
	}{
		{
			name:  "valid window",
			since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "since in the future",
			since:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			until:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			wantErr: true,
		},
		{
			name:    "inverted window",
			since:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			until:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			wantErr: true,
		},
		{
			name:    "zero-width window (equal dates)",
			since:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			until:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			wantErr: true,
		},
		{
			name:  "since is now (not future)",
			since: testNow,
			until: testNow.Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWindow(tt.since, tt.until, testNow)
			if tt.wantErr {
				if err == nil {
					t.Error("ValidateWindow() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateWindow() error: %v", err)
			}
		})
	}
}
