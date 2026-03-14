//go:build integration
// +build integration

package github

import (
	"context"
	"testing"
	"time"
)

// TestListProjectItemsWithFields_Integration exercises pagination and
// SingleSelect field parsing against real project boards.
//
// Run with: go test -tags=integration -run TestListProjectItemsWithFields_Integration ./internal/github/ -v
//
// Requires `gh auth status` to have valid credentials.
func TestListProjectItemsWithFields_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("microsoft eBPF org project (large board)", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Any client works — DiscoverProjectByOwner bypasses the repo.
		client, err := NewClient("microsoft", "ebpf-for-windows", 0)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}

		// Resolve org-level project #2098 via the project URL.
		info, err := client.ResolveProject(ctx, "https://github.com/orgs/microsoft/projects/2098", "")
		if err != nil {
			t.Fatalf("ResolveProject: %v", err)
		}
		t.Logf("Resolved project: %s", info.ProjectID)

		// Fetch items with Status SingleSelect field.
		items, err := client.ListProjectItemsWithFields(ctx, info.ProjectID, "", "", []string{"Status"})
		if err != nil {
			t.Fatalf("ListProjectItemsWithFields: %v", err)
		}

		t.Logf("Fetched %d items", len(items))
		if len(items) < 100 {
			t.Errorf("expected at least 100 items from Microsoft eBPF board, got %d", len(items))
		}

		// Verify some items have Status field values populated.
		var withStatus int
		statusCounts := map[string]int{}
		for _, item := range items {
			if v, ok := item.Fields["Status"]; ok && v != "" {
				withStatus++
				statusCounts[v]++
			}
		}

		t.Logf("Items with Status: %d/%d", withStatus, len(items))
		t.Logf("Status distribution: %v", statusCounts)

		if withStatus == 0 {
			t.Error("expected at least some items to have Status field populated")
		}
	})

	t.Run("dvhthomas user project (with Size field)", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		client, err := NewClient("dvhthomas", "gh-velocity", 0)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}

		info, err := client.ResolveProject(ctx, "https://github.com/users/dvhthomas/projects/1", "")
		if err != nil {
			t.Fatalf("ResolveProject: %v", err)
		}
		t.Logf("Resolved project: %s", info.ProjectID)

		items, err := client.ListProjectItemsWithFields(ctx, info.ProjectID, "", "", []string{"Status", "Size"})
		if err != nil {
			t.Fatalf("ListProjectItemsWithFields: %v", err)
		}

		t.Logf("Fetched %d items", len(items))
		if len(items) == 0 {
			t.Fatal("expected at least 1 item")
		}

		var withSize int
		sizeCounts := map[string]int{}
		for _, item := range items {
			if v, ok := item.Fields["Size"]; ok && v != "" {
				withSize++
				sizeCounts[v]++
			}
		}
		t.Logf("Items with Size: %d, distribution: %v", withSize, sizeCounts)
	})
}
