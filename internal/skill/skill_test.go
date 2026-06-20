package skill

import (
	"os"
	"path/filepath"
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

func TestDiscoverSingleSkillRoot(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, filepath.Base(root), "Single")

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
