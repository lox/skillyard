package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
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
	w := tabwriter.NewWriter(ctx.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "AGENTS")
	_, _ = fmt.Fprintln(w, "NAME\tENABLED\tSKILLS_DIR\tEXISTS")
	for _, a := range out.Agents {
		_, _ = fmt.Fprintf(w, "%s\t%t\t%s\t%t\n", a.Name, a.Enabled, a.SkillsDir, a.Exists)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Skillyard config dir:\t%s\t%t\n", out.ConfigDir, exists(out.ConfigDir))
	_, _ = fmt.Fprintf(w, "Skillyard config file:\t%s\t%t\n", out.ConfigPath, out.ConfigExists)
	_, _ = fmt.Fprintf(w, "Skillyard sources:\t%s\t%t\n", out.SourceDir, exists(out.SourceDir))
	_, _ = fmt.Fprintf(w, "Lockfile:\t%s\t%t\n", out.LockPath, out.LockExists)
	_, _ = fmt.Fprintf(w, "Git:\t%s\t\n", out.GitVersion)
	_, _ = fmt.Fprintf(w, "Managed installs:\t%d\t\n", out.ManagedInstallCount)
	return w.Flush()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
