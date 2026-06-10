package cmd

import (
	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/state"
	syncer "github.com/lox/skillyard/internal/sync"
)

type SubscribeCmd struct {
	Source  string   `arg:"" help:"Git source or local path to subscribe to."`
	Include []string `name:"include" help:"Skill name or glob pattern to include. Defaults to the only skill when the source has exactly one."`
	Exclude []string `name:"exclude" help:"Skill name or glob pattern to exclude after includes."`
	Target  []string `name:"target" help:"Target agent to install into. Defaults to all enabled configured agents."`
	Name    string   `name:"name" help:"Override the generated source id."`
	Force   bool     `name:"force" help:"Replace unmanaged symlinks and drifted managed links."`
	DryRun  bool     `name:"dry-run" help:"Show the plan without changing links or lockfile."`
	JSON    bool     `name:"json" help:"Emit machine-readable JSON."`
}

func (c SubscribeCmd) Run(ctx *Context) error {
	targets, err := defaultTargets(ctx, c.Target)
	if err != nil {
		return err
	}
	lock, err := loadLock(ctx)
	if err != nil {
		return err
	}
	ref, err := gitexec.Normalize(c.Source, c.Name)
	if err != nil {
		return err
	}
	reconciler, err := ctx.reconciler()
	if err != nil {
		return err
	}
	next, result, err := reconciler.Subscribe(lock, ref, state.Selection{
		Include: c.Include,
		Exclude: c.Exclude,
	}, targets, syncer.Options{DryRun: c.DryRun, Force: c.Force})
	if err != nil {
		return err
	}
	if !c.DryRun {
		if err := saveLock(ctx, next); err != nil {
			return err
		}
	}
	if c.JSON {
		return writeJSON(ctx.Out, result)
	}
	printWarnings(ctx.Err, result.Warnings)
	printActions(ctx.Out, result)
	return nil
}
