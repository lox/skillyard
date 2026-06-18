package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type DoctorCmd struct {
	JSON bool `name:"json" help:"Emit machine-readable JSON."`
}

type doctorOutput struct {
	Agents              []doctorAgent `json:"agents"`
	ConfigDir           string        `json:"config_dir"`
	ConfigPath          string        `json:"config_path"`
	ConfigExists        bool          `json:"config_exists"`
	SourceDir           string        `json:"source_dir"`
	LockPath            string        `json:"lock_path"`
	LockExists          bool          `json:"lock_exists"`
	GitVersion          string        `json:"git_version"`
	ManagedInstallCount int           `json:"managed_install_count"`
}

type doctorAgent struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SkillsDir string `json:"skills_dir"`
	Exists    bool   `json:"exists"`
}

func (c DoctorCmd) Run(ctx *Context) error {
	if err := ctx.ensureRuntime(); err != nil {
		return err
	}
	lock, err := loadLock(ctx)
	if err != nil {
		return err
	}
	gitVersion, err := ctx.Git.Version()
	if err != nil {
		gitVersion = "unavailable: " + err.Error()
	}
	out := doctorOutput{
		Agents:              []doctorAgent{},
		ConfigDir:           ctx.Paths.ConfigDir,
		ConfigPath:          ctx.Paths.ConfigPath,
		ConfigExists:        exists(ctx.Paths.ConfigPath),
		SourceDir:           ctx.Paths.SourcesDir,
		LockPath:            ctx.Paths.LockPath,
		LockExists:          exists(ctx.Paths.LockPath),
		GitVersion:          gitVersion,
		ManagedInstallCount: len(lock.Installs),
	}
	for _, name := range ctx.Agents.TargetNames() {
		a := ctx.Agents.Agents[name]
		out.Agents = append(out.Agents, doctorAgent{
			Name:      a.Name,
			Enabled:   a.Enabled,
			SkillsDir: a.SkillsDir,
			Exists:    exists(a.SkillsDir),
		})
	}
	if c.JSON {
		return writeJSON(ctx.Out, out)
	}
	styles := newOutputStyles(ctx.Out)
	agentRows := make([][]string, 0, len(out.Agents))
	for _, a := range out.Agents {
		agentRows = append(agentRows, []string{a.Name, boolText(a.Enabled), a.SkillsDir, boolText(a.Exists)})
	}
	renderSectionTable(ctx.Out, styles, "Agents", []string{"NAME", "ENABLED", "SKILLS_DIR", "EXISTS"}, agentRows, func(_ int, col int, value string) lipgloss.Style {
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
	_, _ = fmt.Fprintln(ctx.Out)

	gitStatus := "ok"
	if out.GitVersion == "" || strings.HasPrefix(out.GitVersion, "unavailable:") {
		gitStatus = "unavailable"
	}
	pathRows := [][]string{
		{"Config dir", out.ConfigDir, boolText(exists(out.ConfigDir))},
		{"Config file", out.ConfigPath, boolText(out.ConfigExists)},
		{"Sources", out.SourceDir, boolText(exists(out.SourceDir))},
		{"Lockfile", out.LockPath, boolText(out.LockExists)},
		{"Git", out.GitVersion, gitStatus},
		{"Managed installs", strconv.Itoa(out.ManagedInstallCount), "-"},
	}
	renderSectionTable(ctx.Out, styles, "State", []string{"ITEM", "VALUE", "STATUS"}, pathRows, func(_ int, col int, value string) lipgloss.Style {
		switch col {
		case 0:
			return styles.cell
		case 1:
			return styles.muted
		case 2:
			switch value {
			case "yes", "ok":
				return styles.success
			case "no":
				return styles.warn
			case "unavailable":
				return styles.warn
			default:
				return mutedIfDash(styles, value)
			}
		default:
			return styles.cell
		}
	})
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
