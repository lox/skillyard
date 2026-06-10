package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/lox/skillyard/internal/cmd"
)

var version = "dev"

func main() {
	cli := cmd.CLI{}
	ctx := cmd.NewContext(os.Stdout, os.Stderr)
	parser := kong.Parse(&cli,
		kong.Name("skillyard"),
		kong.Description("Manage global Codex and Amp skills from Git sources."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)
	if err := parser.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "skillyard: %v\n", err)
		os.Exit(1)
	}
}
