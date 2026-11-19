// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Lint mise lockfiles CLI subcommand.

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/getoutreach/devbase/v2/internal/miselock"
	"github.com/urfave/cli/v3"
)

// errMiseLockNoConfigs is returned when "lint mise-lock" is not given a
// mise config to check.
var errMiseLockNoConfigs = errors.New("expected at least one mise config (mise.<env>.toml)")

// newLintMiseLockCommand returns the "mise-lock" subcommand of "lint",
// which checks mise lockfiles against the configs that request their
// tools.
func newLintMiseLockCommand() *cli.Command {
	return &cli.Command{
		Name: "mise-lock",
		Usage: "Check that each mise config's lockfile locks exactly the versions it requests, " +
			"with a url and checksum per platform",
		ArgsUsage: "<mise.<env>.toml>...",
		Action:    lintMiseLock,
	}
}

// lintMiseLock is the Action for the "mise-lock" subcommand. It checks
// the lockfile of every config it is given, prints each problem, and
// exits 1 if there were any.
func lintMiseLock(_ context.Context, c *cli.Command) error {
	configs := c.Args().Slice()
	if len(configs) == 0 {
		return errMiseLockNoConfigs
	}

	failed := false
	for _, config := range configs {
		problems, err := miselock.Check(config)
		if err != nil {
			return err
		}
		for _, p := range problems {
			if _, err := fmt.Fprintln(c.Writer, p); err != nil {
				return fmt.Errorf("write problem: %w", err)
			}
		}
		failed = failed || len(problems) > 0
	}
	if failed {
		return cli.Exit("", 1)
	}
	return nil
}
