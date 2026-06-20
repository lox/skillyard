package syncer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/materialize"
	"github.com/lox/skillyard/internal/skill"
	"github.com/lox/skillyard/internal/state"
)

type Reconciler struct {
	Paths  state.Paths
	Agents agent.Registry
	Git    gitexec.Git
}

type Options struct {
	DryRun bool
	Force  bool
	Source string
	Target string
}

type Result struct {
	Actions  []Action        `json:"actions"`
	Warnings []skill.Finding `json:"warnings,omitempty"`
	Lock     state.Lock      `json:"lock,omitempty"`
}

type DiscoveryResult struct {
	Source   DiscoverySource    `json:"source"`
	Skills   []skill.Inspection `json:"skills"`
	Warnings []skill.Finding    `json:"warnings,omitempty"`
}

type DiscoverOptions struct {
	FullDepth bool
}

type DiscoverySource struct {
	ID       string       `json:"id"`
	State    state.Source `json:"state"`
	Root     string       `json:"root"`
	Snapshot string       `json:"snapshot,omitempty"`
	Commit   string       `json:"commit,omitempty"`
	Type     string       `json:"type"`
}

type Action struct {
	Op     string `json:"op"`
	Skill  string `json:"skill,omitempty"`
	Target string `json:"target,omitempty"`
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type resolvedSource struct {
	ID       string
	State    state.Source
	Root     string
	Snapshot string
	Commit   string
	Type     string
}

type desiredInstall struct {
	Install    state.Install
	SourceType string
	SourceRoot string
}

func (r Reconciler) Subscribe(lock state.Lock, ref gitexec.SourceRef, selection state.Selection, targets []string, opts Options) (state.Lock, Result, error) {
	next := cloneLock(lock)
	src, err := r.sourceState(ref, opts)
	if err != nil {
		return lock, Result{}, err
	}
	state.UpsertSource(&next, ref.ID, src.State)
	for _, target := range targets {
		state.UpsertSubscription(&next, state.Subscription{
			Source:    ref.ID,
			Target:    target,
			Selection: selection,
		})
	}
	return r.Reconcile(lock, next, opts)
}

func (r Reconciler) Discover(ref gitexec.SourceRef, opts DiscoverOptions) (DiscoveryResult, error) {
	src, err := r.sourceState(ref, Options{})
	if err != nil {
		return DiscoveryResult{}, err
	}
	lock := state.NewLock()
	state.UpsertSource(&lock, ref.ID, src.State)
	state.UpsertSubscription(&lock, state.Subscription{
		Source: ref.ID,
		Selection: state.Selection{
			Include: []string{"*"},
		},
	})
	resolved, warnings, err := r.resolveSources(lock, Options{DryRun: true})
	if err != nil {
		return DiscoveryResult{}, err
	}
	source, ok := resolved[ref.ID]
	if !ok {
		return DiscoveryResult{}, fmt.Errorf("source %s was not resolved", ref.ID)
	}
	skills, err := skill.InspectWithOptions(source.Root, skill.DiscoveryOptions{FullDepth: opts.FullDepth})
	if err != nil {
		return DiscoveryResult{}, err
	}
	return DiscoveryResult{
		Source:   DiscoverySource(source),
		Skills:   skills,
		Warnings: warnings,
	}, nil
}

func (r Reconciler) Sync(lock state.Lock, opts Options) (state.Lock, Result, error) {
	return r.Reconcile(lock, lock, opts)
}

func (r Reconciler) Unsubscribe(lock state.Lock, skillName string, targets []string, opts Options) (state.Lock, Result, error) {
	next := cloneLock(lock)
	targetSet := map[string]bool{}
	for _, target := range targets {
		targetSet[target] = true
	}
	for i := range next.Subscriptions {
		sub := &next.Subscriptions[i]
		if len(targetSet) > 0 && !targetSet[sub.Target] {
			continue
		}
		sub.Selection.Include = removeValue(sub.Selection.Include, skillName)
		if matchesAny(sub.Selection.Include, skillName) && !matchesAny(sub.Selection.Exclude, skillName) {
			sub.Selection.Exclude = append(sub.Selection.Exclude, skillName)
			sort.Strings(sub.Selection.Exclude)
		}
	}
	state.RemoveEmptySubscriptions(&next)
	return r.Reconcile(lock, next, opts)
}

func (r Reconciler) Unlink(lock state.Lock, skillName string, targets []string, opts Options) (state.Lock, Result, error) {
	next := cloneLock(lock)
	var actions []Action
	targetSet := map[string]bool{}
	for _, target := range targets {
		targetSet[target] = true
	}
	var removals []state.Install
	for _, install := range next.Installs {
		if install.Skill != skillName || len(targetSet) > 0 && !targetSet[install.Target] {
			continue
		}
		removals = append(removals, install)
	}
	if !opts.DryRun {
		for _, install := range removals {
			if err := materialize.CanUnlink(install, opts.Force); err != nil {
				return lock, Result{}, err
			}
		}
	}
	var kept []state.Install
	for _, install := range next.Installs {
		if install.Skill != skillName || len(targetSet) > 0 && !targetSet[install.Target] {
			kept = append(kept, install)
			continue
		}
		if opts.DryRun {
			actions = append(actions, Action{Op: "unlink", Skill: install.Skill, Target: install.Target, Source: install.Source, Path: install.LinkPathResolved})
			kept = append(kept, install)
			continue
		}
		if err := materialize.Unlink(install, opts.Force); err != nil {
			return lock, Result{}, err
		}
		actions = append(actions, Action{Op: "unlink", Skill: install.Skill, Target: install.Target, Source: install.Source, Path: install.LinkPathResolved})
	}
	next.Installs = kept
	return next, Result{Actions: actions, Lock: next}, nil
}

func (r Reconciler) Reconcile(oldLock, desiredLock state.Lock, opts Options) (state.Lock, Result, error) {
	resolved, warnings, err := r.resolveSources(desiredLock, opts)
	if err != nil {
		return oldLock, Result{}, err
	}
	desiredLock, err = applyDefaultSelections(desiredLock, resolved, opts)
	if err != nil {
		return oldLock, Result{}, err
	}
	desired, moreWarnings, err := r.desiredInstalls(desiredLock, resolved, opts)
	if err != nil {
		return oldLock, Result{}, err
	}
	warnings = append(warnings, moreWarnings...)

	desiredKey := map[string]desiredInstall{}
	for _, d := range desired {
		key := installKey(d.Install)
		if _, ok := desiredKey[key]; ok {
			return oldLock, Result{}, fmt.Errorf("duplicate desired install for %s on %s", d.Install.Skill, d.Install.Target)
		}
		desiredKey[key] = d
	}

	actions := sourceUpdateActions(oldLock, resolved)
	next := cloneLock(desiredLock)
	for id, src := range resolved {
		next.Sources[id] = src.State
	}
	var removals []state.Install
	for _, existing := range oldLock.Installs {
		if !inScope(existing, opts) {
			continue
		}
		if _, ok := desiredKey[installKey(existing)]; ok {
			continue
		}
		removals = append(removals, existing)
	}

	if err := preflight(removals, desired, oldLock, opts); err != nil {
		return oldLock, Result{}, err
	}

	next.Installs = nil
	for _, existing := range oldLock.Installs {
		if !inScope(existing, opts) {
			next.Installs = append(next.Installs, existing)
			continue
		}
		if _, ok := desiredKey[installKey(existing)]; ok {
			continue
		}
		if opts.DryRun {
			actions = append(actions, Action{Op: "unlink", Skill: existing.Skill, Target: existing.Target, Source: existing.Source, Path: existing.LinkPathResolved, Reason: "no longer desired"})
		} else if err := materialize.Unlink(existing, opts.Force); err != nil {
			return oldLock, Result{}, err
		} else {
			actions = append(actions, Action{Op: "unlink", Skill: existing.Skill, Target: existing.Target, Source: existing.Source, Path: existing.LinkPathResolved, Reason: "no longer desired"})
		}
	}

	for _, d := range desired {
		existing, hadExisting := findInstall(oldLock.Installs, installKey(d.Install))
		if hadExisting && sameInstall(existing, d.Install) && inScope(existing, opts) && installHealthy(d) {
			next.Installs = append(next.Installs, existing)
			actions = append(actions, Action{Op: "keep", Skill: existing.Skill, Target: existing.Target, Source: existing.Source, Path: existing.LinkPathResolved})
			continue
		}
		if opts.DryRun {
			op := "link"
			if hadExisting {
				op = "retarget"
			}
			action := Action{Op: op, Skill: d.Install.Skill, Target: d.Install.Target, Source: d.Install.Source, Path: d.Install.LinkPathResolved}
			if hadExisting {
				action = installUpdateAction(action, existing, d.Install)
			}
			next.Installs = append(next.Installs, d.Install)
			actions = append(actions, action)
			continue
		}
		sourcePath := sourcePathForInstall(d.Install)
		force := hadExisting || opts.Force
		link, err := materialize.Link(d.Install.TargetRootResolved, d.Install.Skill, sourcePath, force)
		if err != nil {
			return oldLock, Result{}, err
		}
		d.Install.LinkPathResolved = link
		next.Installs = append(next.Installs, d.Install)
		op := "link"
		if hadExisting {
			op = "retarget"
			if sameInstall(existing, d.Install) {
				op = "repair"
			}
		}
		action := Action{Op: op, Skill: d.Install.Skill, Target: d.Install.Target, Source: d.Install.Source, Path: d.Install.LinkPathResolved}
		if hadExisting {
			action = installUpdateAction(action, existing, d.Install)
		}
		actions = append(actions, action)
	}
	sortInstalls(next.Installs)
	return next, Result{Actions: actions, Warnings: warnings, Lock: next}, nil
}

func sourceUpdateActions(oldLock state.Lock, resolved map[string]resolvedSource) []Action {
	var actions []Action
	var sourceIDs []string
	for id, src := range resolved {
		if src.Type != "git" || src.Commit == "" {
			continue
		}
		previous := oldLock.Sources[id].LastSeenCommit
		if previous == "" || previous == src.Commit {
			continue
		}
		sourceIDs = append(sourceIDs, id)
	}
	sort.Strings(sourceIDs)
	for _, id := range sourceIDs {
		src := resolved[id]
		actions = append(actions, Action{
			Op:     "source-update",
			Source: id,
			From:   oldLock.Sources[id].LastSeenCommit,
			To:     src.Commit,
			Reason: "git commit changed",
		})
	}
	return actions
}

func installUpdateAction(action Action, existing, desired state.Install) Action {
	if existing.SourceCommit != "" && desired.SourceCommit != "" && existing.SourceCommit != desired.SourceCommit {
		action.From = existing.SourceCommit
		action.To = desired.SourceCommit
		action.Reason = "source commit changed"
	}
	return action
}

func preflight(removals []state.Install, desired []desiredInstall, oldLock state.Lock, opts Options) error {
	for _, existing := range removals {
		if err := materialize.CanUnlink(existing, opts.Force); err != nil {
			return err
		}
	}
	for _, d := range desired {
		existing, hadExisting := findInstall(oldLock.Installs, installKey(d.Install))
		if hadExisting && sameInstall(existing, d.Install) && inScope(existing, opts) && installHealthy(d) {
			continue
		}
		if hadExisting {
			if err := materialize.CanUnlink(existing, opts.Force); err != nil {
				return err
			}
		}
		if err := materialize.CanLink(d.Install.TargetRootResolved, d.Install.Skill, sourcePathForInstall(d.Install), hadExisting || opts.Force); err != nil {
			return err
		}
	}
	return nil
}

func installHealthy(d desiredInstall) bool {
	status := materialize.Check(d.Install, d.SourceType)
	return status == materialize.StatusLinked || status == materialize.StatusMutableSource
}

func applyDefaultSelections(lock state.Lock, sources map[string]resolvedSource, opts Options) (state.Lock, error) {
	next := cloneLock(lock)
	for i := range next.Subscriptions {
		sub := &next.Subscriptions[i]
		if opts.Target != "" && sub.Target != opts.Target {
			continue
		}
		if len(sub.Selection.Include) > 0 {
			continue
		}
		src, ok := sources[sub.Source]
		if !ok {
			continue
		}
		skills, err := skill.Discover(src.Root)
		if err != nil {
			return lock, err
		}
		if len(skills) == 0 {
			return lock, fmt.Errorf("source %s contains no skills; pass --include <skill> or --include '*'", sub.Source)
		}
		if len(skills) > 1 {
			return lock, fmt.Errorf("source %s contains multiple skills (%s); pass --include <skill> or --include '*'", sub.Source, skillNames(skills))
		}
		sub.Selection.Include = []string{skills[0].Name}
	}
	return next, nil
}

func skillNames(skills []skill.Skill) string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (r Reconciler) resolveSources(lock state.Lock, opts Options) (map[string]resolvedSource, []skill.Finding, error) {
	out := map[string]resolvedSource{}
	var warnings []skill.Finding
	for id, src := range lock.Sources {
		if opts.Source != "" && opts.Source != src.Input && opts.Source != id && opts.Source != src.URL {
			continue
		}
		if !sourceHasInScopeSubscription(lock, id, opts) {
			continue
		}
		ref := resolvedSource{ID: id, State: src, Type: src.Type}
		switch src.Type {
		case "local":
			root := src.ResolvedPath
			if root == "" {
				root = src.InputPath
			}
			ref.Root = root
		case "git":
			checkout := src.CheckoutPath
			if checkout == "" {
				checkout = filepath.Join(r.Paths.SourcesDir, id, "repo")
			}
			if opts.DryRun {
				if err := os.MkdirAll(r.Paths.CacheDir, 0o755); err != nil {
					return nil, nil, fmt.Errorf("create cache directory: %w", err)
				}
				temp, err := os.MkdirTemp(r.Paths.CacheDir, "source-*")
				if err != nil {
					return nil, nil, fmt.Errorf("create temporary source clone: %w", err)
				}
				checkout = filepath.Join(temp, "repo")
			}
			if err := r.Git.EnsureClone(src.URL, checkout); err != nil {
				return nil, nil, err
			}
			if !opts.DryRun {
				if err := r.Git.Fetch(checkout); err != nil {
					return nil, nil, err
				}
			}
			commit, err := r.Git.Head(checkout)
			if err != nil {
				return nil, nil, err
			}
			snapshot := filepath.Join(r.Paths.SourcesDir, id, "snapshots", commit)
			if !opts.DryRun {
				if err := r.Git.Snapshot(checkout, commit, snapshot); err != nil {
					return nil, nil, err
				}
				src.CheckoutPath = checkout
				src.LastSeenCommit = commit
			} else {
				snapshot = checkout
			}
			ref.State = src
			ref.Root = snapshot
			ref.Snapshot = snapshot
			ref.Commit = commit
		default:
			return nil, nil, fmt.Errorf("unsupported source type %q", src.Type)
		}
		out[id] = ref
		for _, s := range lock.Subscriptions {
			if s.Source == id {
				for _, inc := range s.Selection.Include {
					if inc == "" {
						warnings = append(warnings, skill.Finding{Code: "empty-include", Message: "empty include ignored"})
					}
				}
			}
		}
	}
	return out, warnings, nil
}

func sourceHasInScopeSubscription(lock state.Lock, sourceID string, opts Options) bool {
	for _, sub := range lock.Subscriptions {
		if sub.Source != sourceID {
			continue
		}
		if opts.Target != "" && sub.Target != opts.Target {
			continue
		}
		return true
	}
	return false
}

func (r Reconciler) desiredInstalls(lock state.Lock, sources map[string]resolvedSource, opts Options) ([]desiredInstall, []skill.Finding, error) {
	var desired []desiredInstall
	var warnings []skill.Finding
	for _, sub := range lock.Subscriptions {
		if opts.Target != "" && sub.Target != opts.Target {
			continue
		}
		src, ok := sources[sub.Source]
		if !ok {
			continue
		}
		targetRoot, err := r.Agents.Root(sub.Target)
		if err != nil {
			return nil, nil, err
		}
		targetExpr, err := r.Agents.RootExpression(sub.Target)
		if err != nil {
			return nil, nil, err
		}
		skills, err := skill.Discover(src.Root)
		if err != nil {
			return nil, nil, err
		}
		matchedExact := map[string]bool{}
		for _, s := range skills {
			if !matchesSelection(sub.Selection, s.Name) {
				continue
			}
			for _, finding := range skill.Validate(s) {
				return nil, nil, fmt.Errorf("%s: %s", finding.Code, finding.Message)
			}
			warnings = append(warnings, s.Warnings...)
			matchedExact[s.Name] = true
			relSource := s.RelPath
			install := state.Install{
				Skill:              s.Name,
				Source:             sub.Source,
				SourcePath:         relSource,
				SourceCommit:       src.Commit,
				SnapshotPath:       src.Snapshot,
				Target:             sub.Target,
				TargetRoot:         targetExpr,
				TargetRootResolved: targetRoot,
				LinkPath:           filepath.ToSlash(filepath.Join(targetExpr, s.Name)),
				LinkPathResolved:   filepath.Join(targetRoot, s.Name),
			}
			if src.Type == "local" {
				install.SourcePath = s.Path
				install.SnapshotPath = ""
			}
			desired = append(desired, desiredInstall{Install: install, SourceType: src.Type, SourceRoot: src.Root})
		}
		for _, inc := range sub.Selection.Include {
			if containsPatternMeta(inc) {
				continue
			}
			if !matchedExact[inc] && !matchesAny(sub.Selection.Exclude, inc) {
				return nil, nil, fmt.Errorf("include %q matched no skill in source %s", inc, sub.Source)
			}
		}
	}
	return desired, warnings, nil
}

func (r Reconciler) sourceState(ref gitexec.SourceRef, opts Options) (resolvedSource, error) {
	switch ref.Type {
	case "local":
		return resolvedSource{
			ID: ref.ID,
			State: state.Source{
				Input:        ref.Input,
				Type:         "local",
				InputPath:    ref.Input,
				ResolvedPath: ref.Path,
			},
			Root: ref.Path,
			Type: "local",
		}, nil
	case "git":
		checkout := filepath.Join(r.Paths.SourcesDir, ref.ID, "repo")
		return resolvedSource{
			ID: ref.ID,
			State: state.Source{
				Input:        ref.Input,
				Type:         "git",
				URL:          ref.URL,
				CheckoutPath: checkout,
			},
			Type: "git",
		}, nil
	default:
		return resolvedSource{}, fmt.Errorf("unsupported source type %q", ref.Type)
	}
}

func matchesSelection(sel state.Selection, name string) bool {
	return matchesAny(sel.Include, name) && !matchesAny(sel.Exclude, name)
}

func matchesAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		ok, err := filepath.Match(pattern, name)
		if err == nil && ok {
			return true
		}
		if pattern == name {
			return true
		}
	}
	return false
}

func containsPatternMeta(pattern string) bool {
	for _, ch := range pattern {
		if ch == '*' || ch == '?' || ch == '[' {
			return true
		}
	}
	return false
}

func removeValue(values []string, value string) []string {
	var out []string
	for _, v := range values {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}

func cloneLock(lock state.Lock) state.Lock {
	out := state.NewLock()
	out.Version = lock.Version
	for id, src := range lock.Sources {
		out.Sources[id] = src
	}
	out.Subscriptions = append(out.Subscriptions, lock.Subscriptions...)
	out.Installs = append(out.Installs, lock.Installs...)
	return out
}

func installKey(install state.Install) string {
	return install.Target + "\x00" + install.Skill
}

func findInstall(installs []state.Install, key string) (state.Install, bool) {
	for _, install := range installs {
		if installKey(install) == key {
			return install, true
		}
	}
	return state.Install{}, false
}

func sameInstall(a, b state.Install) bool {
	return a.Source == b.Source &&
		a.SourcePath == b.SourcePath &&
		a.SourceCommit == b.SourceCommit &&
		a.SnapshotPath == b.SnapshotPath &&
		a.TargetRootResolved == b.TargetRootResolved &&
		a.LinkPathResolved == b.LinkPathResolved
}

func inScope(install state.Install, opts Options) bool {
	if opts.Target != "" && install.Target != opts.Target {
		return false
	}
	if opts.Source != "" && install.Source != opts.Source {
		return false
	}
	return true
}

func sourcePathForInstall(install state.Install) string {
	if install.SnapshotPath != "" {
		return filepath.Join(install.SnapshotPath, filepath.FromSlash(install.SourcePath))
	}
	return install.SourcePath
}

func sortInstalls(installs []state.Install) {
	sort.Slice(installs, func(i, j int) bool {
		if installs[i].Target != installs[j].Target {
			return installs[i].Target < installs[j].Target
		}
		return installs[i].Skill < installs[j].Skill
	})
}
