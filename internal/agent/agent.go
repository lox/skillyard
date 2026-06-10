package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	Codex = "codex"
	Amp   = "amp"
)

type Agent struct {
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	SkillsDir     string `json:"skills_dir"`
	SkillsDirExpr string `json:"skills_dir_expr"`
}

type Registry struct {
	Agents map[string]Agent
}

func BuiltInRegistry() (Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Registry{}, fmt.Errorf("resolve home directory: %w", err)
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	return NewRegistry([]Agent{
		{
			Name:          Codex,
			Enabled:       true,
			SkillsDir:     filepath.Join(codexHome, "skills"),
			SkillsDirExpr: "${CODEX_HOME:-~/.codex}/skills",
		},
		{
			Name:          Amp,
			Enabled:       true,
			SkillsDir:     filepath.Join(home, ".config", "agents", "skills"),
			SkillsDirExpr: "~/.config/agents/skills",
		},
	})
}

func NewRegistry(agents []Agent) (Registry, error) {
	registry := Registry{Agents: map[string]Agent{}}
	for _, a := range agents {
		if err := registry.Upsert(a); err != nil {
			return Registry{}, err
		}
	}
	return registry, nil
}

func (r *Registry) Upsert(a Agent) error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if a.SkillsDir == "" {
		if a.Enabled {
			return fmt.Errorf("agent %q requires skills_dir", a.Name)
		}
	} else {
		if a.SkillsDirExpr == "" {
			a.SkillsDirExpr = a.SkillsDir
		}
		resolved, err := ResolvePath(a.SkillsDir)
		if err != nil {
			return fmt.Errorf("resolve skills_dir for agent %q: %w", a.Name, err)
		}
		a.SkillsDir = resolved
	}
	if r.Agents == nil {
		r.Agents = map[string]Agent{}
	}
	r.Agents[a.Name] = a
	return nil
}

func (r Registry) Root(target string) (string, error) {
	a, ok := r.Agents[target]
	if !ok {
		return "", fmt.Errorf("unknown target %q", target)
	}
	if !a.Enabled {
		return "", fmt.Errorf("target %q is disabled", target)
	}
	return a.SkillsDir, nil
}

func (r Registry) RootExpression(target string) (string, error) {
	a, ok := r.Agents[target]
	if !ok {
		return "", fmt.Errorf("unknown target %q", target)
	}
	if !a.Enabled {
		return "", fmt.Errorf("target %q is disabled", target)
	}
	if a.SkillsDirExpr != "" {
		return a.SkillsDirExpr, nil
	}
	return a.SkillsDir, nil
}

func (r Registry) EnabledTargets() []string {
	var targets []string
	for name, a := range r.Agents {
		if a.Enabled {
			targets = append(targets, name)
		}
	}
	sort.Strings(targets)
	return targets
}

func (r Registry) EnabledAgents() []Agent {
	targets := r.EnabledTargets()
	out := make([]Agent, 0, len(targets))
	for _, target := range targets {
		out = append(out, r.Agents[target])
	}
	return out
}

func (r Registry) TargetNames() []string {
	var names []string
	for name := range r.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ResolvePath(path string) (string, error) {
	path = strings.TrimSpace(os.ExpandEnv(path))
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	return filepath.Clean(path), nil
}
