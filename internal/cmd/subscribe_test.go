package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/state"
)

func TestSubscribeJSONSavesLockfile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")

	ctx := commandTestContext(root)
	cmd := SubscribeCmd{
		Source:  source,
		Include: []string{"valid"},
		Target:  []string{agent.Codex},
		JSON:    true,
	}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	lock, err := state.Load(ctx.Paths.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Installs) != 1 {
		t.Fatalf("installs=%d, want 1", len(lock.Installs))
	}
	if lock.Subscriptions[0].Selection.Exclude == nil {
		t.Fatalf("exclude selection encoded as nil")
	}
	if _, err := os.Stat(filepath.Join(rootForContext(t, ctx, agent.Codex), "valid", "SKILL.md")); err != nil {
		t.Fatalf("linked skill missing: %v", err)
	}
}

func TestSubscribeDefaultsToAllEnabledTargets(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")

	ctx := commandTestContext(root)
	cmd := SubscribeCmd{
		Source:  source,
		Include: []string{"valid"},
	}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	lock, err := state.Load(ctx.Paths.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Subscriptions) != 2 {
		t.Fatalf("subscriptions=%+v, want codex and amp defaults", lock.Subscriptions)
	}
	if _, err := os.Stat(filepath.Join(rootForContext(t, ctx, agent.Codex), "valid", "SKILL.md")); err != nil {
		t.Fatalf("default codex link missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootForContext(t, ctx, agent.Amp), "valid", "SKILL.md")); err != nil {
		t.Fatalf("default amp link missing: %v", err)
	}
}

func TestSubscribeDefaultsToConfiguredCustomTarget(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")

	ctx := commandTestContext(root)
	agents, err := agent.NewRegistry([]agent.Agent{
		{Name: "custom", Enabled: true, SkillsDir: filepath.Join(root, "custom"), SkillsDirExpr: "~/custom/skills"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Agents = agents
	cmd := SubscribeCmd{
		Source:  source,
		Include: []string{"valid"},
	}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	lock, err := state.Load(ctx.Paths.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Subscriptions) != 1 || lock.Subscriptions[0].Target != "custom" {
		t.Fatalf("subscriptions=%+v, want custom default", lock.Subscriptions)
	}
	if _, err := os.Stat(filepath.Join(rootForContext(t, ctx, "custom"), "valid", "SKILL.md")); err != nil {
		t.Fatalf("custom link missing: %v", err)
	}
}

func TestSubscribeLoadsHCLConfigForDefaultTargets(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configData := `
agent "codex" {
  enabled = false
}

agent "amp" {
  enabled = false
}

agent "custom" {
  enabled    = true
  skills_dir = "~/custom-skills"
}
`
	if err := os.WriteFile(filepath.Join(configDir, "config.hcl"), []byte(configData), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := NewContext(&stdout, &stderr)
	ctx.Paths = state.Paths{
		ConfigDir:  configDir,
		DataDir:    filepath.Join(root, "data"),
		CacheDir:   filepath.Join(root, "cache"),
		SourcesDir: filepath.Join(root, "data", "sources"),
		ConfigPath: filepath.Join(configDir, "config.hcl"),
		LockPath:   filepath.Join(configDir, "skillyard.lock.json"),
	}
	t.Setenv("HOME", filepath.Join(root, "home"))

	if err := (SubscribeCmd{Source: source, Include: []string{"valid"}}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	lock, err := state.Load(ctx.Paths.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Subscriptions) != 1 || lock.Subscriptions[0].Target != "custom" {
		t.Fatalf("subscriptions=%+v, want custom only", lock.Subscriptions)
	}
	if _, err := os.Stat(filepath.Join(root, "home", "custom-skills", "valid", "SKILL.md")); err != nil {
		t.Fatalf("custom skill link missing: %v", err)
	}
}

func TestSubscribeDryRunJSONDoesNotSaveLockfile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")

	ctx := commandTestContext(root)
	cmd := SubscribeCmd{
		Source:  source,
		Include: []string{"valid"},
		Target:  []string{agent.Codex},
		DryRun:  true,
		JSON:    true,
	}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ctx.Paths.LockPath); !os.IsNotExist(err) {
		t.Fatalf("lockfile exists or errored after dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootForContext(t, ctx, agent.Codex), "valid")); !os.IsNotExist(err) {
		t.Fatalf("skill link exists or errored after dry-run: %v", err)
	}
}

func TestSubscribeHumanOutputWritesWarningsToStderr(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")
	if err := os.MkdirAll(filepath.Join(source, "valid", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := commandTestContextWithWriters(root, &stdout, &stderr)
	cmd := SubscribeCmd{
		Source:  source,
		Include: []string{"valid"},
		Target:  []string{agent.Codex},
	}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "warning: has-scripts") {
		t.Fatalf("stderr=%q, want has-scripts warning", stderr.String())
	}
	if strings.Contains(stdout.String(), "warning:") {
		t.Fatalf("stdout contains warning: %q", stdout.String())
	}
}

func TestSyncUnknownSourceErrors(t *testing.T) {
	root := t.TempDir()
	ctx := commandTestContext(root)
	if err := state.Save(ctx.Paths.LockPath, state.NewLock()); err != nil {
		t.Fatal(err)
	}
	err := (SyncCmd{Source: "github:missing/source"}).Run(ctx)
	if err == nil {
		t.Fatal("expected unknown source error")
	}
}

func TestSubscribeInvalidTargetErrors(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")

	ctx := commandTestContext(root)
	err := (SubscribeCmd{
		Source:  source,
		Include: []string{"valid"},
		Target:  []string{"claude"},
	}).Run(ctx)
	if err == nil {
		t.Fatal("expected invalid target error")
	}
}

func commandTestContext(root string) *Context {
	return commandTestContextWithWriters(root, &bytes.Buffer{}, &bytes.Buffer{})
}

func commandTestContextWithWriters(root string, out, stderr *bytes.Buffer) *Context {
	ctx := NewContext(out, stderr)
	ctx.Paths = state.Paths{
		ConfigDir:  filepath.Join(root, "config"),
		DataDir:    filepath.Join(root, "data"),
		CacheDir:   filepath.Join(root, "cache"),
		SourcesDir: filepath.Join(root, "data", "sources"),
		ConfigPath: filepath.Join(root, "config", "config.hcl"),
		LockPath:   filepath.Join(root, "config", "skillyard.lock.json"),
	}
	agents, err := agent.NewRegistry([]agent.Agent{
		{Name: agent.Codex, Enabled: true, SkillsDir: filepath.Join(root, "codex"), SkillsDirExpr: "${CODEX_HOME:-~/.codex}/skills"},
		{Name: agent.Amp, Enabled: true, SkillsDir: filepath.Join(root, "amp"), SkillsDirExpr: "~/.config/agents/skills"},
	})
	if err != nil {
		panic(err)
	}
	ctx.Agents = agents
	return ctx
}

func rootForContext(t *testing.T, ctx *Context, target string) string {
	t.Helper()
	root, err := ctx.Agents.Root(target)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeCommandTestSkill(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "---\nname: " + name + "\ndescription: Test skill\n---\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
