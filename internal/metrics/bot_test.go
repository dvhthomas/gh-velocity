package metrics

import "testing"

func TestIsBotUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		login        string
		excludeUsers []string
		want         bool
	}{
		{name: "human user", login: "alice", want: false},
		{name: "bot suffix", login: "dependabot[bot]", want: true},
		{name: "bot suffix uppercase", login: "Dependabot[bot]", want: true},
		{name: "dash-bot suffix", login: "renovate-bot", want: true},
		{name: "dash-bot suffix mixed case", login: "Renovate-Bot", want: true},
		{name: "known bot dependabot", login: "dependabot", want: true},
		{name: "known bot renovate", login: "renovate", want: true},
		{name: "known bot copilot", login: "copilot", want: true},
		{name: "known bot github-actions", login: "github-actions", want: true},
		{name: "known bot case insensitive", login: "Dependabot", want: true},
		{name: "exclude_users match", login: "my-ci-user", excludeUsers: []string{"my-ci-user"}, want: true},
		{name: "exclude_users case insensitive", login: "My-CI-User", excludeUsers: []string{"my-ci-user"}, want: true},
		{name: "exclude_users no match", login: "alice", excludeUsers: []string{"my-ci-user"}, want: false},
		{name: "empty login", login: "", want: false},
		{name: "unassigned is not a bot", login: "unassigned", want: false},
		{name: "partial bot suffix not matched", login: "robot", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsBotUser(tt.login, tt.excludeUsers)
			if got != tt.want {
				t.Errorf("IsBotUser(%q, %v) = %v, want %v", tt.login, tt.excludeUsers, got, tt.want)
			}
		})
	}
}
