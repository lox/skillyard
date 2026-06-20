package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsePrintsSelectedSkillWithoutLockfile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "alpha")
	writeCommandTestSkill(t, source, "beta")

	var stdout bytes.Buffer
	ctx := commandTestContextWithWriters(root, &stdout, &bytes.Buffer{})
	if err := (UseCmd{Source: source, Include: []string{"beta"}}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ctx.Paths.LockPath); !os.IsNotExist(err) {
		t.Fatalf("lockfile exists or errored after use: %v", err)
	}
	text := stdout.String()
	if !strings.Contains(text, "name: beta") {
		t.Fatalf("stdout=%q, want beta skill content", text)
	}
	if strings.Contains(text, "name: alpha") {
		t.Fatalf("stdout=%q, want only selected skill", text)
	}
}

func TestUseDefaultsToOnlySkill(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "only")

	var stdout bytes.Buffer
	ctx := commandTestContextWithWriters(root, &stdout, &bytes.Buffer{})
	if err := (UseCmd{Source: source}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "name: only") {
		t.Fatalf("stdout=%q, want only skill content", stdout.String())
	}
}

func TestUseErrorsWhenSelectionIsAmbiguous(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "alpha")
	writeCommandTestSkill(t, source, "beta")

	ctx := commandTestContext(root)
	err := (UseCmd{Source: source}).Run(ctx)
	if err == nil {
		t.Fatal("expected ambiguous selection error")
	}
	if !strings.Contains(err.Error(), "contains multiple skills") {
		t.Fatalf("err=%v, want multiple skills error", err)
	}

	err = (UseCmd{Source: source, Include: []string{"*"}}).Run(ctx)
	if err == nil {
		t.Fatal("expected multiple match error")
	}
	if !strings.Contains(err.Error(), "matched multiple skills") {
		t.Fatalf("err=%v, want multiple match error", err)
	}
}

func TestUseRejectsInvalidSkill(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	bad := filepath.Join(source, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: other\ndescription: Bad\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := commandTestContext(root)
	err := (UseCmd{Source: source, Include: []string{"other"}}).Run(ctx)
	if err == nil {
		t.Fatal("expected invalid skill error")
	}
	if !strings.Contains(err.Error(), "not installable") {
		t.Fatalf("err=%v, want not installable error", err)
	}
}
