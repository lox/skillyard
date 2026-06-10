package cmd

import (
	syncer "github.com/lox/skillyard/internal/sync"
)

type SyncCmd struct {
	Source string `arg:"" optional:"" help:"Optional source id, input, or URL to sync."`
	Target string `name:"target" help:"Only sync one target: codex or amp."`
	Force  bool   `name:"force" help:"Replace unmanaged symlinks and drifted managed links."`
	DryRun bool   `name:"dry-run" help:"Show the plan without changing links or lockfile."`
	JSON   bool   `name:"json" help:"Emit machine-readable JSON."`
}

func (c SyncCmd) Run(ctx *Context) error {
	if err := validateTarget(ctx, c.Target); err != nil {
		return err
	}
	lock, err := loadLock(ctx)
	if err != nil {
		return err
	}
	reconciler, err := ctx.reconciler()
	if err != nil {
		return err
	}
	source, err := sourceFilter(lock, c.Source)
	if err != nil {
		return err
	}
	next, result, err := reconciler.Sync(lock, syncer.Options{
		DryRun: c.DryRun,
		Force:  c.Force,
		Source: source,
		Target: c.Target,
	})
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
