package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/lox/skillyard/internal/state"
	syncer "github.com/lox/skillyard/internal/sync"
)

type ExportCmd struct {
	Target string `name:"target" help:"Only export subscriptions for one target."`
}

type ApplyCmd struct {
	File   string `arg:"" help:"Desired-state JSON file to apply."`
	Target string `name:"target" help:"Only apply subscriptions for one target."`
	Force  bool   `name:"force" help:"Replace unmanaged symlinks and drifted managed links."`
	DryRun bool   `name:"dry-run" help:"Show the plan without changing links or lockfile."`
	JSON   bool   `name:"json" help:"Emit machine-readable JSON."`
}

type desiredStateFile struct {
	Version       int                     `json:"version"`
	Sources       map[string]state.Source `json:"sources"`
	Subscriptions []state.Subscription    `json:"subscriptions"`
}

func (c ExportCmd) Run(ctx *Context) error {
	if err := validateTarget(ctx, c.Target); err != nil {
		return err
	}
	lock, err := loadLock(ctx)
	if err != nil {
		return err
	}
	return writeJSON(ctx.Out, desiredStateFromLock(lock, c.Target))
}

func (c ApplyCmd) Run(ctx *Context) error {
	if err := validateTarget(ctx, c.Target); err != nil {
		return err
	}
	current, err := loadLock(ctx)
	if err != nil {
		return err
	}
	desiredFile, err := loadDesiredStateFile(c.File)
	if err != nil {
		return err
	}
	next, err := lockWithDesiredState(current, desiredFile, c.Target)
	if err != nil {
		return err
	}
	reconciler, err := ctx.reconciler()
	if err != nil {
		return err
	}
	applied, result, err := reconciler.Reconcile(current, next, syncer.Options{
		DryRun: c.DryRun,
		Force:  c.Force,
		Target: c.Target,
	})
	if err != nil {
		return err
	}
	if !c.DryRun {
		if err := saveLock(ctx, applied); err != nil {
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

func desiredStateFromLock(lock state.Lock, target string) desiredStateFile {
	out := desiredStateFile{
		Version:       state.Version,
		Sources:       map[string]state.Source{},
		Subscriptions: []state.Subscription{},
	}
	for _, sub := range lock.Subscriptions {
		if target != "" && sub.Target != target {
			continue
		}
		out.Subscriptions = append(out.Subscriptions, sub)
		if src, ok := lock.Sources[sub.Source]; ok {
			out.Sources[sub.Source] = portableSource(src)
		}
	}
	sort.Slice(out.Subscriptions, func(i, j int) bool {
		if out.Subscriptions[i].Target != out.Subscriptions[j].Target {
			return out.Subscriptions[i].Target < out.Subscriptions[j].Target
		}
		return out.Subscriptions[i].Source < out.Subscriptions[j].Source
	})
	return out
}

func portableSource(src state.Source) state.Source {
	if src.Type == "git" {
		return state.Source{
			Input: src.Input,
			Type:  src.Type,
			URL:   src.URL,
			Ref:   src.Ref,
		}
	}
	return src
}

func loadDesiredStateFile(path string) (desiredStateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return desiredStateFile{}, fmt.Errorf("read desired state %s: %w", path, err)
	}
	var desired desiredStateFile
	if err := json.Unmarshal(data, &desired); err != nil {
		return desiredStateFile{}, fmt.Errorf("parse desired state %s: %w", path, err)
	}
	if desired.Version == 0 {
		desired.Version = state.Version
	}
	if desired.Sources == nil {
		desired.Sources = map[string]state.Source{}
	}
	return desired, nil
}

func lockWithDesiredState(current state.Lock, desired desiredStateFile, target string) (state.Lock, error) {
	next := copyLockForDesiredState(current)
	if next.Version == 0 {
		next.Version = state.Version
	}
	if next.Sources == nil {
		next.Sources = map[string]state.Source{}
	}
	imported := subscriptionsForTarget(desired.Subscriptions, target)
	for id := range referencedSources(imported) {
		src, ok := desired.Sources[id]
		if !ok {
			continue
		}
		next.Sources[id] = src
	}
	next.Subscriptions = replaceSubscriptions(next.Subscriptions, imported, target)
	for _, sub := range next.Subscriptions {
		if _, ok := next.Sources[sub.Source]; !ok {
			return state.Lock{}, fmt.Errorf("subscription references unknown source %q", sub.Source)
		}
	}
	return next, nil
}

func copyLockForDesiredState(lock state.Lock) state.Lock {
	out := state.Lock{
		Version:       lock.Version,
		Sources:       map[string]state.Source{},
		Subscriptions: append([]state.Subscription{}, lock.Subscriptions...),
		Installs:      append([]state.Install{}, lock.Installs...),
	}
	for id, src := range lock.Sources {
		out.Sources[id] = src
	}
	return out
}

func subscriptionsForTarget(subscriptions []state.Subscription, target string) []state.Subscription {
	if target == "" {
		return append([]state.Subscription{}, subscriptions...)
	}
	var out []state.Subscription
	for _, sub := range subscriptions {
		if sub.Target == target {
			out = append(out, sub)
		}
	}
	return out
}

func referencedSources(subscriptions []state.Subscription) map[string]bool {
	out := map[string]bool{}
	for _, sub := range subscriptions {
		out[sub.Source] = true
	}
	return out
}

func replaceSubscriptions(existing, imported []state.Subscription, target string) []state.Subscription {
	var out []state.Subscription
	for _, sub := range existing {
		if target == "" || sub.Target == target {
			continue
		}
		out = append(out, sub)
	}
	out = append(out, imported...)
	return out
}
