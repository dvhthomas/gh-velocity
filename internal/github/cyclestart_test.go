package github

import (
	"context"
	"testing"
	"time"
)

func TestMatchProjectStatus_InBacklog(t *testing.T) {
	ts := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	items := []projectItemNode{
		{
			Project: struct {
				ID string `json:"id"`
			}{ID: "PVT_proj1"},
			FieldValues: struct {
				Nodes []fieldValueNode `json:"nodes"`
			}{
				Nodes: []fieldValueNode{
					{
						Typename:  "ProjectV2ItemFieldSingleSelectValue",
						Name:      "Backlog",
						UpdatedAt: &ts,
						Field: &struct {
							ID string `json:"id"`
						}{ID: "PVTSSF_status"},
					},
				},
			},
		},
	}

	ps := matchProjectStatus(items, "PVT_proj1", "PVTSSF_status", "Backlog")
	if !ps.InBacklog {
		t.Fatal("expected InBacklog=true for backlog status")
	}
	if ps.CycleStart != nil {
		t.Fatal("expected nil CycleStart when in backlog")
	}
}

func TestMatchProjectStatus_ActiveStatus(t *testing.T) {
	ts := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	items := []projectItemNode{
		{
			Project: struct {
				ID string `json:"id"`
			}{ID: "PVT_proj1"},
			FieldValues: struct {
				Nodes []fieldValueNode `json:"nodes"`
			}{
				Nodes: []fieldValueNode{
					{
						Typename:  "ProjectV2ItemFieldSingleSelectValue",
						Name:      "In Progress",
						UpdatedAt: &ts,
						Field: &struct {
							ID string `json:"id"`
						}{ID: "PVTSSF_status"},
					},
				},
			},
		},
	}

	ps := matchProjectStatus(items, "PVT_proj1", "PVTSSF_status", "Backlog")
	if ps.InBacklog {
		t.Fatal("expected InBacklog=false for active status")
	}
	if ps.CycleStart == nil {
		t.Fatal("expected CycleStart for active status")
	}
	if !ps.CycleStart.Time.Equal(ts) {
		t.Fatalf("got CycleStart.Time=%v, want %v", ps.CycleStart.Time, ts)
	}
	if ps.CycleStart.Signal != "status-change" {
		t.Fatalf("got signal=%q, want status-change", ps.CycleStart.Signal)
	}
}

func TestMatchProjectStatus_WrongProject(t *testing.T) {
	ts := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	items := []projectItemNode{
		{
			Project: struct {
				ID string `json:"id"`
			}{ID: "PVT_other"},
			FieldValues: struct {
				Nodes []fieldValueNode `json:"nodes"`
			}{
				Nodes: []fieldValueNode{
					{
						Typename:  "ProjectV2ItemFieldSingleSelectValue",
						Name:      "In Progress",
						UpdatedAt: &ts,
						Field: &struct {
							ID string `json:"id"`
						}{ID: "PVTSSF_status"},
					},
				},
			},
		},
	}

	ps := matchProjectStatus(items, "PVT_proj1", "PVTSSF_status", "Backlog")
	if ps.CycleStart != nil {
		t.Fatal("expected nil CycleStart for wrong project ID")
	}
}

func TestMatchProjectStatus_WrongField(t *testing.T) {
	ts := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	items := []projectItemNode{
		{
			Project: struct {
				ID string `json:"id"`
			}{ID: "PVT_proj1"},
			FieldValues: struct {
				Nodes []fieldValueNode `json:"nodes"`
			}{
				Nodes: []fieldValueNode{
					{
						Typename:  "ProjectV2ItemFieldSingleSelectValue",
						Name:      "In Progress",
						UpdatedAt: &ts,
						Field: &struct {
							ID string `json:"id"`
						}{ID: "PVTSSF_other"},
					},
				},
			},
		},
	}

	ps := matchProjectStatus(items, "PVT_proj1", "PVTSSF_status", "Backlog")
	if ps.CycleStart != nil {
		t.Fatal("expected nil CycleStart for wrong field ID")
	}
}

func TestMatchProjectStatus_NoItems(t *testing.T) {
	ps := matchProjectStatus(nil, "PVT_proj1", "PVTSSF_status", "Backlog")
	if ps.CycleStart != nil || ps.InBacklog {
		t.Fatal("expected empty ProjectStatus for nil items")
	}
}

func TestMatchProjectStatus_SkipsNonSingleSelect(t *testing.T) {
	ts := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	items := []projectItemNode{
		{
			Project: struct {
				ID string `json:"id"`
			}{ID: "PVT_proj1"},
			FieldValues: struct {
				Nodes []fieldValueNode `json:"nodes"`
			}{
				Nodes: []fieldValueNode{
					{
						Typename:  "ProjectV2ItemFieldTextValue",
						Name:      "In Progress",
						UpdatedAt: &ts,
						Field: &struct {
							ID string `json:"id"`
						}{ID: "PVTSSF_status"},
					},
				},
			},
		},
	}

	ps := matchProjectStatus(items, "PVT_proj1", "PVTSSF_status", "Backlog")
	if ps.CycleStart != nil {
		t.Fatal("expected nil CycleStart for non-SingleSelect field")
	}
}

func TestProjectStatusCacheKey_DifferentInputs(t *testing.T) {
	k1 := projectStatusCacheKey("owner", "repo", 1, "proj1", "field1", "Backlog")
	k2 := projectStatusCacheKey("owner", "repo", 2, "proj1", "field1", "Backlog")
	if k1 == k2 {
		t.Fatal("different issue numbers should produce different keys")
	}

	k3 := projectStatusCacheKey("owner", "repo", 1, "proj1", "field1", "Backlog")
	if k1 != k3 {
		t.Fatal("same inputs should produce same key")
	}

	k4 := projectStatusCacheKey("owner", "repo", 1, "proj1", "field1", "Todo")
	if k1 == k4 {
		t.Fatal("different backlog status should produce different keys")
	}
}

func TestBatchGetProjectStatuses_CacheWarming(t *testing.T) {
	// Create a client with a cache but no real GraphQL client.
	// We test that after batch pre-fetch, GetProjectStatus hits cache.
	cache := NewQueryCache(time.Minute)
	c := &Client{
		owner: "testowner",
		repo:  "testrepo",
		cache: cache,
	}

	// Manually warm cache as if BatchGetProjectStatuses had succeeded.
	ps := &ProjectStatus{
		CycleStart: &CycleStart{
			Time:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
			Signal: "status-change",
			Detail: "Backlog → In Progress",
		},
	}
	key := projectStatusCacheKey("testowner", "testrepo", 42, "PVT_proj1", "PVTSSF_status", "Backlog")
	cache.Set(key, ps)

	// Now GetProjectStatus should hit cache without making any API call.
	// (Since we have no real gql client, an API call would panic.)
	got, err := c.GetProjectStatus(context.TODO(), 42, "PVT_proj1", "PVTSSF_status", "Backlog")
	if err != nil {
		t.Fatal(err)
	}
	if got.CycleStart == nil {
		t.Fatal("expected CycleStart from cache")
	}
	if got.CycleStart.Detail != "Backlog → In Progress" {
		t.Fatalf("got detail=%q, want 'Backlog → In Progress'", got.CycleStart.Detail)
	}
}

func TestBatchGetProjectStatuses_EmptyBatch(t *testing.T) {
	cache := NewQueryCache(time.Minute)
	c := &Client{
		owner: "testowner",
		repo:  "testrepo",
		cache: cache,
	}

	// Empty batch should not panic.
	c.BatchGetProjectStatuses(context.TODO(), nil, "PVT_proj1", "PVTSSF_status", "Backlog")
	c.BatchGetProjectStatuses(context.TODO(), []int{}, "PVT_proj1", "PVTSSF_status", "Backlog")
}
