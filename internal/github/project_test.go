package github

import "testing"

func TestParseProjectURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantOwner  string
		wantNumber int
		wantIsOrg  bool
		wantErr    bool
	}{
		{
			name:       "user project",
			url:        "https://github.com/users/dvhthomas/projects/1",
			wantOwner:  "dvhthomas",
			wantNumber: 1,
			wantIsOrg:  false,
		},
		{
			name:       "org project",
			url:        "https://github.com/orgs/myorg/projects/42",
			wantOwner:  "myorg",
			wantNumber: 42,
			wantIsOrg:  true,
		},
		{
			name:    "wrong host",
			url:     "https://gitlab.com/users/test/projects/1",
			wantErr: true,
		},
		{
			name:    "wrong path structure",
			url:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "non-numeric project number",
			url:     "https://github.com/users/test/projects/abc",
			wantErr: true,
		},
		{
			name:    "wrong segment",
			url:     "https://github.com/teams/test/projects/1",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:       "trailing slash",
			url:        "https://github.com/users/test/projects/5/",
			wantOwner:  "test",
			wantNumber: 5,
			wantIsOrg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, number, isOrg, err := ParseProjectURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if number != tt.wantNumber {
				t.Errorf("number = %d, want %d", number, tt.wantNumber)
			}
			if isOrg != tt.wantIsOrg {
				t.Errorf("isOrg = %v, want %v", isOrg, tt.wantIsOrg)
			}
		})
	}
}
