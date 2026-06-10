package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathsUseOverrides(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SKILLYARD_CONFIG_DIR", filepath.Join(root, "config"))
	t.Setenv("SKILLYARD_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("SKILLYARD_CACHE_DIR", filepath.Join(root, "cache"))

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.LockPath != filepath.Join(root, "config", "skillyard.lock.json") {
		t.Fatalf("lock path=%q", paths.LockPath)
	}
	if paths.SourcesDir != filepath.Join(root, "data", "sources") {
		t.Fatalf("sources dir=%q", paths.SourcesDir)
	}
	if paths.CacheDir != filepath.Join(root, "cache") {
		t.Fatalf("cache dir=%q", paths.CacheDir)
	}
}

func TestSaveAndLoadLockNormalizesSelectionSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skillyard.lock.json")
	lock := NewLock()
	UpsertSubscription(&lock, Subscription{
		Source:    "source",
		Target:    "codex",
		Selection: Selection{Include: []string{"*"}},
	})
	if err := Save(path, lock); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("lockfile was empty")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Subscriptions) != 1 {
		t.Fatalf("subscriptions=%d, want 1", len(loaded.Subscriptions))
	}
	if loaded.Subscriptions[0].Selection.Exclude == nil {
		t.Fatal("exclude slice is nil after load")
	}
}

func TestLoadMissingLockReturnsEmptyLock(t *testing.T) {
	lock, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if lock.Version != Version || lock.Sources == nil {
		t.Fatalf("lock=%+v", lock)
	}
}

func TestSaveEmptyLockUsesArrays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skillyard.lock.json")
	if err := Save(path, NewLock()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, bad := range []string{`"subscriptions": null`, `"installs": null`} {
		if strings.Contains(text, bad) {
			t.Fatalf("lockfile contains %s:\n%s", bad, text)
		}
	}
}
