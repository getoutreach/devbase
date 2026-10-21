// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: yq CLI subcommand.

package main

import (
	"context"

	"github.com/getoutreach/devbase/v2/internal/yq"
	"github.com/urfave/cli/v3"
)

// newYqCommand returns the "yq" command: a jq-compatible query engine
// over YAML (or JSON) input, replacing shell/yq.sh's gojq/python-yq
// subprocess wrapper.
//
// SkipFlagParsing is required because --arg/--argjson/--slurpfile each
// need two argv tokens, which no cli/v3 Flag type supports (see the
// design plan's "Flag parsing decision") -- devbase yq parses its own
// argv entirely via internal/yq.Main, which also means cli/v3 never
// auto-handles -h/--help for this subcommand; internal/yq's own flag
// parser decides that behavior instead (currently: -h/--help is
// simply unrecognized, like any other flag devbase yq does not
// define).
func newYqCommand() *cli.Command {
	return &cli.Command{
		Name:            "yq",
		Usage:           "Evaluate a jq-compatible filter against YAML (or JSON) input",
		ArgsUsage:       "<filter> [file...]",
		SkipFlagParsing: true,
		HideHelp:        true,
		Action:          runYq,
	}
}

// runYq is the Action for the "yq" subcommand. c.Args().Slice() is
// every token after "yq" on the command line, entirely unparsed by
// cli/v3 (see SkipFlagParsing above).
func runYq(ctx context.Context, c *cli.Command) error {
	exitCode := yq.Main(ctx, c.Args().Slice(), c.Reader, c.Writer, c.ErrWriter)
	if exitCode != 0 {
		return cli.Exit("", exitCode)
	}
	return nil
}
