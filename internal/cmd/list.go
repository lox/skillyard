package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/lox/skillyard/internal/materialize"
	"github.com/lox/skillyard/internal/state"
)

type ListCmd struct {
	JSON bool `name:"json" help:"Emit machine-readable JSON."`
}

type listOutput struct {
	Subscriptions []subscriptionOutput `json:"subscriptions"`
	Installs      []installOutput      `json:"installs"`
	Unmanaged     []unmanagedOutput    `json:"unmanaged"`
}

type subscriptionOutput struct {
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type installOutput struct {
	state.Install
	Status materialize.Status `json:"status"`
}

type unmanagedOutput struct {
	Skill      string `json:"skill"`
	Target     string `json:"target"`
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	LinkTarget string `json:"link_target,omitempty"`
}

func (c ListCmd) Run(ctx *Context) error {
	lock, err := loadLock(ctx)
	if err != nil {
		return err
	}
	out := listOutput{
		Subscriptions: []subscriptionOutput{},
		Installs:      []installOutput{},
		Unmanaged:     []unmanagedOutput{},
	}
	for _, sub := range lock.Subscriptions {
		out.Subscriptions = append(out.Subscriptions, subscriptionOutput{
			Source:  sub.Source,
			Target:  sub.Target,
			Include: sub.Selection.Include,
			Exclude: sub.Selection.Exclude,
		})
	}
	for _, install := range lock.Installs {
		sourceType := ""
		if src, ok := lock.Sources[install.Source]; ok {
			sourceType = src.Type
		}
		out.Installs = append(out.Installs, installOutput{
			Install: install,
			Status:  materialize.Check(install, sourceType),
		})
	}
	unmanaged, err := listUnmanaged(ctx, lock)
	if err != nil {
		return err
	}
	out.Unmanaged = unmanaged
	if c.JSON {
		return writeJSON(ctx.Out, out)
	}
	styles := newOutputStyles(ctx.Out)
	subscriptions := make([][]string, 0, len(out.Subscriptions))
	for _, sub := range out.Subscriptions {
		subscriptions = append(subscriptions, []string{
			sub.Target,
			sub.Source,
			selectionText(sub.Include, "auto"),
			selectionText(sub.Exclude, "-"),
		})
	}
	renderSectionTable(ctx.Out, styles, "Subscriptions", []string{"TARGET", "SOURCE", "INCLUDE", "EXCLUDE"}, subscriptions, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 1:
			return styles.info
		case 2:
			if value == "auto" {
				return styles.muted
			}
		case 3:
			return mutedIfDash(styles, value)
		}
		return styles.cell
	})
	_, _ = fmt.Fprintln(ctx.Out)

	managed := make([][]string, 0, len(out.Installs))
	for _, install := range out.Installs {
		managed = append(managed, []string{install.Skill, install.Target, install.Source, string(install.Status), install.LinkPathResolved})
	}
	renderSectionTable(ctx.Out, styles, "Managed", []string{"SKILL", "TARGET", "SOURCE", "STATUS", "PATH"}, managed, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 2:
			return styles.info
		case 3:
			return installStatusStyle(styles, value)
		case 4:
			return styles.muted
		default:
			return styles.cell
		}
	})
	_, _ = fmt.Fprintln(ctx.Out)

	unmanagedRows := make([][]string, 0, len(out.Unmanaged))
	for _, item := range out.Unmanaged {
		unmanagedRows = append(unmanagedRows, []string{item.Skill, item.Target, item.Kind, item.Path, dashIfEmpty(item.LinkTarget)})
	}
	renderSectionTable(ctx.Out, styles, "Unmanaged", []string{"SKILL", "TARGET", "KIND", "PATH", "LINK_TARGET"}, unmanagedRows, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 2:
			return unmanagedKindStyle(styles, value)
		case 3, 4:
			return mutedIfDash(styles, value)
		default:
			return styles.cell
		}
	})
	return nil
}

func installStatusStyle(styles outputStyles, status string) lipgloss.Style {
	switch materialize.Status(status) {
	case materialize.StatusLinked:
		return styles.success
	case materialize.StatusMutableSource:
		return styles.info
	case materialize.StatusDrifted:
		return styles.warn
	case materialize.StatusMissingTarget, materialize.StatusWrongTarget, materialize.StatusMissingSource, materialize.StatusInvalidSkill:
		return styles.danger
	default:
		return styles.cell
	}
}

func unmanagedKindStyle(styles outputStyles, kind string) lipgloss.Style {
	switch kind {
	case "broken-symlink":
		return styles.danger
	case "symlink":
		return styles.info
	case "dir":
		return styles.accent
	case "file":
		return styles.muted
	default:
		return styles.cell
	}
}

func listUnmanaged(ctx *Context, lock state.Lock) ([]unmanagedOutput, error) {
	managed := map[string]bool{}
	for _, install := range lock.Installs {
		managed[install.Target+"\x00"+install.Skill] = true
	}
	targets := []struct {
		name string
		root string
	}{}
	for _, a := range ctx.Agents.EnabledAgents() {
		targets = append(targets, struct {
			name string
			root string
		}{name: a.Name, root: a.SkillsDir})
	}
	var out []unmanagedOutput
	for _, target := range targets {
		items, err := unmanagedInRoot(target.name, target.root, managed)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].Skill < out[j].Skill
	})
	return out, nil
}

func unmanagedInRoot(target, root string, managed map[string]bool) ([]unmanagedOutput, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s skills root %s: %w", target, root, err)
	}
	var out []unmanagedOutput
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] == '.' {
			continue
		}
		if managed[target+"\x00"+name] {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect unmanaged skill path %s: %w", path, err)
		}
		item := unmanagedOutput{
			Skill:  name,
			Target: target,
			Path:   path,
			Kind:   unmanagedKind(info),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			targetPath, err := os.Readlink(path)
			if err != nil {
				return nil, fmt.Errorf("read unmanaged symlink %s: %w", path, err)
			}
			item.LinkTarget = targetPath
			resolved := targetPath
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			if _, err := os.Stat(resolved); os.IsNotExist(err) {
				item.Kind = "broken-symlink"
			} else if err != nil {
				return nil, fmt.Errorf("inspect unmanaged symlink target %s: %w", resolved, err)
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func unmanagedKind(info os.FileInfo) string {
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "dir"
	}
	if info.Mode().IsRegular() {
		return "file"
	}
	return "other"
}
