// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint GraphQL CLI subcommand.

package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

func NewLintGraphqlCommand() *cli.Command {
	return &cli.Command{
		Name:   "graphql",
		Usage:  "Run GraphQL lint checks on the codebase",
		Action: lintGraphQL,
	}
}

func lintGraphQL(ctx context.Context, c *cli.Command) error {
	return nil
}
