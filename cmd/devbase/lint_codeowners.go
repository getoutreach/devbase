// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint CODEOWNERS CLI subcommand.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	giturls "github.com/chainguard-dev/git-urls"
	"github.com/fatih/color"
	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"github.com/getoutreach/devbase/v2/internal/codeowners/check"
	"github.com/getoutreach/devbase/v2/internal/codeowners/ghclient"
	"github.com/getoutreach/devbase/v2/internal/codeowners/runner"
	githubauth "github.com/getoutreach/gobox/pkg/cli/github"
	"github.com/urfave/cli/v3"
)

// The names of the checks that the --checks flag accepts.
const (
	// checkFiles is the name of the check that the patterns match files.
	checkFiles = "files"

	// checkOwners is the name of the check that the owners have access to the repository.
	checkOwners = "owners"

	// checkDupPatterns is the name of the check for repeated patterns.
	checkDupPatterns = "duppatterns"

	// checkSyntax is the name of the check for the syntax of the file.
	checkSyntax = "syntax"

	// checkNotOwned is the name of the check that every file has an owner.
	checkNotOwned = "notowned"

	// checkAvoidShadowing is the name of the check for patterns that shadow earlier patterns.
	checkAvoidShadowing = "avoid-shadowing"
)

// The errors that the command wraps.
var (
	// errUnknownCheck is wrapped by the error for a check name that does not exist.
	errUnknownCheck = errors.New("unknown check")

	// errInvalidOrigin is wrapped by the error for an origin remote that is not a GitHub repository.
	errInvalidOrigin = errors.New("the origin remote is not in the form owner/repository")
)

// codeownersCheckNames returns the names of all checks, sorted.
func codeownersCheckNames() []string {
	return []string{checkAvoidShadowing, checkDupPatterns, checkFiles, checkNotOwned, checkOwners, checkSyntax}
}

// newLintCodeownersCommand returns the "codeowners" subcommand of "lint",
// which validates the CODEOWNERS file of a repository.
func newLintCodeownersCommand() *cli.Command {
	return &cli.Command{
		Name:  "codeowners",
		Usage: "Validate the CODEOWNERS file of a repository",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "repository-path",
				Usage: "Path to the root of the repository (default: the root of the git work tree of the current directory)",
			},
			&cli.StringSliceFlag{
				Name: "checks",
				Usage: "Checks to run, in order (" + strings.Join(codeownersCheckNames(), ", ") + "). " +
					"The owners check needs GitHub access",
				Value: []string{checkFiles, checkOwners, checkDupPatterns, checkSyntax},
			},
			&cli.StringFlag{
				Name:  "check-failure-level",
				Usage: "Lowest severity of an issue that makes the command fail (error or warning)",
				Value: "warning",
			},
			&cli.StringFlag{
				Name:  "repository",
				Usage: "GitHub repository in the form owner/repository, for the owners check (default: the origin remote)",
			},
			&cli.StringSliceFlag{
				Name:  "ignored-owners",
				Usage: "Owners that the owners check does not validate",
				Value: []string{"@ghost"},
			},
			&cli.BoolFlag{
				Name:  "allow-unowned-patterns",
				Usage: "Allow patterns without owners in the owners check (use =false to report them)",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "owners-must-be-teams",
				Usage: "Require every owner to be a team of the organization of the repository",
			},
			&cli.StringSliceFlag{
				Name:  "skip-patterns",
				Usage: "Patterns that do not count as owning a file in the notowned check",
			},
			&cli.StringSliceFlag{
				Name:  "subdirectories",
				Usage: "Only check the files in these directories in the notowned check",
			},
			&cli.BoolFlag{
				Name:  "staged",
				Usage: "Read CODEOWNERS and the files from the git index, and ignore changes that are not staged (for a pre-commit hook)",
			},
			&cli.BoolFlag{
				Name:  "trust-workspace",
				Usage: "Make git trust the repository directory, even when another user owns it",
			},
			&cli.StringFlag{
				Name:  "github-base-url",
				Usage: "Base URL of the API of a GitHub Enterprise Server",
			},
			&cli.StringFlag{
				Name:  "github-upload-url",
				Usage: "Upload URL of a GitHub Enterprise Server (default: the base URL)",
			},
			&cli.DurationFlag{
				Name:  "github-timeout",
				Usage: "Timeout of each GitHub API request",
				Value: 30 * time.Second,
			},
			&cli.Int64Flag{
				Name:    "github-app-id",
				Usage:   "ID of a GitHub App to authenticate as, instead of a token (private key: GITHUB_APP_PRIVATE_KEY)",
				Sources: cli.EnvVars("GITHUB_APP_ID"),
			},
			&cli.Int64Flag{
				Name:    "github-app-installation-id",
				Usage:   "Installation ID of the GitHub App",
				Sources: cli.EnvVars("GITHUB_APP_INSTALLATION_ID"),
			},
		},
		Action: lintCodeowners,
	}
}

// lintCodeowners is the Action for the "codeowners" subcommand. It loads the CODEOWNERS file of
// the repository, runs the checks of --checks in parallel, and prints their results in the order
// of --checks. It exits with 1 when a check returned an error, or found an issue that is at least
// as serious as --check-failure-level.
func lintCodeowners(ctx context.Context, c *cli.Command) error {
	failureLevel, err := check.ParseSeverity(c.String("check-failure-level"))
	if err != nil {
		return fmt.Errorf("parse --check-failure-level: %w", err)
	}

	names, err := parseCheckNames(c.StringSlice("checks"))
	if err != nil {
		return err
	}

	repoDir, err := repositoryRoot(ctx, c.String("repository-path"))
	if err != nil {
		return fmt.Errorf("resolve --repository-path: %w", err)
	}

	staged, trust := c.Bool("staged"), c.Bool("trust-workspace")

	var entries []codeowners.Entry
	if staged {
		entries, err = codeowners.LoadFromIndex(ctx, repoDir, trust)
	} else {
		entries, err = codeowners.Load(repoDir)
	}
	if err != nil {
		return fmt.Errorf("load CODEOWNERS: %w", err)
	}

	checks := make([]check.Checker, 0, len(names))
	for _, name := range names {
		chk, err := newCodeownersCheck(ctx, c, repoDir, name)
		if err != nil {
			return err
		}
		checks = append(checks, chk)
	}

	in := check.Input{RepoDir: repoDir, Entries: entries, TrustRepoDir: trust, Staged: staged}
	printer := runner.NewPrinter(c.Writer, color.NoColor || c.Writer != os.Stdout)
	failed := runner.Run(ctx, printer, in, failureLevel, checks...)

	if err := ctx.Err(); err != nil {
		return err
	}
	if failed {
		return cli.Exit("", 1)
	}

	return nil
}

// repositoryRoot returns the absolute path of the repository: path when it is not empty, or else
// the root of the git work tree of the current directory, or else the current directory.
func repositoryRoot(ctx context.Context, path string) (string, error) {
	if path == "" {
		out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}

		// Not in a git work tree. The checks that need git report it.
		path = "."
	}

	return filepath.Abs(path)
}

// parseCheckNames returns names without the repeated names, or an error when a name is not the
// name of a check.
func parseCheckNames(names []string) ([]string, error) {
	valid := codeownersCheckNames()
	unique := make([]string, 0, len(names))

	for _, name := range names {
		if !slices.Contains(valid, name) {
			return nil, fmt.Errorf("%w %q, the valid checks are: %s", errUnknownCheck, name, strings.Join(valid, ", "))
		}
		if !slices.Contains(unique, name) {
			unique = append(unique, name)
		}
	}

	return unique, nil
}

// newCodeownersCheck returns the check with the name, configured with the flags of c.
func newCodeownersCheck(ctx context.Context, c *cli.Command, repoDir, name string) (check.Checker, error) {
	switch name {
	case checkFiles:
		return check.NewFileExist(), nil
	case checkOwners:
		return newOwnersCheck(ctx, c, repoDir)
	case checkDupPatterns:
		return check.NewDuplicatedPattern(), nil
	case checkSyntax:
		return check.NewValidSyntax(), nil
	case checkNotOwned:
		return check.NewNotOwnedFile(check.NotOwnedFileConfig{
			SkipPatterns:   c.StringSlice("skip-patterns"),
			Subdirectories: c.StringSlice("subdirectories"),
		}), nil
	case checkAvoidShadowing:
		return check.NewAvoidShadowing(), nil
	default:
		return nil, fmt.Errorf("%w %q", errUnknownCheck, name)
	}
}

// newOwnersCheck returns the owners check, after it made sure that its GitHub access is enough.
func newOwnersCheck(ctx context.Context, c *cli.Command, repoDir string) (check.Checker, error) {
	repository := c.String("repository")
	if repository == "" {
		var err error
		repository, err = repositoryFromOrigin(ctx, repoDir, c.Bool("trust-workspace"))
		if err != nil {
			return nil, err
		}
	}

	cfg := ghclient.Config{
		AppID:             c.Int64("github-app-id"),
		AppInstallationID: c.Int64("github-app-installation-id"),
		BaseURL:           c.String("github-base-url"),
		UploadURL:         c.String("github-upload-url"),
		Timeout:           c.Duration("github-timeout"),
	}
	if cfg.AppID == 0 {
		token, err := githubauth.GetToken()
		if err != nil {
			return nil, fmt.Errorf("get a GitHub access token for the owners check: %w", err)
		}
		cfg.AccessToken = string(token)
	} else {
		cfg.AppPrivateKey = []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
	}

	gh, err := ghclient.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create the GitHub client: %w", err)
	}

	owners, err := check.NewValidOwner(check.ValidOwnerConfig{
		Repository:           repository,
		IgnoredOwners:        c.StringSlice("ignored-owners"),
		AllowUnownedPatterns: c.Bool("allow-unowned-patterns"),
		OwnersMustBeTeams:    c.Bool("owners-must-be-teams"),
	}, gh)
	if err != nil {
		return nil, fmt.Errorf("create the owners check: %w", err)
	}

	if err := owners.CheckSatisfied(ctx); err != nil {
		return nil, fmt.Errorf("the owners check cannot run: %w", err)
	}

	return owners, nil
}

// repositoryFromOrigin returns the GitHub repository, in the form owner/repository, that the
// origin remote of the git repository in repoDir points to.
func repositoryFromOrigin(ctx context.Context, repoDir string, trust bool) (string, error) {
	args := []string{"-C", repoDir, "remote", "get-url", "origin"}
	if trust {
		args = append([]string{"-c", "safe.directory=" + repoDir}, args...)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("get the URL of the origin remote of %s (or set --repository): %w: %s",
			repoDir, err, strings.TrimSpace(stderr.String()))
	}

	// The error does not wrap the parse error, which shows the full URL with its credentials.
	origin := strings.TrimSpace(stdout.String())
	path, err := pathFromGitURL(origin)
	repository := strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".git")
	if owner, repo, ok := strings.Cut(repository, "/"); err != nil || !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", fmt.Errorf("%w: %q", errInvalidOrigin, redactURL(origin))
	}

	return repository, nil
}

// redactURL returns the git URL without its user information, which can hold a password or a
// token. Like git, it takes the user information from the text before the first "/" after the
// scheme, and it removes all of it up to the last "@".
func redactURL(rawURL string) string {
	start := 0
	if i := strings.Index(rawURL, "://"); i >= 0 {
		start = i + len("://")
	}

	authority, _, _ := strings.Cut(rawURL[start:], "/")
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		return rawURL[:start] + rawURL[start+at+1:]
	}

	return rawURL
}

// pathFromGitURL returns the path of the git URL, which is "owner/repository" with an optional
// ".git" suffix for a GitHub repository.
func pathFromGitURL(origin string) (string, error) {
	if strings.HasPrefix(origin, "https://") {
		u, err := url.Parse(origin)
		if err != nil {
			return "", fmt.Errorf("parse HTTPS URL: %w", err)
		}

		return u.Path, nil
	}

	u, err := giturls.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("parse SSH URL: %w", err)
	}

	return u.Path, nil
}
