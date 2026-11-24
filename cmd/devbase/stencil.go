// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: stencil CLI subcommand.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/getoutreach/devbase/v2/internal/stencilupgrade"
	"github.com/urfave/cli/v3"
)

// newStencilCommand returns the "stencil" command group: helpers for
// the scheduled stencil-upgrade CI job.
func newStencilCommand() *cli.Command {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return newStencilCommandWithLister(stencilupgrade.NewGitHubLister(token))
}

// newStencilCommandWithLister is newStencilCommand with the version
// source given, so that tests need no network.
func newStencilCommandWithLister(vl stencilupgrade.VersionLister) *cli.Command {
	return &cli.Command{
		Name:  "stencil",
		Usage: "Helpers for the stencil-upgrade CI job",
		Commands: []*cli.Command{
			newStencilLockDiffCommand(),
			newStencilPRBodyCommand(vl),
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

// newStencilPRBodyCommand returns the "stencil pr-body" command, which
// prints the body of the stencil-upgrade pull request.
func newStencilPRBodyCommand(vl stencilupgrade.VersionLister) *cli.Command {
	return &cli.Command{
		Name:  "pr-body",
		Usage: "Print the Markdown body of the stencil-upgrade pull request",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "changes",
				Usage:    "Path to the JSON output of lock-diff, or - to read it from stdin",
				Required: true,
			},
			&cli.StringFlag{Name: "stencil-version", Usage: "Version of stencil that made the changes"},
			&cli.StringFlag{Name: "service-yaml", Usage: "Path to service.yaml, to read the module channels from"},
			&cli.StringFlag{Name: "note", Usage: "Markdown to show at the top of the body"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return runStencilPRBody(ctx, c, vl)
		},
	}
}

// runStencilPRBody is the Action for "stencil pr-body".
func runStencilPRBody(ctx context.Context, c *cli.Command, vl stencilupgrade.VersionLister) error {
	raw, err := readChanges(c)
	if err != nil {
		return err
	}
	var changes []stencilupgrade.ModuleChange
	if err := json.Unmarshal(raw, &changes); err != nil {
		return fmt.Errorf("decode changes: %w", err)
	}

	in := stencilupgrade.BodyInput{
		Changes:        changes,
		StencilVersion: c.String("stencil-version"),
		Note:           c.String("note"),
	}
	if path := c.String("service-yaml"); path != "" {
		if in.Channels, err = stencilupgrade.ChannelsFromServiceYaml(path); err != nil {
			return fmt.Errorf("read module channels: %w", err)
		}
	}

	body, err := stencilupgrade.RenderBody(ctx, in, vl)
	if err != nil {
		return fmt.Errorf("render body: %w", err)
	}
	if _, err := io.WriteString(c.Writer, body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// readChanges reads the file named by --changes, or stdin for "-".
func readChanges(c *cli.Command) ([]byte, error) {
	path := c.String("changes")
	if path == "-" {
		raw, err := io.ReadAll(c.Root().Reader)
		if err != nil {
			return nil, fmt.Errorf("read changes from stdin: %w", err)
		}
		return raw, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read changes: %w", err)
	}
	return raw, nil
}
