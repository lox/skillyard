package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const Version = 1

type Lock struct {
	Version       int               `json:"version"`
	Sources       map[string]Source `json:"sources"`
	Subscriptions []Subscription    `json:"subscriptions"`
	Installs      []Install         `json:"installs"`
}

type Source struct {
	Input          string `json:"input"`
	Type           string `json:"type"`
	URL            string `json:"url,omitempty"`
	Ref            string `json:"ref,omitempty"`
	CheckoutPath   string `json:"checkout_path,omitempty"`
	LastSeenCommit string `json:"last_seen_commit,omitempty"`
	InputPath      string `json:"input_path,omitempty"`
	ResolvedPath   string `json:"resolved_path,omitempty"`
}

type Subscription struct {
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	Selection Selection `json:"selection"`
}

type Selection struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type Install struct {
	Skill              string `json:"skill"`
	Source             string `json:"source"`
	SourcePath         string `json:"source_path"`
	SourceCommit       string `json:"source_commit,omitempty"`
	SnapshotPath       string `json:"snapshot_path,omitempty"`
	Target             string `json:"target"`
	TargetRoot         string `json:"target_root"`
	TargetRootResolved string `json:"target_root_resolved"`
	LinkPath           string `json:"link_path"`
	LinkPathResolved   string `json:"link_path_resolved"`
}

func NewLock() Lock {
	return Lock{
		Version: Version,
		Sources: map[string]Source{},
	}
}

func Load(path string) (Lock, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewLock(), nil
	}
	if err != nil {
		return Lock{}, fmt.Errorf("read lockfile: %w", err)
	}
	if len(data) == 0 {
		return NewLock(), nil
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lock{}, fmt.Errorf("parse lockfile: %w", err)
	}
	if lock.Version == 0 {
		lock.Version = Version
	}
	if lock.Sources == nil {
		lock.Sources = map[string]Source{}
	}
	normalizeLock(&lock)
	return lock, nil
}

func Save(path string, lock Lock) error {
	if lock.Version == 0 {
		lock.Version = Version
	}
	if lock.Sources == nil {
		lock.Sources = map[string]Source{}
	}
	normalizeLock(&lock)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lockfile directory: %w", err)
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lockfile: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".skillyard.lock.*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary lockfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary lockfile: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary lockfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary lockfile: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace lockfile: %w", err)
	}
	return nil
}

func UpsertSource(lock *Lock, id string, source Source) {
	if lock.Sources == nil {
		lock.Sources = map[string]Source{}
	}
	lock.Sources[id] = source
}

func UpsertSubscription(lock *Lock, sub Subscription) {
	sub.Selection = normalizeSelection(sub.Selection)
	for i, existing := range lock.Subscriptions {
		if existing.Source == sub.Source && existing.Target == sub.Target {
			lock.Subscriptions[i] = sub
			return
		}
	}
	lock.Subscriptions = append(lock.Subscriptions, sub)
}

func RemoveEmptySubscriptions(lock *Lock) {
	var kept []Subscription
	for _, sub := range lock.Subscriptions {
		sub.Selection = normalizeSelection(sub.Selection)
		if len(sub.Selection.Include) > 0 {
			kept = append(kept, sub)
		}
	}
	lock.Subscriptions = kept
}

func normalizeLock(lock *Lock) {
	if lock.Subscriptions == nil {
		lock.Subscriptions = []Subscription{}
	}
	if lock.Installs == nil {
		lock.Installs = []Install{}
	}
	for i := range lock.Subscriptions {
		lock.Subscriptions[i].Selection = normalizeSelection(lock.Subscriptions[i].Selection)
	}
}

func normalizeSelection(selection Selection) Selection {
	if selection.Include == nil {
		selection.Include = []string{}
	}
	if selection.Exclude == nil {
		selection.Exclude = []string{}
	}
	return selection
}
