package agent

import (
	"path/filepath"
	"testing"
)

func TestBuiltInRegistryUsesCodexHomeAndHome(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(home, "codex-custom")
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codexHome)

	registry, err := BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	codex, err := registry.Root(Codex)
	if err != nil {
		t.Fatal(err)
	}
	if codex != filepath.Join(codexHome, "skills") {
		t.Fatalf("codex root=%q", codex)
	}
	amp, err := registry.Root(Amp)
	if err != nil {
		t.Fatal(err)
	}
	if amp != filepath.Join(home, ".config", "agents", "skills") {
		t.Fatalf("amp root=%q", amp)
	}
}

func TestBuiltInRegistryFallsBackToHomeCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	registry, err := BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	codex, err := registry.Root(Codex)
	if err != nil {
		t.Fatal(err)
	}
	if codex != filepath.Join(home, ".codex", "skills") {
		t.Fatalf("codex root=%q", codex)
	}
}

func TestNewRegistryRejectsEnabledAgentWithoutSkillsDir(t *testing.T) {
	if _, err := NewRegistry([]Agent{{Name: "custom", Enabled: true}}); err == nil {
		t.Fatal("expected missing skills_dir error")
	}
}
