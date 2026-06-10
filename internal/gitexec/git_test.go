package gitexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeGitHubShorthand(t *testing.T) {
	ref, err := Normalize("github:lox/agent-skills", "")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Type != "git" {
		t.Fatalf("type=%q, want git", ref.Type)
	}
	if ref.URL != "https://github.com/lox/agent-skills.git" {
		t.Fatalf("url=%q", ref.URL)
	}
	if ref.ID != "github-com-lox-agent-skills-0ed5901f" {
		t.Fatalf("id=%q", ref.ID)
	}
}

func TestNormalizeExplicitID(t *testing.T) {
	ref, err := Normalize("github:lox/agent-skills", "Lox Skills")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != "lox-skills" {
		t.Fatalf("id=%q", ref.ID)
	}
}

func TestNormalizeLocalPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "slack-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ref, err := Normalize(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Type != "local" {
		t.Fatalf("type=%q, want local", ref.Type)
	}
	if !filepath.IsAbs(ref.Path) {
		t.Fatalf("path=%q, want absolute", ref.Path)
	}
	if !strings.HasPrefix(ref.ID, "slack-cli-") {
		t.Fatalf("id=%q, want basename prefix", ref.ID)
	}
}

func TestNormalizeRejectsEmptyGitHubShorthand(t *testing.T) {
	if _, err := Normalize("github:", ""); err == nil {
		t.Fatal("expected error")
	}
}
