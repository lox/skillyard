package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverJSONReportsSkillsWithoutLockfile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")
	if err := os.MkdirAll(filepath.Join(source, "valid", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(source, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: other\ndescription: Bad\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx := commandTestContextWithWriters(root, &stdout, &bytes.Buffer{})
	if err := (DiscoverCmd{Source: source, JSON: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ctx.Paths.LockPath); !os.IsNotExist(err) {
		t.Fatalf("lockfile exists or errored after discover: %v", err)
	}

	var out discoverOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Source.Type != "local" || out.Source.Root != source {
		t.Fatalf("source=%+v, want local root %s", out.Source, source)
	}
	if len(out.Skills) != 2 {
		t.Fatalf("skills=%+v, want 2", out.Skills)
	}
	byName := map[string]discoverSkillOutput{}
	for _, s := range out.Skills {
		byName[s.Name] = s
	}
	if !byName["valid"].Installable {
		t.Fatalf("valid skill not installable: %+v", byName["valid"])
	}
	if len(byName["valid"].Warnings) != 1 || byName["valid"].Warnings[0].Code != "has-scripts" {
		t.Fatalf("valid warnings=%+v, want has-scripts", byName["valid"].Warnings)
	}
	if byName["other"].Installable || len(byName["other"].Findings) != 1 || byName["other"].Findings[0].Code != "name-mismatch" {
		t.Fatalf("bad skill=%+v, want name-mismatch finding", byName["other"])
	}
}

func TestDiscoverHumanOutputShowsInstallability(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")

	var stdout bytes.Buffer
	ctx := commandTestContextWithWriters(root, &stdout, &bytes.Buffer{})
	if err := (DiscoverCmd{Source: source}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	text := plainOutput(stdout.String())
	for _, want := range []string{"Source", "Skills", "valid", "yes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("stdout=%q, want %q", text, want)
		}
	}
}

func TestDiscoverFullDepthFindsArbitraryNestedSkill(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, filepath.Join(source, "examples", "deep", "demo"), "demo")

	var defaultOut bytes.Buffer
	defaultCtx := commandTestContextWithWriters(root, &defaultOut, &bytes.Buffer{})
	if err := (DiscoverCmd{Source: source, JSON: true}).Run(defaultCtx); err != nil {
		t.Fatal(err)
	}
	var defaultResult discoverOutput
	if err := json.Unmarshal(defaultOut.Bytes(), &defaultResult); err != nil {
		t.Fatal(err)
	}
	if len(defaultResult.Skills) != 0 {
		t.Fatalf("default skills=%+v, want none", defaultResult.Skills)
	}

	var fullOut bytes.Buffer
	fullCtx := commandTestContextWithWriters(root, &fullOut, &bytes.Buffer{})
	if err := (DiscoverCmd{Source: source, FullDepth: true, JSON: true}).Run(fullCtx); err != nil {
		t.Fatal(err)
	}
	var fullResult discoverOutput
	if err := json.Unmarshal(fullOut.Bytes(), &fullResult); err != nil {
		t.Fatal(err)
	}
	if len(fullResult.Skills) != 1 || fullResult.Skills[0].Name != "demo" {
		t.Fatalf("full-depth skills=%+v, want demo", fullResult.Skills)
	}
}
