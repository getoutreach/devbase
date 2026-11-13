// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint CODEOWNERS CLI subcommand.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"github.com/getoutreach/devbase/v2/internal/codeowners/check"
	"github.com/getoutreach/devbase/v2/internal/codeowners/ghclient"
	"github.com/getoutreach/devbase/v2/internal/codeowners/gitcmd"
	"github.com/getoutreach/devbase/v2/internal/codeowners/runner"
	"github.com/getoutreach/devbase/v2/internal/giturl"
	"github.com/getoutreach/devbase/v2/internal/graphql/gitdiff"
	"github.com/getoutreach/gobox/pkg/cfg"
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

// tokenGetter returns a GitHub access token.
type tokenGetter func() (cfg.SecretData, error)

// codeownersCheckNames returns the names of all checks, sorted.
func codeownersCheckNames() []string {
	return []string{checkAvoidShadowing, checkDupPatterns, checkFiles, checkNotOwned, checkOwners, checkSyntax}
}

// newLintCodeownersCommand returns the "codeowners" subcommand of "lint",
// which validates the CODEOWNERS file of a repository.
func newLintCodeownersCommand() *cli.Command {
	return newLintCodeownersCommandWith(githubauth.GetToken)
}

// newLintCodeownersCommandWith returns the "codeowners" subcommand of "lint", which gets a GitHub
// access token from getToken when the owners check needs one.
func newLintCodeownersCommandWith(getToken tokenGetter) *cli.Command {
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
				Name: "owners-if-changed",
				Usage: "Run the owners check only when CODEOWNERS changed since the merge base of HEAD and --base, " +
					"or when the merge base cannot be read",
			},
			&cli.StringFlag{
				Name:  "base",
				Usage: "Ref to compute the merge base against; used with --owners-if-changed",
				Value: "main",
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
			&cli.IntFlag{
				Name:  "github-rate-limit-reserve",
				Usage: "Skip the owners check when fewer GitHub API requests remain in the rate limit (0 disables it)",
				Value: 500,
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
		Action: func(ctx context.Context, c *cli.Command) error {
			return lintCodeowners(ctx, c, getToken)
		},
	}
}

// lintCodeowners is the Action for the "codeowners" subcommand. It loads the CODEOWNERS file of
// the repository, runs the checks of --checks in parallel, and prints their results in the order
// of --checks. With --owners-if-changed, it skips the owners check when CODEOWNERS did not change
// since the merge base of HEAD and --base. It exits with 1 when a check returned an error that is
// not a skip, or found an issue that is at least as serious as --check-failure-level.
func lintCodeowners(ctx context.Context, c *cli.Command, getToken tokenGetter) error {
	failureLevel, err := check.ParseSeverity(c.String("check-failure-level"))
	if err != nil {
		return fmt.Errorf("parse --check-failure-level: %w", err)
	}

	names, err := parseCheckNames(c.StringSlice("checks"))
	if err != nil {
		return err
	}

	staged, trust := c.Bool("staged"), c.Bool("trust-workspace")

	repoDir, err := repositoryRoot(ctx, c.String("repository-path"), trust)
	if err != nil {
		return fmt.Errorf("resolve --repository-path: %w", err)
	}

	var (
		rel     string
		content []byte
	)
	if staged {
		rel, content, err = codeowners.ReadFromIndex(ctx, repoDir, trust)
	} else {
		rel, content, err = codeowners.Read(repoDir)
	}
	if err != nil {
		return fmt.Errorf("load CODEOWNERS: %w", err)
	}

	entries, err := codeowners.Parse(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("load CODEOWNERS: parse %s: %w", rel, err)
	}

	checks := make([]check.Checker, 0, len(names))
	for _, name := range names {
		if name == checkOwners && c.Bool("owners-if-changed") {
			if reason := ownersSkipReason(c.ErrWriter, repoDir, c.String("base"), rel, content); reason != "" {
				// Name does not use the fields of the check.
				checks = append(checks, skippedCheck{name: new(check.ValidOwner).Name(), reason: reason})
				continue
			}
		}

		chk, err := newCodeownersCheck(ctx, c, repoDir, name, getToken)
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

// ownersSkipReason returns why the owners check does not need to run: the CODEOWNERS file at rel,
// relative to repoDir, has the same content at the merge base of HEAD and base. It returns ""
// when the file changed, when it did not exist at the merge base, or when the merge base cannot
// be read. In the last case, it writes a warning line to w.
func ownersSkipReason(w io.Writer, repoDir, base, rel string, content []byte) string {
	mergeBase, err := gitdiff.MergeBaseFiles(repoDir, base, []string{rel})
	if err != nil {
		cause := err.Error()
		if errors.Is(err, gitdiff.ErrShallowClone) {
			// The message of ErrShallowClone names a flag of another command.
			cause = "the repository is a shallow clone"
		}
		fmt.Fprintf(w, "warning: the owners check runs, because CODEOWNERS cannot be compared with the merge base of HEAD and %s: %s\n",
			base, cause)

		return ""
	}

	if old, ok := mergeBase[rel]; ok && old == string(content) {
		return "CODEOWNERS did not change since the merge base with " + base
	}

	return ""
}

// skippedCheck is a check that does not run, and reports why.
type skippedCheck struct {
	// name is the name of the check.
	name string

	// reason is why the check does not run.
	reason string
}

// Name returns the name of the check.
func (s skippedCheck) Name() string {
	return s.name
}

// Check returns an error that wraps check.ErrSkipped with the reason.
func (s skippedCheck) Check(context.Context, check.Input) (check.Output, error) {
	return check.Output{}, fmt.Errorf("%w: %s", check.ErrSkipped, s.reason)
}

// repositoryRoot returns the absolute path of the repository: path when it is not empty, or else
// the root of the git work tree of the current directory, or else the current directory when it
// is not in a git repository. With trust, git trusts the repository, even when another user owns it.
func repositoryRoot(ctx context.Context, path string, trust bool) (string, error) {
	if path != "" {
		return filepath.Abs(path)
	}

	out, err := gitcmd.Output(rootCommand(ctx, trust))

	var gitErr *gitcmd.Error
	switch {
	case err == nil:
		return strings.TrimSpace(string(out)), nil
	case errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, "not a git repository"):
		// The checks that need git report it.
		return filepath.Abs(".")
	default:
		return "", fmt.Errorf("find the root of the git work tree: %w", err)
	}
}

// rootCommand returns the git command that prints the root of the git work tree of the current
// directory. Its messages are in English, so that repositoryRoot can detect them. With trust, git
// trusts every directory for this command, because the root is not known yet.
func rootCommand(ctx context.Context, trust bool) *exec.Cmd {
	args := []string{"rev-parse", "--show-toplevel"}
	if trust {
		args = append([]string{"-c", "safe.directory=*"}, args...)
	}

	cmd := gitcmd.Command(ctx, ".", false, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")

	return cmd
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

// newCodeownersCheck returns the check with the name, configured with the flags of c. The owners
// check gets a GitHub access token from getToken.
func newCodeownersCheck(ctx context.Context, c *cli.Command, repoDir, name string, getToken tokenGetter) (check.Checker, error) {
	switch name {
	case checkFiles:
		return check.NewFileExist(), nil
	case checkOwners:
		return newOwnersCheck(ctx, c, repoDir, getToken)
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
// Without a GitHub App, it gets the access token from getToken.
func newOwnersCheck(ctx context.Context, c *cli.Command, repoDir string, getToken tokenGetter) (check.Checker, error) {
	repository := c.String("repository")
	if repository == "" {
		var err error
		repository, err = repositoryFromOrigin(ctx, repoDir, c.Bool("trust-workspace"))
		if err != nil {
			return nil, err
		}
	}

	clientCfg := ghclient.Config{
		AppID:             c.Int64("github-app-id"),
		AppInstallationID: c.Int64("github-app-installation-id"),
		BaseURL:           c.String("github-base-url"),
		UploadURL:         c.String("github-upload-url"),
		Timeout:           c.Duration("github-timeout"),
	}
	if clientCfg.AppID == 0 {
		token, err := getToken()
		if err != nil {
			return nil, fmt.Errorf("get a GitHub access token for the owners check: %w", err)
		}
		clientCfg.AccessToken = string(token)
	} else {
		clientCfg.AppPrivateKey = []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
	}

	gh, err := ghclient.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("create the GitHub client: %w", err)
	}

	owners, err := check.NewValidOwner(check.ValidOwnerConfig{
		Repository:           repository,
		IgnoredOwners:        c.StringSlice("ignored-owners"),
		AllowUnownedPatterns: c.Bool("allow-unowned-patterns"),
		OwnersMustBeTeams:    c.Bool("owners-must-be-teams"),
		RateLimitReserve:     c.Int("github-rate-limit-reserve"),
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
	out, err := gitcmd.Run(ctx, repoDir, trust, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("get the URL of the origin remote (or set --repository): %w", err)
	}

	origin := strings.TrimSpace(string(out))
	path, err := giturl.Path(origin)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", errInvalidOrigin, giturl.Redact(origin), err)
	}

	repository := strings.TrimPrefix(path, "/")
	repository = strings.TrimSuffix(repository, "/")
	repository = strings.TrimSuffix(repository, ".git")
	if owner, repo, ok := strings.Cut(repository, "/"); !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", fmt.Errorf("%w: %q", errInvalidOrigin, giturl.Redact(origin))
	}

	return repository, nil
}
