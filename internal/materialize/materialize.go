package materialize

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lox/skillyard/internal/skill"
	"github.com/lox/skillyard/internal/state"
)

type Status string

const (
	StatusLinked        Status = "linked"
	StatusMutableSource Status = "mutable-source"
	StatusDrifted       Status = "drifted"
	StatusMissingTarget Status = "missing-target"
	StatusWrongTarget   Status = "wrong-target"
	StatusMissingSource Status = "missing-source"
	StatusInvalidSkill  Status = "invalid-skill"
)

func Link(root, skillName, sourcePath string, force bool) (string, error) {
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create target root %s: %w", root, err)
	}
	linkPath := filepath.Join(root, skillName)
	if err := CanLink(root, skillName, sourcePath, force); err != nil {
		return "", err
	}

	tmp := filepath.Join(root, "."+skillName+".skillyard.tmp")
	_ = os.Remove(tmp)
	if err := os.Symlink(sourcePath, tmp); err != nil {
		return "", fmt.Errorf("create temporary symlink %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, linkPath); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("publish symlink %s: %w", linkPath, err)
	}
	if _, err := os.Stat(filepath.Join(linkPath, "SKILL.md")); err != nil {
		return "", fmt.Errorf("verify linked skill %s: %w", linkPath, err)
	}
	return linkPath, nil
}

func CanLink(root, skillName, sourcePath string, force bool) error {
	root = filepath.Clean(root)
	linkPath := filepath.Join(root, skillName)
	if err := ensureInside(root, linkPath); err != nil {
		return err
	}
	if existing, err := os.Lstat(linkPath); err == nil {
		if existing.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s exists and is not a symlink", linkPath)
		}
		target, err := os.Readlink(linkPath)
		if err != nil {
			return fmt.Errorf("read existing symlink %s: %w", linkPath, err)
		}
		absTarget, _ := filepath.Abs(resolveLinkTarget(linkPath, target))
		absSource, _ := filepath.Abs(sourcePath)
		if filepath.Clean(absTarget) == filepath.Clean(absSource) {
			return nil
		}
		if !force {
			return fmt.Errorf("%s is already linked to %s", linkPath, target)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect target path %s: %w", linkPath, err)
	}
	return nil
}

func Unlink(install state.Install, force bool) error {
	linkPath := install.LinkPathResolved
	if linkPath == "" {
		linkPath = install.LinkPath
	}
	if err := CanUnlink(install, force); err != nil {
		return err
	}
	if err := os.Remove(linkPath); err != nil {
		return fmt.Errorf("remove symlink %s: %w", linkPath, err)
	}
	return nil
}

func CanUnlink(install state.Install, force bool) error {
	linkPath := install.LinkPathResolved
	if linkPath == "" {
		linkPath = install.LinkPath
	}
	info, err := os.Lstat(linkPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", linkPath)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		return fmt.Errorf("read symlink %s: %w", linkPath, err)
	}
	expected := expectedSourcePath(install)
	absTarget, _ := filepath.Abs(resolveLinkTarget(linkPath, target))
	absExpected, _ := filepath.Abs(expected)
	if filepath.Clean(absTarget) != filepath.Clean(absExpected) && !force {
		return fmt.Errorf("%s points to %s, expected %s", linkPath, target, expected)
	}
	return nil
}

func Check(install state.Install, sourceType string) Status {
	sourcePath := expectedSourcePath(install)
	if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
		return StatusMissingSource
	}
	sourceRoot := install.SnapshotPath
	if sourceRoot == "" {
		sourceRoot = filepath.Dir(sourcePath)
	}
	parsed, err := skill.Parse(sourceRoot, sourcePath)
	if err != nil {
		return StatusInvalidSkill
	}
	if len(skill.Validate(parsed)) > 0 {
		return StatusInvalidSkill
	}
	linkPath := install.LinkPathResolved
	if linkPath == "" {
		linkPath = install.LinkPath
	}
	info, err := os.Lstat(linkPath)
	if os.IsNotExist(err) {
		return StatusMissingTarget
	}
	if err != nil {
		return StatusWrongTarget
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return StatusWrongTarget
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		return StatusWrongTarget
	}
	absTarget, _ := filepath.Abs(resolveLinkTarget(linkPath, target))
	absSource, _ := filepath.Abs(sourcePath)
	if filepath.Clean(absTarget) != filepath.Clean(absSource) {
		return StatusDrifted
	}
	if sourceType == "local" {
		return StatusMutableSource
	}
	return StatusLinked
}

func expectedSourcePath(install state.Install) string {
	if install.SnapshotPath != "" {
		return filepath.Join(install.SnapshotPath, filepath.FromSlash(install.SourcePath))
	}
	return filepath.FromSlash(install.SourcePath)
}

func resolveLinkTarget(linkPath, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(linkPath), target)
}

func ensureInside(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("validate target path: %w", err)
	}
	if rel == "." || rel == "" {
		return nil
	}
	if rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
		return fmt.Errorf("target path %s escapes root %s", path, root)
	}
	return nil
}
