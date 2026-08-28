// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint GraphQL CLI subcommand.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/devbase/v2/internal/graphql/gitdiff"
	"github.com/getoutreach/devbase/v2/internal/graphql/lint"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"github.com/vektah/gqlparser/v2/ast"
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
			&cli.BoolFlag{
				Name:  "diff",
				Usage: "Report only violations not present at the merge-base of HEAD and --base",
			},
			&cli.StringFlag{
				Name:  "base",
				Usage: "Ref to compute the merge-base against; used with --diff",
				Value: "main",
			},
		},
		Action: lintGraphQL,
	}
}

// lintGraphQL is the Action for the "graphql" subcommand. It resolves
// scripts/devbase.yaml for the current working directory, finds the
// *.graphql files under the given paths (the current directory if none
// are given), runs the Tier 1 and enabled Tier 2 rules against them,
// and prints every violation found -- or, with --diff, only the
// violations not already present at the merge-base of HEAD and --base.
// It returns a non-zero exit code only if at least one reported
// violation resolves to "error" severity: every Tier 1 rule always
// does, since scripts/devbase.yaml can never override it, but a Tier
// 2/3 rule configured as "warn" never does.
func lintGraphQL(ctx context.Context, c *cli.Command) error {
	// This command parses a schema once and exits; there is no later run
	// in the process to free memory for, so collecting garbage here is
	// pure overhead. "Small enough to let the heap grow unchecked" is
	// an assumption about schema sizes, not a bound, so back it with a
	// real one: SetMemoryLimit keeps GC off in the common case but
	// still triggers it before the process approaches that ceiling, per
	// https://go.dev/doc/gc-guide#Memory_limit.
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(1 << 30) // 1GiB

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

	if c.Bool("diff") {
		violations, err = diffViolations(cwd, c.String("base"), files, violations, lintConfig)
		if err != nil {
			return fmt.Errorf("diff graphql files against merge-base: %w", err)
		}
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

// diffViolations narrows workingTreeViolations down to the violations
// not already present at the merge-base of HEAD and base, by reading
// files' merge-base content from the git repository containing dir and
// linting it the same way as the working tree.
func diffViolations(dir, base string, files []string, workingTreeViolations []lint.Violation,
	cfg *config.Lint,
) ([]lint.Violation, error) {
	mergeBaseContent, err := gitdiff.MergeBaseFiles(dir, base, files)
	if err != nil {
		return nil, err
	}

	mergeBaseSources := make([]*ast.Source, 0, len(mergeBaseContent))
	for _, file := range files {
		if content, ok := mergeBaseContent[file]; ok {
			mergeBaseSources = append(mergeBaseSources, &ast.Source{Name: file, Input: content})
		}
	}

	mergeBaseViolations, err := lint.FilesFromSources(mergeBaseSources, cfg)
	if err != nil {
		return nil, fmt.Errorf("lint merge-base graphql files: %w", err)
	}

	return lint.New(mergeBaseViolations, workingTreeViolations), nil
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
