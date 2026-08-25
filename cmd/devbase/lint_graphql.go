// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint GraphQL CLI subcommand.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// NewLintGraphqlCommand returns the "graphql" subcommand of "lint",
// which runs GraphQL schema lint checks.
func NewLintGraphqlCommand() *cli.Command {
	return &cli.Command{
		Name:  "graphql",
		Usage: "Run GraphQL lint checks on the codebase",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "exclude",
				Aliases: []string{"e"},
				Usage:   "Glob pattern to exclude (repeatable, additive with scripts/devbase.yaml)",
			},
		},
		Action: lintGraphQL,
	}
}

// lintGraphQL is the Action for the "graphql" subcommand. It resolves
// scripts/devbase.yaml for the current working directory and logs the
// effective exclude patterns and rule-override count.
func lintGraphQL(ctx context.Context, c *cli.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	lintConfig, configDir, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("load scripts/devbase.yaml: %w", err)
	}

	excludes := lintConfig.MergeExcludes(c.StringSlice("exclude")...)
	logrus.WithFields(logrus.Fields{
		"configDir": configDir,
		"excludes":  excludes,
		"ruleCount": len(lintConfig.Rules),
	}).Debug("resolved graphql lint config")

	return nil
}
