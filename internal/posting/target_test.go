package posting

import "testing"

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOwner string
		wantRepo  string
		wantCat   string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "simple",
			input:     "myorg/myrepo/General",
			wantOwner: "myorg",
			wantRepo:  "myrepo",
			wantCat:   "General",
		},
		{
			name:      "category with spaces",
			input:     "myorg/myrepo/Show and Tell",
			wantOwner: "myorg",
			wantRepo:  "myrepo",
			wantCat:   "Show and Tell",
		},
		{
			name:      "quoted category with slash",
			input:     `myorg/myrepo/"My / Category"`,
			wantOwner: "myorg",
			wantRepo:  "myrepo",
			wantCat:   "My / Category",
		},
		{
			name:      "quoted category no slash",
			input:     `myorg/myrepo/"General"`,
			wantOwner: "myorg",
			wantRepo:  "myrepo",
			wantCat:   "General",
		},
		{
			name:    "missing category",
			input:   "myorg/myrepo",
			wantErr: true,
			errMsg:  "owner/repo/category",
		},
		{
			name:    "just a name",
			input:   "General",
			wantErr: true,
			errMsg:  "owner/repo/category",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
			errMsg:  "owner/repo/category",
		},
		{
			name:    "empty owner",
			input:   "/myrepo/General",
			wantErr: true,
			errMsg:  "owner",
		},
		{
			name:    "empty repo",
			input:   "myorg//General",
			wantErr: true,
			errMsg:  "repo",
		},
		{
			name:    "empty category",
			input:   "myorg/myrepo/",
			wantErr: true,
			errMsg:  "category",
		},
		{
			name:    "unclosed quote",
			input:   `myorg/myrepo/"Unclosed`,
			wantErr: true,
			errMsg:  "unclosed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt, err := ParseTarget(tt.input)
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
			if dt.Owner != tt.wantOwner {
				t.Errorf("Owner = %q, want %q", dt.Owner, tt.wantOwner)
			}
			if dt.Repo != tt.wantRepo {
				t.Errorf("Repo = %q, want %q", dt.Repo, tt.wantRepo)
			}
			if dt.Category != tt.wantCat {
				t.Errorf("Category = %q, want %q", dt.Category, tt.wantCat)
			}
		})
	}
}
