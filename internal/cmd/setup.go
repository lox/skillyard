package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/config"
	"github.com/lox/skillyard/internal/state"
)

type SetupCmd struct {
	Force  bool `name:"force" help:"Overwrite an existing config.hcl."`
	DryRun bool `name:"dry-run" help:"Show what would be written without changing files."`
	JSON   bool `name:"json" help:"Emit machine-readable JSON."`
}

type setupOutput struct {
	ConfigPath   string       `json:"config_path"`
	ExistsBefore bool         `json:"exists_before"`
	Wrote        bool         `json:"wrote"`
	DryRun       bool         `json:"dry_run"`
	Agents       []setupAgent `json:"agents"`
	Content      string       `json:"content,omitempty"`
}

type setupAgent struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SkillsDir string `json:"skills_dir"`
	Exists    bool   `json:"exists"`
}

func (c SetupCmd) Run(ctx *Context) error {
	if ctx.Paths.LockPath == "" {
		paths, err := state.DefaultPaths()
		if err != nil {
			return err
		}
		ctx.Paths = paths
	}
	content, registry, err := config.DefaultContent()
	if err != nil {
		return err
	}
	existsBefore := exists(ctx.Paths.ConfigPath)
	out := setupOutput{
		ConfigPath:   ctx.Paths.ConfigPath,
		ExistsBefore: existsBefore,
		DryRun:       c.DryRun,
		Agents:       setupAgents(registry),
	}
	if c.DryRun {
		out.Content = string(content)
	} else if !existsBefore || c.Force {
		if err := os.MkdirAll(filepath.Dir(ctx.Paths.ConfigPath), 0o755); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}
		if err := os.WriteFile(ctx.Paths.ConfigPath, content, 0o644); err != nil {
			return fmt.Errorf("write config %s: %w", ctx.Paths.ConfigPath, err)
		}
		out.Wrote = true
	} else if existsBefore && !c.Force {
		loaded, err := config.LoadAgents(ctx.Paths.ConfigPath)
		if err != nil {
			return err
		}
		out.Agents = setupAgents(loaded)
	}
	if c.JSON {
		return writeJSON(ctx.Out, out)
	}
	printSetup(ctx.Out, out)
	return nil
}

func setupAgents(registry agent.Registry) []setupAgent {
	out := []setupAgent{}
	for _, name := range registry.TargetNames() {
		a := registry.Agents[name]
		out = append(out, setupAgent{
			Name:      a.Name,
			Enabled:   a.Enabled,
			SkillsDir: a.SkillsDir,
			Exists:    exists(a.SkillsDir),
		})
	}
	return out
}

func printSetup(out io.Writer, result setupOutput) {
	styles := newOutputStyles(out)
	switch {
	case result.DryRun:
		_, _ = fmt.Fprintf(out, "%s %s\n", styles.info.Render("Would write"), styles.muted.Render(result.ConfigPath))
	case result.Wrote:
		_, _ = fmt.Fprintf(out, "%s %s\n", styles.success.Render("Wrote"), styles.muted.Render(result.ConfigPath))
	case result.ExistsBefore:
		_, _ = fmt.Fprintf(out, "%s %s\n", styles.warn.Render("Config already exists:"), styles.muted.Render(result.ConfigPath))
	default:
		_, _ = fmt.Fprintf(out, "%s %s\n", styles.muted.Render("No changes:"), styles.muted.Render(result.ConfigPath))
	}
	_, _ = fmt.Fprintln(out)
	rows := make([][]string, 0, len(result.Agents))
	for _, a := range result.Agents {
		rows = append(rows, []string{a.Name, boolText(a.Enabled), a.SkillsDir, boolText(a.Exists)})
	}
	renderSectionTable(out, styles, "Agents", []string{"NAME", "ENABLED", "SKILLS_DIR", "EXISTS"}, rows, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 1:
			return boolStyle(styles, value)
		case 3:
			return existsStyle(styles, value)
		case 2:
			return styles.muted
		default:
			return styles.cell
		}
	})
	if result.DryRun && result.Content != "" {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprint(out, result.Content)
	}
}
