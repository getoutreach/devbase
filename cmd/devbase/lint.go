// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint CLI subcommand.

package main

import "github.com/urfave/cli/v3"

func NewLintCommand() *cli.Command {
	return &cli.Command{
		Name:  "lint",
		Usage: "Run lint checks on the codebase",
		Commands: []*cli.Command{
			NewLintGraphqlCommand(),
		},
	}
}
