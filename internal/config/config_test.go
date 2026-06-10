package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lox/skillyard/internal/agent"
)

func TestLoadAgentsMissingConfigUsesBuiltIns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	registry, err := LoadAgents(filepath.Join(home, "missing.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.EnabledTargets(); !reflect.DeepEqual(got, []string{agent.Amp, agent.Codex}) {
		t.Fatalf("enabled=%+v", got)
	}
}

func TestLoadAgentsOverridesDisablesAndAddsAgents(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex-home"))
	t.Setenv("CUSTOM_SKILLS_DIR", filepath.Join(root, "claude-skills"))

	configPath := filepath.Join(root, "config.hcl")
	data := `
agent "codex" {
  skills_dir = "~/custom-codex-skills"
}

agent "amp" {
  enabled = false
}

agent "claude" {
  enabled = true
  skills_dir = "$CUSTOM_SKILLS_DIR"
}
`
	if err := os.WriteFile(configPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := LoadAgents(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.EnabledTargets(); !reflect.DeepEqual(got, []string{"claude", agent.Codex}) {
		t.Fatalf("enabled=%+v", got)
	}
	codex, err := registry.Root(agent.Codex)
	if err != nil {
		t.Fatal(err)
	}
	if codex != filepath.Join(home, "custom-codex-skills") {
		t.Fatalf("codex=%q", codex)
	}
	claude, err := registry.Root("claude")
	if err != nil {
		t.Fatal(err)
	}
	if claude != filepath.Join(root, "claude-skills") {
		t.Fatalf("claude=%q", claude)
	}
	if _, err := registry.Root(agent.Amp); err == nil {
		t.Fatal("expected disabled amp error")
	}
}

func TestDefaultContentLoadsAsHCL(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex-home"))

	content, _, err := DefaultContent()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.hcl")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadAgents(path)
	if err != nil {
		t.Fatal(err)
	}
	codex, err := registry.Root(agent.Codex)
	if err != nil {
		t.Fatal(err)
	}
	if codex != filepath.Join(root, "codex-home", "skills") {
		t.Fatalf("codex=%q", codex)
	}
}
