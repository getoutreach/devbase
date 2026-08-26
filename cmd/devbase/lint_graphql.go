// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint GraphQL CLI subcommand.

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/devbase/v2/internal/graphql/lint"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// newLintGraphQLCommand returns the "graphql" subcommand of "lint",
// which runs GraphQL schema lint checks.
func newLintGraphQLCommand() *cli.Command {
	return &cli.Command{
		Name:      "graphql",
		Usage:     "Run GraphQL lint checks on the codebase",
		ArgsUsage: "[paths...]",
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
// scripts/devbase.yaml for the current working directory, finds the
// *.graphql files under the given paths (the current directory if none
// are given), runs the Tier 1 and enabled Tier 2 rules against them,
// and prints every violation found. It returns a non-zero exit code
// only if at least one violation resolves to "error" severity: every
// Tier 1 rule always does, since scripts/devbase.yaml can never
// override it, but a Tier 2/3 rule configured as "warn" never does.
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

	paths := c.Args().Slice()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	files, err := lint.FindGraphQLFiles(paths, excludes)
	if err != nil {
		return fmt.Errorf("find graphql files: %w", err)
	}

	violations, err := lint.Files(files, lintConfig)
	if err != nil {
		return fmt.Errorf("lint graphql files: %w", err)
	}

	hasError, err := reportViolations(c.Writer, violations, lintConfig)
	if err != nil {
		return err
	}
	if hasError {
		return cli.Exit("", 1)
	}

	return nil
}

// reportViolations prints each violation to w and reports whether any
// of them resolved to "error" severity via cfg.SeverityOf -- the
// signal lintGraphQL uses to decide whether the run failed. A "warn"
// violation is printed but never sets hasError.
func reportViolations(w io.Writer, violations []lint.Violation, cfg *config.Lint) (hasError bool, err error) {
	for _, v := range violations {
		if _, err := fmt.Fprintln(w, v.String()); err != nil {
			return false, fmt.Errorf("write violation: %w", err)
		}
		if cfg.SeverityOf(v.Rule) == config.SeverityError {
			hasError = true
		}
	}
	return hasError, nil
}
