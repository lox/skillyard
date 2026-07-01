package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverTopLevelAndSkillsContainer(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "alpha"), "alpha", "Alpha")
	writeTestSkill(t, filepath.Join(root, "skills", "beta"), "beta", "Beta")

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills=%d, want 2", len(skills))
	}
	if skills[0].Name != "alpha" || skills[0].RelPath != "alpha" {
		t.Fatalf("first skill=%+v", skills[0])
	}
	if skills[1].Name != "beta" || skills[1].RelPath != "skills/beta" {
		t.Fatalf("second skill=%+v", skills[1])
	}
}

func TestDiscoverCatalogAndAgentSkillContainers(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "skills", "productivity", "teach"), "teach", "Teach")
	writeTestSkill(t, filepath.Join(root, ".agents", "skills", "shared"), "shared", "Shared")
	writeTestSkill(t, filepath.Join(root, ".claude", "skills", "claude-only"), "claude-only", "Claude")

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range skills {
		got[s.Name] = s.RelPath
	}
	want := map[string]string{
		"teach":       "skills/productivity/teach",
		"shared":      ".agents/skills/shared",
		"claude-only": ".claude/skills/claude-only",
	}
	for name, rel := range want {
		if got[name] != rel {
			t.Fatalf("skill %q rel=%q, want %q; all=%+v", name, got[name], rel, skills)
		}
	}
}

func TestDiscoverDedupesSymlinkedSkillContainers(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "skills", "o11ylite"), "o11ylite", "O11yLite")
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../skills", filepath.Join(root, ".agents", "skills")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills=%+v, want one deduped skill", skills)
	}
	if skills[0].Name != "o11ylite" || skills[0].RelPath != "skills/o11ylite" {
		t.Fatalf("skill=%+v, want o11ylite at skills/o11ylite", skills[0])
	}
}

func TestDiscoverWithFullDepthFindsArbitraryNestedSkills(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "examples", "deep", "demo"), "demo", "Demo")
	writeTestSkill(t, filepath.Join(root, "node_modules", "ignored"), "ignored", "Ignored")

	skills, err := DiscoverWithOptions(root, DiscoveryOptions{FullDepth: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills=%+v, want only full-depth demo skill", skills)
	}
	if skills[0].Name != "demo" || skills[0].RelPath != "examples/deep/demo" {
		t.Fatalf("skill=%+v, want demo at examples/deep/demo", skills[0])
	}
}

func TestDiscoverPluginManifestSkills(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "plugin-skills", "review"), "review", "Review")
	writeTestSkill(t, filepath.Join(root, "plugins", "deploy", "resources", "deploy-review"), "deploy-review", "Deploy")
	writeFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{
  "name": "review-plugin",
  "skills": ["plugin-skills/review"]
}`)
	writeFile(t, filepath.Join(root, ".codex-plugin", "marketplace.json"), `{
  "metadata": { "pluginRoot": "plugins" },
  "plugins": [
    {
      "name": "deploy",
      "source": "deploy",
      "skills": ["resources/deploy-review"]
    }
  ]
}`)

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range skills {
		got[s.Name] = s.RelPath
	}
	want := map[string]string{
		"review":        "plugin-skills/review",
		"deploy-review": "plugins/deploy/resources/deploy-review",
	}
	for name, rel := range want {
		if got[name] != rel {
			t.Fatalf("skill %q rel=%q, want %q; all=%+v", name, got[name], rel, skills)
		}
	}
}

func TestDiscoverPluginManifestAcceptsStringSkillDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "plugin-skills", "one"), "one", "One")
	writeTestSkill(t, filepath.Join(root, "plugin-skills", "two"), "two", "Two")
	writeFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{
  "name": "plugin",
  "skills": "plugin-skills"
}`)

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills=%+v, want two plugin skill children", skills)
	}
}

func TestDiscoverPluginManifestRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{
  "name": "bad-plugin",
  "skills": ["../outside"]
}`)

	_, err := Discover(root)
	if err == nil {
		t.Fatal("expected escaping plugin skill path error")
	}
	if !strings.Contains(err.Error(), "escapes source root") {
		t.Fatalf("err=%v, want escape error", err)
	}
}

func TestDiscoverSingleSkillRoot(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "single", "Single")

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills=%d, want 1", len(skills))
	}
	if skills[0].RelPath != "." {
		t.Fatalf("relpath=%q, want .", skills[0].RelPath)
	}

	inspections, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspections) != 1 || len(inspections[0].Findings) != 0 {
		t.Fatalf("inspections=%+v, want root skill without findings", inspections)
	}
}

func TestInspectReportsInvalidCandidates(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, filepath.Join(root, "valid"), "valid", "Valid")
	bad := filepath.Join(root, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("plain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inspections, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspections) != 2 {
		t.Fatalf("inspections=%d, want 2", len(inspections))
	}
	if inspections[0].Skill.Name != "bad" || len(inspections[0].Findings) != 1 || inspections[0].Findings[0].Code != "invalid-skill" {
		t.Fatalf("bad inspection=%+v", inspections[0])
	}
	if inspections[1].Skill.Name != "valid" || len(inspections[1].Findings) != 0 {
		t.Fatalf("valid inspection=%+v", inspections[1])
	}
}

func TestWarningsIgnoreGitMetadata(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "single", "Single")
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	writeFile(t, hook, "#!/bin/sh\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}

	inspections, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspections) != 1 || len(inspections[0].Skill.Warnings) != 0 {
		t.Fatalf("warnings=%+v, want none from .git metadata", inspections)
	}
}

func TestParseRequiresFrontmatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("plain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(root, path); err == nil {
		t.Fatal("expected frontmatter error")
	}
}

func TestValidateRequiresNameDescriptionAndDirectoryMatch(t *testing.T) {
	findings := Validate(Skill{Name: "other", Path: filepath.Join(t.TempDir(), "actual")})
	codes := map[string]bool{}
	for _, finding := range findings {
		codes[finding.Code] = true
	}
	for _, code := range []string{"missing-description", "name-mismatch"} {
		if !codes[code] {
			t.Fatalf("missing finding %q in %+v", code, findings)
		}
	}
}

func writeTestSkill(t *testing.T, path, name, description string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
