package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type dummyItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestCache_PersistenceAcrossRestarts(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mediafs-cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.Setenv("MEDIAFS_CACHE_DIR", tempDir)
	defer os.Unsetenv("MEDIAFS_CACHE_DIR")

	c1, err := Open(7*24*time.Hour, 30*24*time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Open c1: %v", err)
	}

	items := []dummyItem{
		{ID: "m1", Name: "Movie 1"},
		{ID: "m2", Name: "Movie 2"},
	}

	if err := c1.StoreItems("server1", "lib1", items); err != nil {
		t.Fatalf("StoreItems: %v", err)
	}
	c1.Close()

	// Verify SQLite file was created on disk
	if _, err := os.Stat(filepath.Join(tempDir, "cache.db")); err != nil {
		t.Fatalf("cache.db not found on disk: %v", err)
	}

	// Open second cache instance (simulating application restart)
	c2, err := Open(7*24*time.Hour, 30*24*time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Open c2: %v", err)
	}
	defer c2.Close()

	var loaded []dummyItem
	ok, err := c2.GetItems("server1", "lib1", &loaded)
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if !ok {
		t.Fatalf("expected cache hit from SQLite disk after restart, got miss")
	}
	if len(loaded) != 2 || loaded[0].Name != "Movie 1" || loaded[1].Name != "Movie 2" {
		t.Fatalf("unexpected items loaded from disk: %+v", loaded)
	}
}
