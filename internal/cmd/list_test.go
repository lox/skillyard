package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/state"
)

func TestListIncludesUnmanagedSkills(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "managed")
	writeCommandTestSkill(t, source, "linked")

	ctx := commandTestContext(root)
	if err := (SubscribeCmd{
		Source:  source,
		Include: []string{"managed"},
		Target:  []string{agent.Codex},
	}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rootForContext(t, ctx, agent.Codex), "local-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootForContext(t, ctx, agent.Codex), "local-file"), []byte("not a skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "linked"), filepath.Join(rootForContext(t, ctx, agent.Codex), "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "missing"), filepath.Join(rootForContext(t, ctx, agent.Codex), "broken")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "linked"), filepath.Join(rootForContext(t, ctx, agent.Codex), ".linked.skillyard.tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rootForContext(t, ctx, agent.Amp), "amp-local"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (ListCmd{JSON: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out listOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Installs) != 1 {
		t.Fatalf("installs=%d, want 1", len(out.Installs))
	}
	got := map[string]unmanagedOutput{}
	for _, item := range out.Unmanaged {
		got[item.Target+"/"+item.Skill] = item
	}
	assertUnmanagedKind(t, got, "codex/linked", "symlink")
	assertUnmanagedKind(t, got, "codex/broken", "broken-symlink")
	assertUnmanagedKind(t, got, "codex/local-dir", "dir")
	assertUnmanagedKind(t, got, "codex/local-file", "file")
	assertUnmanagedKind(t, got, "amp/amp-local", "dir")
	if _, ok := got["codex/managed"]; ok {
		t.Fatalf("managed skill appeared as unmanaged: %+v", got["codex/managed"])
	}
	if _, ok := got["codex/.linked.skillyard.tmp"]; ok {
		t.Fatalf("dot temp link appeared as unmanaged")
	}
	if got["codex/linked"].LinkTarget == "" {
		t.Fatalf("symlink link_target was empty")
	}
}

func TestListHumanOutputShowsUnmanagedSectionWithPaths(t *testing.T) {
	root := t.TempDir()
	ctx := commandTestContext(root)
	if err := os.MkdirAll(filepath.Join(rootForContext(t, ctx, agent.Codex), "local-dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (ListCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, want := range []string{"UNMANAGED", "local-dir", rootForContext(t, ctx, agent.Codex)} {
		if !strings.Contains(text, want) {
			t.Fatalf("list output missing %q:\n%s", want, text)
		}
	}
}

func TestListUnmanagedEmptyWhenRootsMissing(t *testing.T) {
	root := t.TempDir()
	ctx := commandTestContext(root)
	if err := state.Save(ctx.Paths.LockPath, state.NewLock()); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (ListCmd{JSON: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out listOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Unmanaged) != 0 {
		t.Fatalf("unmanaged=%+v, want empty", out.Unmanaged)
	}
}

func assertUnmanagedKind(t *testing.T, got map[string]unmanagedOutput, key, kind string) {
	t.Helper()
	item, ok := got[key]
	if !ok {
		t.Fatalf("missing unmanaged %s in %+v", key, got)
	}
	if item.Kind != kind {
		t.Fatalf("%s kind=%q, want %q", key, item.Kind, kind)
	}
	if item.Path == "" {
		t.Fatalf("%s path was empty", key)
	}
}
