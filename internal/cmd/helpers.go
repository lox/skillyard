package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/skill"
	"github.com/lox/skillyard/internal/state"
	syncer "github.com/lox/skillyard/internal/sync"
)

func loadLock(ctx *Context) (state.Lock, error) {
	if err := ctx.ensureRuntime(); err != nil {
		return state.Lock{}, err
	}
	return state.Load(ctx.Paths.LockPath)
}

func saveLock(ctx *Context, lock state.Lock) error {
	if err := ctx.Paths.Ensure(); err != nil {
		return err
	}
	return state.Save(ctx.Paths.LockPath, lock)
}

func writeJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printActions(out io.Writer, result syncer.Result) {
	if len(result.Actions) == 0 {
		_, _ = fmt.Fprintln(out, "No changes.")
		return
	}
	for _, action := range result.Actions {
		if action.Reason != "" {
			_, _ = fmt.Fprintf(out, "%-8s %-20s %-5s %s (%s)\n", action.Op, action.Skill, action.Target, action.Source, action.Reason)
		} else {
			_, _ = fmt.Fprintf(out, "%-8s %-20s %-5s %s\n", action.Op, action.Skill, action.Target, action.Source)
		}
	}
}

func printWarnings(err io.Writer, warnings []skill.Finding) {
	for _, warning := range warnings {
		if warning.Path != "" {
			_, _ = fmt.Fprintf(err, "warning: %s: %s (%s)\n", warning.Code, warning.Message, warning.Path)
		} else {
			_, _ = fmt.Fprintf(err, "warning: %s: %s\n", warning.Code, warning.Message)
		}
	}
}

func sourceFilter(lock state.Lock, input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if _, ok := lock.Sources[input]; ok {
		return input, nil
	}
	for id, src := range lock.Sources {
		if src.Input == input || src.URL == input {
			return id, nil
		}
		if strings.HasPrefix(input, "github:") {
			ref, err := gitexec.Normalize(input, "")
			if err == nil && src.URL == ref.URL {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("source %q is not in the lockfile", input)
}

func validateTarget(ctx *Context, target string) error {
	if target == "" {
		return nil
	}
	if err := ctx.ensureRuntime(); err != nil {
		return err
	}
	if _, ok := ctx.Agents.Agents[target]; ok {
		if _, err := ctx.Agents.Root(target); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("unknown target %q; configured targets: %s", target, strings.Join(ctx.Agents.TargetNames(), ", "))
}

func validateTargets(ctx *Context, targets []string) error {
	for _, target := range targets {
		if err := validateTarget(ctx, target); err != nil {
			return err
		}
	}
	return nil
}

func defaultTargets(ctx *Context, targets []string) ([]string, error) {
	if len(targets) > 0 {
		return targets, validateTargets(ctx, targets)
	}
	if err := ctx.ensureRuntime(); err != nil {
		return nil, err
	}
	enabled := ctx.Agents.EnabledTargets()
	if len(enabled) == 0 {
		return nil, fmt.Errorf("no enabled agents configured")
	}
	return enabled, nil
}
