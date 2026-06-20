package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/skill"
	syncer "github.com/lox/skillyard/internal/sync"
)

type UseCmd struct {
	Source  string   `arg:"" help:"Git source or local path to inspect."`
	Include []string `name:"include" help:"Skill name or glob pattern to use. Defaults to the only discovered skill when the source has exactly one."`
}

func (c UseCmd) Run(ctx *Context) error {
	if err := ctx.ensurePaths(); err != nil {
		return err
	}
	ref, err := gitexec.Normalize(c.Source, "")
	if err != nil {
		return err
	}
	result, err := (syncer.Reconciler{
		Paths: ctx.Paths,
		Git:   ctx.Git,
	}).Discover(ref, syncer.DiscoverOptions{})
	if err != nil {
		return err
	}
	selected, err := selectUseSkill(result.Skills, c.Include, result.Source.ID)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(selected.Skill.Path, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("read selected skill: %w", err)
	}
	_, err = ctx.Out.Write(data)
	return err
}

func selectUseSkill(inspections []skill.Inspection, includes []string, sourceID string) (skill.Inspection, error) {
	if len(inspections) == 0 {
		return skill.Inspection{}, fmt.Errorf("source %s contains no skills", sourceID)
	}
	var selected []skill.Inspection
	if len(includes) == 0 {
		if len(inspections) != 1 {
			return skill.Inspection{}, fmt.Errorf("source %s contains multiple skills (%s); pass --include <skill>", sourceID, inspectionNames(inspections))
		}
		selected = append(selected, inspections[0])
	} else {
		for _, inspection := range inspections {
			if matchesUseInclude(includes, inspection.Skill.Name) {
				selected = append(selected, inspection)
			}
		}
	}
	if len(selected) == 0 {
		return skill.Inspection{}, fmt.Errorf("include %q matched no skill in source %s", strings.Join(includes, ", "), sourceID)
	}
	if len(selected) > 1 {
		return skill.Inspection{}, fmt.Errorf("include matched multiple skills (%s); select exactly one skill", inspectionNames(selected))
	}
	if len(selected[0].Findings) > 0 {
		return skill.Inspection{}, fmt.Errorf("selected skill %s is not installable: %s", selected[0].Skill.Name, findingCodes(selected[0].Findings))
	}
	return selected[0], nil
}

func matchesUseInclude(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if ok, err := filepath.Match(pattern, name); err == nil && ok {
			return true
		}
		if pattern == name {
			return true
		}
	}
	return false
}

func inspectionNames(inspections []skill.Inspection) string {
	names := make([]string, 0, len(inspections))
	for _, inspection := range inspections {
		name := inspection.Skill.Name
		if name == "" {
			name = filepath.Base(inspection.Skill.Path)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
