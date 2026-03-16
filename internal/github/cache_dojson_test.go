package github

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoJSON_MemoryHit(t *testing.T) {
	c := NewQueryCache(time.Minute)
	var calls atomic.Int32

	fn := func() (any, error) {
		calls.Add(1)
		return "fresh", nil
	}
	unmarshal := func(raw json.RawMessage) (any, error) {
		var s string
		return s, json.Unmarshal(raw, &s)
	}

	// First call: miss.
	v, err := c.DoJSON("k1", "test", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	if v.(string) != "fresh" {
		t.Fatalf("got %v, want fresh", v)
	}
	if calls.Load() != 1 {
		t.Fatalf("fn called %d, want 1", calls.Load())
	}

	// Second call: memory hit (fn not called again).
	v, err = c.DoJSON("k1", "test", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	if v.(string) != "fresh" {
		t.Fatalf("got %v, want fresh", v)
	}
	if calls.Load() != 1 {
		t.Fatalf("fn called %d, want 1 (memory hit)", calls.Load())
	}
}

func TestDoJSON_DiskHit(t *testing.T) {
	dir := t.TempDir()
	disk := NewDiskCache(dir, 5*time.Minute)
	c := NewQueryCache(time.Minute)
	c.disk = disk

	// Pre-populate disk.
	raw, _ := json.Marshal("from-disk")
	disk.Set("k1", "test", raw)

	var calls atomic.Int32
	fn := func() (any, error) {
		calls.Add(1)
		return "from-api", nil
	}
	unmarshal := func(raw json.RawMessage) (any, error) {
		var s string
		return s, json.Unmarshal(raw, &s)
	}

	v, err := c.DoJSON("k1", "test", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	if v.(string) != "from-disk" {
		t.Fatalf("got %v, want from-disk", v)
	}
	if calls.Load() != 0 {
		t.Fatalf("fn called %d, want 0 (disk hit)", calls.Load())
	}

	// Should now be in memory too — second call doesn't touch disk.
	v2, err := c.DoJSON("k1", "test", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	if v2.(string) != "from-disk" {
		t.Fatalf("got %v, want from-disk (promoted)", v2)
	}
}

func TestDoJSON_NoDiskWhenNilDiskCache(t *testing.T) {
	c := NewQueryCache(time.Minute)
	// c.disk is nil (simulates --no-cache).

	var calls atomic.Int32
	fn := func() (any, error) {
		calls.Add(1)
		return "api-result", nil
	}
	unmarshal := func(raw json.RawMessage) (any, error) {
		var s string
		return s, json.Unmarshal(raw, &s)
	}

	v, err := c.DoJSON("k1", "test", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	if v.(string) != "api-result" {
		t.Fatalf("got %v, want api-result", v)
	}
	if calls.Load() != 1 {
		t.Fatalf("fn called %d, want 1", calls.Load())
	}
}

func TestDoJSON_DiskWrittenOnMiss(t *testing.T) {
	dir := t.TempDir()
	disk := NewDiskCache(dir, 5*time.Minute)
	c := NewQueryCache(time.Minute)
	c.disk = disk

	fn := func() (any, error) {
		return "computed", nil
	}
	unmarshal := func(raw json.RawMessage) (any, error) {
		var s string
		return s, json.Unmarshal(raw, &s)
	}

	_, err := c.DoJSON("k1", "test", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}

	// Verify disk now has the value.
	raw, _, ok := disk.Get("k1")
	if !ok {
		t.Fatal("expected disk cache to be populated after miss")
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "computed" {
		t.Fatalf("disk got %v, want computed", got)
	}
}

func TestDoJSON_DoesNotCacheErrors(t *testing.T) {
	c := NewQueryCache(time.Minute)
	var calls atomic.Int32

	fn := func() (any, error) {
		calls.Add(1)
		return nil, errors.New("api down")
	}
	unmarshal := func(raw json.RawMessage) (any, error) {
		return nil, nil
	}

	_, err := c.DoJSON("k1", "test", fn, unmarshal)
	if err == nil {
		t.Fatal("expected error")
	}

	_, err = c.DoJSON("k1", "test", fn, unmarshal)
	if err == nil {
		t.Fatal("expected error on retry")
	}
	if calls.Load() != 2 {
		t.Fatalf("fn called %d, want 2 (errors not cached)", calls.Load())
	}
}

func TestDoJSON_TypeSafeRoundtrip(t *testing.T) {
	dir := t.TempDir()
	disk := NewDiskCache(dir, 5*time.Minute)

	// Simulate first process writing to disk.
	c1 := NewQueryCache(time.Minute)
	c1.disk = disk

	type payload struct {
		Count int    `json:"count"`
		Label string `json:"label"`
	}

	fn := func() (any, error) {
		return &payload{Count: 42, Label: "test"}, nil
	}
	unmarshal := func(raw json.RawMessage) (any, error) {
		var p payload
		return &p, json.Unmarshal(raw, &p)
	}

	_, err := c1.DoJSON("k1", "payload", fn, unmarshal)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate second process (new in-memory cache, same disk).
	c2 := NewQueryCache(time.Minute)
	c2.disk = disk

	apiCalled := false
	fn2 := func() (any, error) {
		apiCalled = true
		return nil, errors.New("should not be called")
	}

	v, err := c2.DoJSON("k1", "payload", fn2, unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	if apiCalled {
		t.Fatal("API should not be called — disk should serve the result")
	}
	p := v.(*payload)
	if p.Count != 42 || p.Label != "test" {
		t.Fatalf("got %+v, want {Count:42 Label:test}", p)
	}
}
