package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	styles := newOutputStyles(out)
	if len(result.Actions) == 0 {
		_, _ = fmt.Fprintln(out, styles.muted.Render("No changes."))
		return
	}
	rows := make([][]string, 0, len(result.Actions))
	for _, action := range result.Actions {
		rows = append(rows, []string{
			action.Op,
			action.Skill,
			action.Target,
			action.Source,
			dashIfEmpty(action.Path),
			shortValue(action.From),
			shortValue(action.To),
			dashIfEmpty(action.Reason),
		})
	}
	renderSectionTable(out, styles, "Actions", []string{"OP", "SKILL", "TARGET", "SOURCE", "PATH", "FROM", "TO", "REASON"}, rows, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 0:
			return actionStyle(styles, value)
		case 3:
			return styles.info
		case 4, 5, 6, 7:
			return mutedIfDash(styles, value)
		default:
			return styles.cell
		}
	})
}

func printWarnings(err io.Writer, warnings []skill.Finding) {
	styles := newOutputStyles(err)
	for _, warning := range warnings {
		if warning.Path != "" {
			_, _ = fmt.Fprintf(err, "%s %s: %s (%s)\n", styles.warn.Render("warning:"), styles.warn.Render(warning.Code), warning.Message, styles.muted.Render(warning.Path))
		} else {
			_, _ = fmt.Fprintf(err, "%s %s: %s\n", styles.warn.Render("warning:"), styles.warn.Render(warning.Code), warning.Message)
		}
	}
}

func actionStyle(styles outputStyles, op string) lipgloss.Style {
	switch op {
	case "link", "repair":
		return styles.success
	case "keep":
		return styles.muted
	case "unlink":
		return styles.warn
	case "retarget", "source-update":
		return styles.accent
	default:
		return styles.cell
	}
}

func shortValue(value string) string {
	if value == "" {
		return "-"
	}
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func mutedIfDash(styles outputStyles, value string) lipgloss.Style {
	if value == "-" {
		return styles.muted
	}
	return styles.cell
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
