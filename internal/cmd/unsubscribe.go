package cmd

import (
	"fmt"

	syncer "github.com/lox/skillyard/internal/sync"
)

type UnsubscribeCmd struct {
	Skill  string   `arg:"" help:"Skill name to unsubscribe."`
	Target []string `name:"target" help:"Target agent to unsubscribe from: codex or amp."`
	Force  bool     `name:"force" help:"Remove managed links even if they drifted."`
	DryRun bool     `name:"dry-run" help:"Show the plan without changing links or lockfile."`
	JSON   bool     `name:"json" help:"Emit machine-readable JSON."`
}

func (c UnsubscribeCmd) Run(ctx *Context) error {
	if c.Skill == "" {
		return fmt.Errorf("skill is required")
	}
	if err := validateTargets(ctx, c.Target); err != nil {
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
	next, result, err := reconciler.Unsubscribe(lock, c.Skill, c.Target, syncer.Options{DryRun: c.DryRun, Force: c.Force})
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
