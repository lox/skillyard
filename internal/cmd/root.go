package cmd

import (
	"io"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/lox/skillyard/internal/agent"
	"github.com/lox/skillyard/internal/config"
	"github.com/lox/skillyard/internal/gitexec"
	"github.com/lox/skillyard/internal/state"
	syncer "github.com/lox/skillyard/internal/sync"
)

type CLI struct {
	Version     kong.VersionFlag `help:"Show version and exit."`
	Subscribe   SubscribeCmd     `cmd:"" help:"Subscribe a target to skills from a source."`
	Setup       SetupCmd         `cmd:"" help:"Create skillyard config and show detected agents."`
	Sync        SyncCmd          `cmd:"" help:"Reconcile subscriptions with installed skill links."`
	List        ListCmd          `cmd:"" help:"List subscriptions and installed skill links."`
	Unsubscribe UnsubscribeCmd   `cmd:"" help:"Stop managing matching skills."`
	Unlink      UnlinkCmd        `cmd:"" help:"Remove realized links without changing subscriptions."`
	Doctor      DoctorCmd        `cmd:"" help:"Report skillyard paths and state."`
}

type Context struct {
	Out    io.Writer
	Err    io.Writer
	Log    *log.Logger
	Paths  state.Paths
	Agents agent.Registry
	Git    gitexec.Git
}

func NewContext(out, err io.Writer) *Context {
	logger := log.New(err)
	logger.SetReportTimestamp(false)
	return &Context{
		Out: out,
		Err: err,
		Log: logger,
		Git: gitexec.New(),
	}
}

func (c *Context) ensureRuntime() error {
	if c.Paths.LockPath == "" {
		paths, err := state.DefaultPaths()
		if err != nil {
			return err
		}
		c.Paths = paths
	}
	if c.Agents.Agents == nil {
		agents, err := config.LoadAgents(c.Paths.ConfigPath)
		if err != nil {
			return err
		}
		c.Agents = agents
	}
	return nil
}

func (c *Context) reconciler() (syncer.Reconciler, error) {
	if err := c.ensureRuntime(); err != nil {
		return syncer.Reconciler{}, err
	}
	return syncer.Reconciler{
		Paths:  c.Paths,
		Agents: c.Agents,
		Git:    c.Git,
	}, nil
}
