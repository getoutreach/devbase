// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: stencil CLI subcommand.

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/getoutreach/devbase/v2/internal/stencilupgrade"
	"github.com/urfave/cli/v3"
)

// newStencilCommand returns the "stencil" command group: helpers for
// the scheduled stencil-upgrade CI job.
func newStencilCommand() *cli.Command {
	return &cli.Command{
		Name:  "stencil",
		Usage: "Helpers for the stencil-upgrade CI job",
		Commands: []*cli.Command{
			newStencilLockDiffCommand(),
		},
	}
}

// newStencilLockDiffCommand returns the "stencil lock-diff" command,
// which lists the modules that changed between two stencil lockfiles.
func newStencilLockDiffCommand() *cli.Command {
	return &cli.Command{
		Name:  "lock-diff",
		Usage: "Print, as JSON, the modules whose version changed between two stencil.lock files",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "old", Usage: "Path to the old stencil.lock", Required: true},
			&cli.StringFlag{Name: "new", Usage: "Path to the new stencil.lock", Required: true},
		},
		Action: runStencilLockDiff,
	}
}

// runStencilLockDiff is the Action for "stencil lock-diff".
func runStencilLockDiff(_ context.Context, c *cli.Command) error {
	oldLock, err := stencilupgrade.LoadLock(c.String("old"))
	if err != nil {
		return fmt.Errorf("load old lockfile: %w", err)
	}
	newLock, err := stencilupgrade.LoadLock(c.String("new"))
	if err != nil {
		return fmt.Errorf("load new lockfile: %w", err)
	}

	changes := stencilupgrade.LockDiff(oldLock, newLock)
	if changes == nil {
		// Encode "no changes" as [] rather than null.
		changes = []stencilupgrade.ModuleChange{}
	}
	if err := json.NewEncoder(c.Writer).Encode(changes); err != nil {
		return fmt.Errorf("write changes: %w", err)
	}
	return nil
}
