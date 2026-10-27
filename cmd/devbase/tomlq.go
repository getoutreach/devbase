// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: tomlq CLI subcommand.

package main

import (
	"context"

	"github.com/getoutreach/devbase/v2/internal/yq"
	"github.com/urfave/cli/v3"
)

// newTomlqCommand returns the "tomlq" command: the same jq-compatible
// query engine as "yq", reading TOML input and writing JSON. See
// newYqCommand for why SkipFlagParsing is set.
func newTomlqCommand() *cli.Command {
	return &cli.Command{
		Name:            "tomlq",
		Usage:           "Evaluate a jq-compatible filter against TOML input",
		ArgsUsage:       "<filter> [file...]",
		SkipFlagParsing: true,
		HideHelp:        true,
		Action:          runTomlq,
	}
}

// runTomlq is the Action for the "tomlq" subcommand.
func runTomlq(ctx context.Context, c *cli.Command) error {
	exitCode := yq.MainTOML(ctx, c.Args().Slice(), c.Reader, c.Writer, c.ErrWriter)
	if exitCode != 0 {
		return cli.Exit("", exitCode)
	}
	return nil
}
