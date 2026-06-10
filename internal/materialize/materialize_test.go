package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lox/skillyard/internal/state"
)

func TestCheckReportsInvalidSkill(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "---\nname: other\ndescription: Bad\n---\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	status := Check(state.Install{SourcePath: path, LinkPathResolved: filepath.Join(t.TempDir(), "bad")}, "local")
	if status != StatusInvalidSkill {
		t.Fatalf("status=%s, want %s", status, StatusInvalidSkill)
	}
}
