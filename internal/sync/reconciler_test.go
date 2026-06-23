package syncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/materialize"
	"github.com/lox/skillyard/internal/state"
)

func TestSubscribeSyncUnsubscribeAndUnlink(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Valid skill")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	lock := state.NewLock()
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := env.Subscribe(lock, ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Subscriptions) != 1 {
		t.Fatalf("subscriptions=%d, want 1", len(next.Subscriptions))
	}
	if len(next.Installs) != 1 {
		t.Fatalf("installs=%d, want 1", len(next.Installs))
	}
	if status := materialize.Check(next.Installs[0], "git"); status != materialize.StatusLinked {
		t.Fatalf("status=%s, want linked", status)
	}

	again, _, err := env.Subscribe(next, ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Installs) != 1 {
		t.Fatalf("idempotent installs=%d, want 1", len(again.Installs))
	}

	unsub, _, err := env.Unsubscribe(again, "valid", []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(unsub.Installs) != 0 {
		t.Fatalf("installs after unsubscribe=%d, want 0", len(unsub.Installs))
	}
	if _, err := os.Lstat(filepath.Join(rootFor(t, env, agent.Codex), "valid")); !os.IsNotExist(err) {
		t.Fatalf("link after unsubscribe exists or errored: %v", err)
	}

	next, _, err = env.Subscribe(lock, ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	unlinked, _, err := env.Unlink(next, "valid", []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(unlinked.Subscriptions) != 1 {
		t.Fatalf("unlink removed subscription")
	}
	if len(unlinked.Installs) != 0 {
		t.Fatalf("installs after unlink=%d, want 0", len(unlinked.Installs))
	}
}

func TestSubscribeDoesNotResolveUnrelatedSources(t *testing.T) {
	env := testEnv(t)
	missing := filepath.Join(t.TempDir(), "missing")
	lock := state.NewLock()
	lock.Sources["broken"] = state.Source{
		Input:        missing,
		Type:         "local",
		InputPath:    missing,
		ResolvedPath: missing,
	}
	lock.Subscriptions = []state.Subscription{{
		Source: "broken",
		Target: agent.Codex,
		Selection: state.Selection{
			Include: []string{"missing"},
			Exclude: []string{},
		},
	}}

	source := t.TempDir()
	writeSkill(t, source, "valid", "Valid")
	ref, err := gitexec.Normalize(source, "")
	if err != nil {
		t.Fatal(err)
	}
	next, result, err := env.Subscribe(lock, ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.Sources["broken"]; !ok {
		t.Fatalf("unrelated source was dropped: %+v", next.Sources)
	}
	if len(next.Installs) != 1 || next.Installs[0].Skill != "valid" {
		t.Fatalf("installs=%+v, want only new source install", next.Installs)
	}
	if len(result.Actions) != 1 || result.Actions[0].Op != "link" {
		t.Fatalf("actions=%+v, want scoped link only", result.Actions)
	}
}

func TestSubscribeRetargetsExistingSkillFromOtherSource(t *testing.T) {
	env := testEnv(t)
	first := t.TempDir()
	writeSkill(t, first, "valid", "Valid")
	firstRef, err := gitexec.Normalize(first, "first")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), firstRef, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	second := t.TempDir()
	writeSkill(t, second, "valid", "Valid")
	secondRef, err := gitexec.Normalize(second, "second")
	if err != nil {
		t.Fatal(err)
	}
	next, result, err := env.Subscribe(lock, secondRef, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Installs) != 1 || next.Installs[0].Source != "second" {
		t.Fatalf("installs=%+v, want single retargeted install", next.Installs)
	}
	if len(result.Actions) != 1 || result.Actions[0].Op != "retarget" {
		t.Fatalf("actions=%+v, want retarget", result.Actions)
	}
}

func TestSubscribeDefaultsIncludeForSingleSkillSource(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "only", "Only")

	env := testEnv(t)
	ref, err := gitexec.Normalize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, result, err := env.Subscribe(state.NewLock(), ref, state.Selection{}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Subscriptions) != 1 {
		t.Fatalf("subscriptions=%d, want 1", len(lock.Subscriptions))
	}
	if got := lock.Subscriptions[0].Selection.Include; len(got) != 1 || got[0] != "only" {
		t.Fatalf("include=%+v, want only", got)
	}
	if len(lock.Installs) != 1 || lock.Installs[0].Skill != "only" {
		t.Fatalf("installs=%+v, want only", lock.Installs)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings=%+v, want none", result.Warnings)
	}
}

func TestSubscribeWithoutIncludeErrorsForMultiSkillSource(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "one", "One")
	writeSkill(t, root, "two", "Two")

	env := testEnv(t)
	ref, err := gitexec.Normalize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = env.Subscribe(state.NewLock(), ref, state.Selection{}, []string{agent.Codex}, Options{})
	if err == nil {
		t.Fatal("expected multi-skill include error")
	}
	if !strings.Contains(err.Error(), "--include '*'") {
		t.Fatalf("error=%q, want include guidance", err.Error())
	}
}

func TestGitDryRunCreatesCacheDirectoryOnly(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Valid")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	next, result, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Installs) != 1 || len(result.Actions) != 1 {
		t.Fatalf("dry-run next=%+v actions=%+v", next.Installs, result.Actions)
	}
	if _, err := os.Stat(env.Paths.CacheDir); err != nil {
		t.Fatalf("cache dir missing: %v", err)
	}
	if _, err := os.Stat(env.Paths.SourcesDir); !os.IsNotExist(err) {
		t.Fatalf("sources dir exists or errored after dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootFor(t, env, agent.Codex), "valid")); !os.IsNotExist(err) {
		t.Fatalf("target link exists or errored after dry-run: %v", err)
	}
}

func TestBroadSubscriptionSyncAddsAndRemovesSkills(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "one", "One")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "one")

	env := testEnv(t)
	lock := state.NewLock()
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err = env.Subscribe(lock, ref, state.Selection{Include: []string{"*"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Installs) != 1 {
		t.Fatalf("installs=%d, want 1", len(lock.Installs))
	}

	writeSkill(t, repo, "two", "Two")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "two")
	lock, _, err = env.Sync(lock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Installs) != 2 {
		t.Fatalf("installs after add=%d, want 2", len(lock.Installs))
	}
	if _, err := os.Stat(filepath.Join(rootFor(t, env, agent.Codex), "two", "SKILL.md")); err != nil {
		t.Fatalf("new skill link missing: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(repo, "one")); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "remove-one")
	lock, _, err = env.Sync(lock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Installs) != 1 || lock.Installs[0].Skill != "two" {
		t.Fatalf("installs after remove=%v, want only two", lock.Installs)
	}
	if _, err := os.Lstat(filepath.Join(rootFor(t, env, agent.Codex), "one")); !os.IsNotExist(err) {
		t.Fatalf("removed skill link exists or errored: %v", err)
	}
}

func TestSyncReportsGitCommitUpdates(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Valid")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	fromCommit := installFor(t, lock, agent.Codex, "valid").SourceCommit

	writeSkill(t, repo, "valid", "Valid changed")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "change-valid")

	lock, result, err := env.Sync(lock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	toCommit := installFor(t, lock, agent.Codex, "valid").SourceCommit
	if fromCommit == toCommit {
		t.Fatalf("commit did not change: %s", fromCommit)
	}

	sourceUpdate := actionFor(t, result.Actions, "source-update", "")
	if sourceUpdate.Source != ref.ID || sourceUpdate.From != fromCommit || sourceUpdate.To != toCommit {
		t.Fatalf("source update action=%+v, want source %s from %s to %s", sourceUpdate, ref.ID, fromCommit, toCommit)
	}
	retarget := actionFor(t, result.Actions, "retarget", "valid")
	if retarget.From != fromCommit || retarget.To != toCommit || retarget.Reason != "source commit changed" {
		t.Fatalf("retarget action=%+v, want commit movement", retarget)
	}
}

func TestSubscribeWithGitRefTracksPinnedBranch(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Pinned")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "pinned")
	git(t, repo, "branch", "pinned")
	pinnedCommit := gitOutput(t, repo, "rev-parse", "pinned")

	writeSkill(t, repo, "valid", "Default branch")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "default-branch")
	defaultCommit := gitOutput(t, repo, "rev-parse", "HEAD")
	if pinnedCommit == defaultCommit {
		t.Fatal("test setup did not create distinct commits")
	}

	env := testEnv(t)
	ref, err := gitexec.NormalizeWithRef("file://"+repo, "", "pinned")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := installFor(t, lock, agent.Codex, "valid").SourceCommit; got != pinnedCommit {
		t.Fatalf("installed commit=%s, want pinned branch commit %s", got, pinnedCommit)
	}
	if got := lock.Sources[ref.ID].Ref; got != "pinned" {
		t.Fatalf("source ref=%q, want pinned", got)
	}

	git(t, repo, "checkout", "pinned")
	writeSkill(t, repo, "valid", "Pinned changed")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "pinned-changed")
	nextPinnedCommit := gitOutput(t, repo, "rev-parse", "pinned")

	lock, _, err = env.Sync(lock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := installFor(t, lock, agent.Codex, "valid").SourceCommit; got != nextPinnedCommit {
		t.Fatalf("synced commit=%s, want advanced pinned branch commit %s", got, nextPinnedCommit)
	}
}

func TestSubscribeExcludeFiltersSelectedSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "one", "One")
	writeSkill(t, root, "two", "Two")

	env := testEnv(t)
	ref, err := gitexec.Normalize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"*"}, Exclude: []string{"two"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Installs) != 1 || lock.Installs[0].Skill != "one" {
		t.Fatalf("installs=%+v, want only one", lock.Installs)
	}
	if _, err := os.Lstat(filepath.Join(rootFor(t, env, agent.Codex), "two")); !os.IsNotExist(err) {
		t.Fatalf("excluded link exists or errored: %v", err)
	}
}

func TestUnsubscribeBroadSubscriptionAddsExclude(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "one", "One")
	writeSkill(t, root, "two", "Two")

	env := testEnv(t)
	ref, err := gitexec.Normalize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"*"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err = env.Unsubscribe(lock, "two", []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Subscriptions) != 1 {
		t.Fatalf("subscriptions=%d, want 1", len(lock.Subscriptions))
	}
	if !matchesAny(lock.Subscriptions[0].Selection.Exclude, "two") {
		t.Fatalf("exclude=%+v, want two", lock.Subscriptions[0].Selection.Exclude)
	}
	if len(lock.Installs) != 1 || lock.Installs[0].Skill != "one" {
		t.Fatalf("installs=%+v, want only one", lock.Installs)
	}
	if _, err := os.Lstat(filepath.Join(rootFor(t, env, agent.Codex), "two")); !os.IsNotExist(err) {
		t.Fatalf("unsubscribed link exists or errored: %v", err)
	}
}

func TestSyncRepairsMissingManagedLink(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Valid")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(rootFor(t, env, agent.Codex), "valid")
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}

	lock, result, err := env.Sync(lock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 1 || result.Actions[0].Op != "repair" {
		t.Fatalf("actions=%+v, want one repair", result.Actions)
	}
	if status := materialize.Check(lock.Installs[0], "git"); status != materialize.StatusLinked {
		t.Fatalf("status=%s, want linked", status)
	}
}

func TestSyncPreflightBlocksPartialMutation(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "one", "One")
	writeSkill(t, repo, "two", "Two")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"*"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(repo, "one")); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, repo, "three", "Three")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "replace-one")
	if err := os.WriteFile(filepath.Join(rootFor(t, env, agent.Codex), "three"), []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}

	next, _, err := env.Sync(lock, Options{})
	if err == nil {
		t.Fatal("expected unmanaged conflict")
	}
	if len(next.Installs) != len(lock.Installs) {
		t.Fatalf("returned lock changed on failure")
	}
	if _, err := os.Stat(filepath.Join(rootFor(t, env, agent.Codex), "one", "SKILL.md")); err != nil {
		t.Fatalf("existing managed link was removed before preflight failure: %v", err)
	}
}

func TestTargetFilteredSyncKeepsOtherTargetCommit(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "shared", "Shared")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	lock := state.NewLock()
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err = env.Subscribe(lock, ref, state.Selection{Include: []string{"shared"}}, []string{agent.Codex, agent.Amp}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	codexCommit := installFor(t, lock, agent.Codex, "shared").SourceCommit

	writeSkill(t, repo, "shared", "Shared changed")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "change-shared")

	lock, _, err = env.Sync(lock, Options{Target: agent.Amp})
	if err != nil {
		t.Fatal(err)
	}
	if got := installFor(t, lock, agent.Codex, "shared").SourceCommit; got != codexCommit {
		t.Fatalf("codex commit changed on amp-only sync: got %s want %s", got, codexCommit)
	}
	if got := installFor(t, lock, agent.Amp, "shared").SourceCommit; got == codexCommit {
		t.Fatalf("amp commit did not update")
	}
}

func TestTargetFilteredSyncSkipsOtherTargetSources(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "amp-skill", "Amp")

	env := testEnv(t)
	ref, err := gitexec.Normalize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	lock := state.NewLock()
	state.UpsertSource(&lock, "broken-codex", state.Source{
		Input: "file:///does/not/exist",
		Type:  "git",
		URL:   "file:///does/not/exist",
	})
	state.UpsertSubscription(&lock, state.Subscription{
		Source:    "broken-codex",
		Target:    agent.Codex,
		Selection: state.Selection{Include: []string{"*"}},
	})
	state.UpsertSource(&lock, ref.ID, state.Source{
		Input:        ref.Input,
		Type:         "local",
		InputPath:    ref.Input,
		ResolvedPath: ref.Path,
	})
	state.UpsertSubscription(&lock, state.Subscription{
		Source:    ref.ID,
		Target:    agent.Amp,
		Selection: state.Selection{Include: []string{"*"}},
	})

	lock, _, err = env.Sync(lock, Options{Target: agent.Amp})
	if err != nil {
		t.Fatal(err)
	}
	if got := installFor(t, lock, agent.Amp, "amp-skill").Skill; got != "amp-skill" {
		t.Fatalf("installed %q", got)
	}
}

func TestUnmanagedConflictBlocksSubscribe(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Valid")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	if err := os.MkdirAll(rootFor(t, env, agent.Codex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFor(t, env, agent.Codex), "valid"), []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err == nil {
		t.Fatalf("expected unmanaged conflict")
	}
}

func TestForceReplacesUnmanagedSymlink(t *testing.T) {
	root := t.TempDir()
	oldSource := filepath.Join(root, "old")
	newSource := filepath.Join(root, "new")
	writeSkill(t, oldSource, "valid", "Old")
	writeSkill(t, newSource, "valid", "New")

	env := testEnv(t)
	if err := os.MkdirAll(rootFor(t, env, agent.Codex), 0o755); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(rootFor(t, env, agent.Codex), "valid")
	if err := os.Symlink(filepath.Join(oldSource, "valid"), linkPath); err != nil {
		t.Fatal(err)
	}
	ref, err := gitexec.Normalize(newSource, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(target) != filepath.Join(newSource, "valid") {
		t.Fatalf("link target=%q, want new source", target)
	}
	if len(lock.Installs) != 1 {
		t.Fatalf("installs=%d, want 1", len(lock.Installs))
	}
}

func TestUnmanagedSymlinkBlocksSubscribeWithoutForce(t *testing.T) {
	root := t.TempDir()
	oldSource := filepath.Join(root, "old")
	newSource := filepath.Join(root, "new")
	writeSkill(t, oldSource, "valid", "Old")
	writeSkill(t, newSource, "valid", "New")

	env := testEnv(t)
	if err := os.MkdirAll(rootFor(t, env, agent.Codex), 0o755); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(rootFor(t, env, agent.Codex), "valid")
	if err := os.Symlink(filepath.Join(oldSource, "valid"), linkPath); err != nil {
		t.Fatal(err)
	}
	ref, err := gitexec.Normalize(newSource, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err == nil {
		t.Fatal("expected unmanaged symlink conflict")
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(target) != filepath.Join(oldSource, "valid") {
		t.Fatalf("link target=%q, want old source", target)
	}
}

func TestForceDoesNotReplaceUnmanagedFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeSkill(t, source, "valid", "Valid")

	env := testEnv(t)
	if err := os.MkdirAll(rootFor(t, env, agent.Codex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFor(t, env, agent.Codex), "valid"), []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := gitexec.Normalize(source, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{Force: true})
	if err == nil {
		t.Fatal("expected unmanaged file conflict")
	}
}

func TestDryRunPreflightBlocksUnmanagedConflict(t *testing.T) {
	repo := makeGitRepo(t)
	writeSkill(t, repo, "valid", "Valid")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	env := testEnv(t)
	if err := os.MkdirAll(rootFor(t, env, agent.Codex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFor(t, env, agent.Codex), "valid"), []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := gitexec.Normalize("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{DryRun: true})
	if err == nil {
		t.Fatalf("expected dry-run unmanaged conflict")
	}
}

func TestLocalSourceIsMutable(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "valid", "Valid")
	env := testEnv(t)
	ref, err := gitexec.Normalize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := env.Subscribe(state.NewLock(), ref, state.Selection{Include: []string{"valid"}}, []string{agent.Codex}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Installs) != 1 {
		t.Fatalf("installs=%d, want 1", len(lock.Installs))
	}
	if lock.Installs[0].SnapshotPath != "" || lock.Installs[0].SourceCommit != "" {
		t.Fatalf("local install recorded immutable source fields: %+v", lock.Installs[0])
	}
	if status := materialize.Check(lock.Installs[0], "local"); status != materialize.StatusMutableSource {
		t.Fatalf("status=%s, want mutable-source", status)
	}
}

func testEnv(t *testing.T) Reconciler {
	t.Helper()
	root := t.TempDir()
	agents, err := agent.NewRegistry([]agent.Agent{
		{Name: agent.Codex, Enabled: true, SkillsDir: filepath.Join(root, "codex"), SkillsDirExpr: "${CODEX_HOME:-~/.codex}/skills"},
		{Name: agent.Amp, Enabled: true, SkillsDir: filepath.Join(root, "amp"), SkillsDirExpr: "~/.config/agents/skills"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Reconciler{
		Paths: state.Paths{
			ConfigDir:  filepath.Join(root, "config"),
			DataDir:    filepath.Join(root, "data"),
			CacheDir:   filepath.Join(root, "cache"),
			SourcesDir: filepath.Join(root, "data", "sources"),
			LockPath:   filepath.Join(root, "config", "skillyard.lock.json"),
		},
		Agents: agents,
		Git:    gitexec.New(),
	}
}

func rootFor(t *testing.T, env Reconciler, target string) string {
	t.Helper()
	root, err := env.Agents.Root(target)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func makeGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test User")
	return repo
}

func writeSkill(t *testing.T, root, name, description string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func installFor(t *testing.T, lock state.Lock, target, name string) state.Install {
	t.Helper()
	for _, install := range lock.Installs {
		if install.Target == target && install.Skill == name {
			return install
		}
	}
	t.Fatalf("install %s/%s not found in %+v", target, name, lock.Installs)
	return state.Install{}
}

func actionFor(t *testing.T, actions []Action, op, skill string) Action {
	t.Helper()
	for _, action := range actions {
		if action.Op == op && action.Skill == skill {
			return action
		}
	}
	t.Fatalf("action %s/%s not found in %+v", op, skill, actions)
	return Action{}
}
