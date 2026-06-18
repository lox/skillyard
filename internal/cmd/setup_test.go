package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/skillyard/internal/state"
)

func TestSetupCreatesConfig(t *testing.T) {
	root := t.TempDir()
	ctx := setupTestContext(root)

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (SetupCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ctx.Paths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`agent "codex"`, `agent "amp"`, `skills_dir`} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q:\n%s", want, text)
		}
	}
	stdoutText := plainOutput(stdout.String())
	if !strings.Contains(stdoutText, "Wrote ") {
		t.Fatalf("stdout=%q, want wrote message", stdoutText)
	}
}

func TestSetupDoesNotOverwriteExistingConfigWithoutForce(t *testing.T) {
	root := t.TempDir()
	ctx := setupTestContext(root)
	if err := os.MkdirAll(filepath.Dir(ctx.Paths.ConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("agent \"custom\" {\n  enabled = true\n  skills_dir = \"~/custom\"\n}\n")
	if err := os.WriteFile(ctx.Paths.ConfigPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (SetupCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ctx.Paths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("config overwritten:\n%s", data)
	}
	stdoutText := plainOutput(stdout.String())
	if !strings.Contains(stdoutText, "Config already exists") {
		t.Fatalf("stdout=%q, want existing message", stdoutText)
	}
}

func TestSetupForceOverwritesExistingConfig(t *testing.T) {
	root := t.TempDir()
	ctx := setupTestContext(root)
	if err := os.MkdirAll(filepath.Dir(ctx.Paths.ConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ctx.Paths.ConfigPath, []byte("not hcl\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := (SetupCmd{Force: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ctx.Paths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `agent "codex"`) {
		t.Fatalf("config not overwritten:\n%s", data)
	}
}

func TestSetupDryRunDoesNotWriteConfig(t *testing.T) {
	root := t.TempDir()
	ctx := setupTestContext(root)

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (SetupCmd{DryRun: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ctx.Paths.ConfigPath); !os.IsNotExist(err) {
		t.Fatalf("config exists or errored after dry-run: %v", err)
	}
	stdoutText := plainOutput(stdout.String())
	if !strings.Contains(stdoutText, `agent "codex"`) {
		t.Fatalf("stdout=%q, want generated content", stdoutText)
	}
}

func TestSetupJSONReportsDetection(t *testing.T) {
	root := t.TempDir()
	ctx := setupTestContext(root)
	if err := os.MkdirAll(filepath.Join(root, "codex", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx.Paths.ConfigPath = filepath.Join(root, "config", "config.hcl")
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (SetupCmd{DryRun: true, JSON: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out setupOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.DryRun || out.ConfigPath != ctx.Paths.ConfigPath {
		t.Fatalf("output=%+v", out)
	}
	foundCodex := false
	for _, a := range out.Agents {
		if a.Name == "codex" {
			foundCodex = true
			if !a.Exists {
				t.Fatalf("codex exists=false in %+v", out.Agents)
			}
		}
	}
	if !foundCodex {
		t.Fatalf("codex missing in %+v", out.Agents)
	}
}

func setupTestContext(root string) *Context {
	ctx := commandTestContext(root)
	ctx.Paths = state.Paths{
		ConfigDir:  filepath.Join(root, "config"),
		DataDir:    filepath.Join(root, "data"),
		CacheDir:   filepath.Join(root, "cache"),
		SourcesDir: filepath.Join(root, "data", "sources"),
		ConfigPath: filepath.Join(root, "config", "config.hcl"),
		LockPath:   filepath.Join(root, "config", "skillyard.lock.json"),
	}
	return ctx
}
