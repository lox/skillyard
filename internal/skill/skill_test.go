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
