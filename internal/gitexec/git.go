package gitexec

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type SourceRef struct {
	Input string
	Type  string
	URL   string
	Ref   string
	Path  string
	ID    string
}

func Normalize(input string, explicitID string) (SourceRef, error) {
	return NormalizeWithRef(input, explicitID, "")
}

func NormalizeWithRef(input string, explicitID string, gitRef string) (SourceRef, error) {
	if input == "" {
		return SourceRef{}, fmt.Errorf("source is required")
	}
	ref := SourceRef{Input: input}
	if strings.HasPrefix(input, "github:") {
		repo := strings.TrimPrefix(input, "github:")
		if repo == "" || strings.Contains(repo, "://") {
			return SourceRef{}, fmt.Errorf("invalid GitHub shorthand %q", input)
		}
		ref.Type = "git"
		ref.URL = "https://github.com/" + strings.TrimSuffix(repo, ".git") + ".git"
	} else if isGitURL(input) {
		ref.Type = "git"
		ref.URL = input
	} else {
		abs, err := filepath.Abs(expandHome(input))
		if err != nil {
			return SourceRef{}, fmt.Errorf("resolve local source path: %w", err)
		}
		ref.Type = "local"
		ref.Path = filepath.Clean(abs)
	}
	if gitRef != "" {
		if ref.Type != "git" {
			return SourceRef{}, fmt.Errorf("--ref is only supported for Git sources")
		}
		ref.Ref = gitRef
	}
	if explicitID != "" {
		ref.ID = slug(explicitID)
	} else if ref.Type == "local" {
		ref.ID = localSourceID(ref.Path)
	} else {
		key := ref.URL
		if ref.Ref != "" {
			key = strings.TrimSuffix(ref.URL, ".git") + "#" + ref.Ref
		}
		ref.ID = sourceID(key)
	}
	return ref, nil
}

func isGitURL(input string) bool {
	if strings.HasPrefix(input, "git@") {
		return true
	}
	if strings.HasPrefix(input, "ssh://") {
		return true
	}
	u, err := url.Parse(input)
	if err != nil {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "git" || u.Scheme == "file"
}

func sourceID(key string) string {
	hash := sha1.Sum([]byte(key))
	return slug(key) + "-" + hex.EncodeToString(hash[:])[:8]
}

func localSourceID(path string) string {
	hash := sha1.Sum([]byte(path))
	return slug(filepath.Base(path)) + "-" + hex.EncodeToString(hash[:])[:8]
}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slug(s string) string {
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "ssh://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.ReplaceAll(s, ":", "-")
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "source"
	}
	return strings.ToLower(s)
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

type Git struct {
	Bin string
}

func New() Git {
	return Git{Bin: "git"}
}

func (g Git) Version() (string, error) {
	out, err := g.run("", "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g Git) EnsureClone(url, path string) error {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create source parent: %w", err)
	}
	if _, err := g.run("", "clone", url, path); err != nil {
		return fmt.Errorf("clone %s: %w", url, err)
	}
	return nil
}

func (g Git) Fetch(path string) error {
	if err := g.FetchRefs(path); err != nil {
		return err
	}
	if _, err := g.run(path, "pull", "--ff-only"); err != nil {
		return fmt.Errorf("fast-forward %s: %w", path, err)
	}
	return nil
}

func (g Git) FetchRefs(path string) error {
	if _, err := g.run(path, "fetch", "--prune", "--tags"); err != nil {
		return fmt.Errorf("fetch %s: %w", path, err)
	}
	return nil
}

func (g Git) Head(path string) (string, error) {
	out, err := g.run(path, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve HEAD for %s: %w", path, err)
	}
	return strings.TrimSpace(out), nil
}

func (g Git) CheckoutRef(path, ref string) (string, error) {
	commit, err := g.ResolveRef(path, ref)
	if err != nil {
		return "", err
	}
	if _, err := g.run(path, "checkout", "--detach", commit); err != nil {
		return "", fmt.Errorf("checkout ref %s in %s: %w", ref, path, err)
	}
	return commit, nil
}

func (g Git) ResolveRef(path, ref string) (string, error) {
	if ref == "" {
		return g.Head(path)
	}
	candidates := []string{
		"refs/remotes/origin/" + ref,
		"refs/tags/" + ref,
		ref,
	}
	var lastErr error
	for _, candidate := range candidates {
		out, err := g.run(path, "rev-parse", "--verify", candidate+"^{commit}")
		if err == nil {
			return strings.TrimSpace(out), nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("resolve ref %s for %s: %w", ref, path, lastErr)
}

func (g Git) Snapshot(repoPath, commit, snapshotPath string) error {
	if _, err := os.Stat(snapshotPath); err == nil {
		return nil
	}
	tmp := snapshotPath + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("create snapshot temp: %w", err)
	}
	archive := exec.Command(g.Bin, "-C", repoPath, "archive", commit)
	tar := exec.Command("tar", "-x", "-C", tmp)
	reader, err := archive.StdoutPipe()
	if err != nil {
		return fmt.Errorf("prepare git archive: %w", err)
	}
	tar.Stdin = reader
	if err := archive.Start(); err != nil {
		return fmt.Errorf("start git archive: %w", err)
	}
	if err := tar.Start(); err != nil {
		_ = archive.Wait()
		return fmt.Errorf("start tar extract: %w", err)
	}
	if err := archive.Wait(); err != nil {
		_ = tar.Wait()
		return fmt.Errorf("git archive %s: %w", commit, err)
	}
	if err := tar.Wait(); err != nil {
		return fmt.Errorf("extract snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		return fmt.Errorf("create snapshot parent: %w", err)
	}
	if err := os.Rename(tmp, snapshotPath); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("publish snapshot: %w", err)
	}
	return nil
}

func (g Git) run(dir string, args ...string) (string, error) {
	cmd := exec.Command(g.Bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", g.Bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
