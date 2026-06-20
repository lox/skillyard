package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/state"
)

func TestExportFiltersTargetAndOmitsInstalls(t *testing.T) {
	root := t.TempDir()
	ctx := commandTestContext(root)
	lock := state.NewLock()
	lock.Sources["git-source"] = state.Source{
		Input:          "github:lox/agent-skills",
		Type:           "git",
		URL:            "https://github.com/lox/agent-skills.git",
		CheckoutPath:   "/machine/specific/checkout",
		LastSeenCommit: "abc123",
	}
	lock.Sources["local-source"] = state.Source{Input: "./local", Type: "local", ResolvedPath: filepath.Join(root, "local")}
	lock.Subscriptions = []state.Subscription{
		{Source: "git-source", Target: agent.Codex, Selection: state.Selection{Include: []string{"*"}}},
		{Source: "local-source", Target: agent.Amp, Selection: state.Selection{Include: []string{"local"}}},
	}
	lock.Installs = []state.Install{{Skill: "old", Target: agent.Codex}}
	if err := state.Save(ctx.Paths.LockPath, lock); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (ExportCmd{Target: agent.Codex}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out desiredStateFile
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Subscriptions) != 1 || out.Subscriptions[0].Target != agent.Codex {
		t.Fatalf("subscriptions=%+v, want codex only", out.Subscriptions)
	}
	if len(out.Sources) != 1 {
		t.Fatalf("sources=%+v, want one referenced source", out.Sources)
	}
	src := out.Sources["git-source"]
	if src.CheckoutPath != "" || src.LastSeenCommit != "" {
		t.Fatalf("exported source kept machine state: %+v", src)
	}
	if bytes.Contains(stdout.Bytes(), []byte("installs")) {
		t.Fatalf("export included installs: %s", stdout.String())
	}
}

func TestApplyDryRunDoesNotSaveAndPlansLinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeCommandTestSkill(t, source, "valid")
	ctx := commandTestContext(root)
	desired := desiredStateFile{
		Version: state.Version,
		Sources: map[string]state.Source{
			"local-source": {Input: source, Type: "local", ResolvedPath: source},
		},
		Subscriptions: []state.Subscription{
			{Source: "local-source", Target: agent.Codex, Selection: state.Selection{Include: []string{"valid"}}},
		},
	}
	desiredPath := filepath.Join(root, "desired.json")
	data, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desiredPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	ctx.Out = &stdout
	if err := (ApplyCmd{File: desiredPath, Target: agent.Codex, DryRun: true, JSON: true}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ctx.Paths.LockPath); !os.IsNotExist(err) {
		t.Fatalf("lockfile exists or errored after dry-run apply: %v", err)
	}
	var result struct {
		Actions []struct {
			Op    string `json:"op"`
			Skill string `json:"skill"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 1 || result.Actions[0].Op != "link" || result.Actions[0].Skill != "valid" {
		t.Fatalf("actions=%+v, want dry-run link for valid", result.Actions)
	}
}

func TestApplyTargetDoesNotImportOtherTargetSources(t *testing.T) {
	current := state.NewLock()
	current.Sources["codex-source"] = state.Source{Input: "/current/codex", Type: "local", ResolvedPath: "/current/codex"}
	current.Sources["amp-source"] = state.Source{Input: "/current/amp", Type: "local", ResolvedPath: "/current/amp"}
	current.Subscriptions = []state.Subscription{
		{Source: "codex-source", Target: agent.Codex, Selection: state.Selection{Include: []string{"codex"}}},
		{Source: "amp-source", Target: agent.Amp, Selection: state.Selection{Include: []string{"amp"}}},
	}
	desired := desiredStateFile{
		Version: state.Version,
		Sources: map[string]state.Source{
			"codex-source": {Input: "/desired/codex", Type: "local", ResolvedPath: "/desired/codex"},
			"amp-source":   {Input: "/desired/amp", Type: "local", ResolvedPath: "/desired/amp"},
		},
		Subscriptions: []state.Subscription{
			{Source: "codex-source", Target: agent.Codex, Selection: state.Selection{Include: []string{"codex-new"}}},
			{Source: "amp-source", Target: agent.Amp, Selection: state.Selection{Include: []string{"amp-new"}}},
		},
	}

	next, err := lockWithDesiredState(current, desired, agent.Codex)
	if err != nil {
		t.Fatal(err)
	}
	if got := next.Sources["amp-source"].ResolvedPath; got != "/current/amp" {
		t.Fatalf("amp source resolved path=%q, want current value preserved", got)
	}
	if got := next.Sources["codex-source"].ResolvedPath; got != "/desired/codex" {
		t.Fatalf("codex source resolved path=%q, want desired value imported", got)
	}
	if len(next.Subscriptions) != 2 {
		t.Fatalf("subscriptions=%+v, want codex replaced and amp preserved", next.Subscriptions)
	}
	for _, sub := range next.Subscriptions {
		switch sub.Target {
		case agent.Codex:
			if len(sub.Selection.Include) != 1 || sub.Selection.Include[0] != "codex-new" {
				t.Fatalf("codex subscription=%+v, want desired codex subscription", sub)
			}
		case agent.Amp:
			if len(sub.Selection.Include) != 1 || sub.Selection.Include[0] != "amp" {
				t.Fatalf("amp subscription=%+v, want current amp subscription preserved", sub)
			}
		default:
			t.Fatalf("unexpected subscription target: %+v", sub)
		}
	}
}
