package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/skill"
	syncer "github.com/lox/skillyard/internal/sync"
)

type DiscoverCmd struct {
	Source    string `arg:"" help:"Git source or local path to inspect."`
	Ref       string `name:"ref" help:"Git branch, tag, or commit to inspect."`
	FullDepth bool   `name:"full-depth" help:"Search all subdirectories for SKILL.md files."`
	JSON      bool   `name:"json" help:"Emit machine-readable JSON."`
}

type discoverOutput struct {
	Source   discoverSourceOutput  `json:"source"`
	Skills   []discoverSkillOutput `json:"skills"`
	Warnings []skill.Finding       `json:"warnings,omitempty"`
}

type discoverSourceOutput struct {
	ID     string `json:"id"`
	Input  string `json:"input"`
	Type   string `json:"type"`
	URL    string `json:"url,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Root   string `json:"root"`
	Commit string `json:"commit,omitempty"`
}

type discoverSkillOutput struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Path        string          `json:"path"`
	RelPath     string          `json:"rel_path"`
	Installable bool            `json:"installable"`
	Findings    []skill.Finding `json:"findings,omitempty"`
	Warnings    []skill.Finding `json:"warnings,omitempty"`
}

func (c DiscoverCmd) Run(ctx *Context) error {
	if err := ctx.ensurePaths(); err != nil {
		return err
	}
	ref, err := gitexec.NormalizeWithRef(c.Source, "", c.Ref)
	if err != nil {
		return err
	}
	result, err := (syncer.Reconciler{
		Paths: ctx.Paths,
		Git:   ctx.Git,
	}).Discover(ref, syncer.DiscoverOptions{FullDepth: c.FullDepth})
	if err != nil {
		return err
	}
	out := discoverResultOutput(result)
	if c.JSON {
		return writeJSON(ctx.Out, out)
	}
	printDiscover(ctx, out)
	return nil
}

func discoverResultOutput(result syncer.DiscoveryResult) discoverOutput {
	out := discoverOutput{
		Source: discoverSourceOutput{
			ID:     result.Source.ID,
			Input:  result.Source.State.Input,
			Type:   result.Source.Type,
			URL:    result.Source.State.URL,
			Ref:    result.Source.State.Ref,
			Root:   result.Source.Root,
			Commit: result.Source.Commit,
		},
		Skills:   make([]discoverSkillOutput, 0, len(result.Skills)),
		Warnings: result.Warnings,
	}
	for _, inspection := range result.Skills {
		s := inspection.Skill
		name := s.Name
		if name == "" {
			name = filepath.Base(s.Path)
		}
		out.Skills = append(out.Skills, discoverSkillOutput{
			Name:        name,
			Description: s.Description,
			Path:        s.Path,
			RelPath:     s.RelPath,
			Installable: len(inspection.Findings) == 0,
			Findings:    inspection.Findings,
			Warnings:    s.Warnings,
		})
	}
	return out
}

func printDiscover(ctx *Context, out discoverOutput) {
	styles := newOutputStyles(ctx.Out)
	sourceRows := [][]string{{
		out.Source.ID,
		out.Source.Type,
		dashIfEmpty(out.Source.Ref),
		shortCommit(dashIfEmpty(out.Source.Commit)),
		dashIfEmpty(out.Source.URL),
		out.Source.Root,
	}}
	renderSectionTable(ctx.Out, styles, "Source", []string{"ID", "TYPE", "REF", "COMMIT", "URL", "ROOT"}, sourceRows, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 0, 4:
			return styles.info
		case 2, 3:
			return mutedIfDash(styles, value)
		case 5:
			return styles.muted
		default:
			return styles.cell
		}
	})
	_, _ = fmt.Fprintln(ctx.Out)

	rows := make([][]string, 0, len(out.Skills))
	for _, s := range out.Skills {
		rows = append(rows, []string{
			s.Name,
			boolText(s.Installable),
			s.RelPath,
			findingCodes(s.Findings),
			findingCodes(s.Warnings),
			s.Description,
		})
	}
	renderSectionTable(ctx.Out, styles, "Skills", []string{"NAME", "INSTALLABLE", "PATH", "FINDINGS", "WARNINGS", "DESCRIPTION"}, rows, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 1:
			return boolStyle(styles, value)
		case 2:
			return styles.muted
		case 3:
			return mutedIfDash(styles, value)
		case 4:
			if value == "-" {
				return styles.muted
			}
			return styles.warn
		default:
			return styles.cell
		}
	})
}

func findingCodes(findings []skill.Finding) string {
	if len(findings) == 0 {
		return "-"
	}
	codes := make([]string, 0, len(findings))
	seen := map[string]bool{}
	for _, finding := range findings {
		if seen[finding.Code] {
			continue
		}
		seen[finding.Code] = true
		codes = append(codes, finding.Code)
	}
	return strings.Join(codes, ", ")
}

func shortCommit(value string) string {
	if value == "-" || len(value) <= 12 {
		return value
	}
	return value[:12]
}
