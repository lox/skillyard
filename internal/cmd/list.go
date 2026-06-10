package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

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
	w := tabwriter.NewWriter(ctx.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SUBSCRIPTIONS")
	_, _ = fmt.Fprintln(w, "TARGET\tSOURCE\tINCLUDE\tEXCLUDE")
	for _, sub := range out.Subscriptions {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%v\n", sub.Target, sub.Source, sub.Include, sub.Exclude)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "MANAGED")
	_, _ = fmt.Fprintln(w, "SKILL\tTARGET\tSOURCE\tSTATUS\tPATH")
	for _, install := range out.Installs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", install.Skill, install.Target, install.Source, install.Status, install.LinkPathResolved)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "UNMANAGED")
	_, _ = fmt.Fprintln(w, "SKILL\tTARGET\tKIND\tPATH\tLINK_TARGET")
	for _, item := range out.Unmanaged {
		linkTarget := item.LinkTarget
		if linkTarget == "" {
			linkTarget = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", item.Skill, item.Target, item.Kind, item.Path, linkTarget)
	}
	return w.Flush()
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
