// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The release commands that push to the remote and the
// forge: publish, run, unstable, promote, preview and rollback.

package main

import (
	"cmp"
	"context"
	"fmt"
	"os"

	"github.com/getoutreach/devbase/v2/internal/release"
	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/forge/github"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// dryRunFlag is the --dry-run flag.
func dryRunFlag(usage string) *cli.BoolFlag {
	return &cli.BoolFlag{Name: "dry-run", Usage: usage}
}

// publishCommands returns the release commands that push.
func publishCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:   "publish",
			Usage:  "Publish a planned release after the build jobs",
			Flags:  []cli.Flag{planFlag(), distFlag(), dryRunFlag("Check the assets and the tag, and publish nothing")},
			Action: releasePublish,
		},
		{
			Name:   "run",
			Usage:  "Plan, build every target and publish in one job",
			Flags:  []cli.Flag{distFlag(), dryRunFlag("Plan, build and check, and publish nothing")},
			Action: releaseRun,
		},
		{
			Name:   "unstable",
			Usage:  "Make the unstable release on the prerelease branch",
			Flags:  []cli.Flag{distFlag(), dryRunFlag("Check the options only")},
			Action: releaseUnstable,
		},
		{
			Name:  "promote",
			Usage: "Move the release branch to the latest release candidate",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "tag", Usage: "Release candidate tag to promote (default: the latest one)"},
				dryRunFlag("Print the promotion and push nothing"),
			},
			Action: releasePromote,
		},
		{
			Name:  "preview",
			Usage: "Preview the release of a pull request",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name: "base", Usage: "Base branch (default: found from the remote)",
					Sources: cli.EnvVars("RELEASE_BASE_BRANCH"),
				},
				&cli.StringFlag{Name: "head", Usage: "Pull request ref", Value: "HEAD"},
			},
			Action: releasePreview,
		},
		{
			Name:  "rollback",
			Usage: "Remove the tag, git note and forge release of a partial release",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "tag", Usage: "Release tag to remove", Required: true},
				&cli.BoolFlag{Name: "force", Usage: "Also remove a published forge release"},
				dryRunFlag("Print what would be deleted"),
			},
			Action: releaseRollback,
		},
	}
}

// openRepo loads the repository in the working directory and returns
// it with the CI branch (CIRCLE_BRANCH, then the GitHub Actions
// GITHUB_REF_NAME), or else the checked-out branch.
func openRepo(ctx context.Context) (*release.Repo, string, error) {
	repo, err := loadRepo()
	if err != nil {
		return nil, "", err
	}
	for _, env := range []string{"CIRCLE_BRANCH", "GITHUB_REF_NAME"} {
		if b := os.Getenv(env); b != "" {
			return repo, b, nil
		}
	}
	branch, err := repo.Git.CurrentBranch(ctx)
	return repo, branch, err
}

// openForge returns the GitHub forge and the git URL to fetch from and
// push to, as semantic-release gets them: the token is GH_TOKEN, then
// GITHUB_TOKEN, and the URL keeps no credentials when a push to branch
// works without them.
func openForge(ctx context.Context, c *cli.Command, repo *release.Repo, branch string) (url string, f forge.Forge, err error) {
	remote, err := repo.Git.RemoteURL(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("read the origin URL: %w", err)
	}
	client := github.New(github.Options{
		Token:          cmp.Or(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")),
		RepoURL:        remote,
		GitCredentials: github.GitCredentialsFromEnv(os.LookupEnv),
		VerifyAuth: func(ctx context.Context, url string) error {
			_, err := repo.Git.Run(ctx, "push", "--dry-run", "--no-verify", "--", url, "HEAD:"+branch)
			return err
		},
		Log: func(format string, args ...any) { fmt.Fprintf(c.Root().ErrWriter, format+"\n", args...) },
	})
	url, err = client.GitURL(ctx)
	return url, client, err
}

// makePlan loads the repository, logs its config warnings, and plans the
// release of branch, or of the checked-out branch when it is empty.
func makePlan(ctx context.Context, branch string) (*release.Repo, *release.Plan, error) {
	repo, err := loadRepo()
	if err != nil {
		return nil, nil, err
	}
	for _, w := range repo.Warnings {
		logrus.Warn(w)
	}
	remote, err := repo.Git.RemoteURL(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read the origin URL: %w", err)
	}
	plan, err := release.MakePlan(ctx, repo, &release.PlanOptions{Branch: branch, NewLinks: github.NewLinks, Remote: remote})
	if err != nil {
		return nil, nil, fmt.Errorf("plan release: %w", err)
	}
	return repo, plan, nil
}

// releasePublish is the Action for "release publish".
func releasePublish(ctx context.Context, c *cli.Command) error {
	repo, plan, err := loadPlan(c)
	if err != nil || plan == nil {
		return err
	}
	return publishPlan(ctx, c, repo, plan)
}

// publishPlan publishes plan with the forge and the URL of repo.
func publishPlan(ctx context.Context, c *cli.Command, repo *release.Repo, plan *release.Plan) error {
	url, f, err := openForge(ctx, c, repo, plan.Branch.Name)
	if err != nil {
		return err
	}
	w := c.Root().Writer
	err = release.Publish(ctx, repo, plan, &release.PublishOptions{
		Dist: c.String("dist"), Remote: url, Forge: f, Env: os.Environ(), Log: w, DryRun: c.Bool("dry-run"),
	})
	if err != nil {
		return fmt.Errorf("publish release: %w", err)
	}
	if c.Bool("dry-run") {
		fmt.Fprintf(w, "Dry run: %s is ready to publish\n", plan.NextRelease.GitTag)
	} else {
		fmt.Fprintf(w, "Published %s\n", plan.NextRelease.GitTag)
	}
	return nil
}

// releaseRun is the Action for "release run": plan, build every target,
// and publish, in one job.
func releaseRun(ctx context.Context, c *cli.Command) error {
	_, branch, err := openRepo(ctx)
	if err != nil {
		return err
	}
	repo, plan, err := makePlan(ctx, branch)
	if err != nil {
		return err
	}
	if !plan.Release {
		printPlan(c, plan)
		return nil
	}
	if a := repo.Config.Artifacts; a != nil {
		_, err := release.Build(ctx, a, plan, &release.BuildOptions{
			Dir: repo.Git.Dir, Dist: c.String("dist"), Env: os.Environ(),
		})
		if err != nil {
			return fmt.Errorf("build release: %w", err)
		}
	}
	return publishPlan(ctx, c, repo, plan)
}

// releaseUnstable is the Action for "release unstable". It checks the
// options before it sets up the forge, so a skipped job does no network
// work.
func releaseUnstable(ctx context.Context, c *cli.Command) error {
	repo, branch, err := openRepo(ctx)
	if err != nil {
		return err
	}
	w := c.Root().Writer
	if ok, err := repo.UnstableEnabled(branch, c.Bool("dry-run"), w); !ok {
		return err
	}
	url, f, err := openForge(ctx, c, repo, branch)
	if err != nil {
		return err
	}
	return release.Unstable(ctx, repo, &release.UnstableOptions{
		Branch: branch, Dist: c.String("dist"), Remote: url, Forge: f, Env: os.Environ(), Log: w,
	})
}

// releasePromote is the Action for "release promote". It runs in the
// release_promote job, or from a laptop with the user's git credentials.
func releasePromote(ctx context.Context, c *cli.Command) error {
	repo, branch, err := openRepo(ctx)
	if err != nil {
		return err
	}
	url, _, err := openForge(ctx, c, repo, branch)
	if err != nil {
		return err
	}
	_, err = release.Promote(ctx, repo, &release.PromoteOptions{
		Tag: c.String("tag"), Remote: url, DryRun: c.Bool("dry-run"), Log: c.Root().Writer,
	})
	return err
}

// releasePreview is the Action for "release preview". It skips a
// CircleCI pull request from a fork, which has no access to the secrets.
func releasePreview(ctx context.Context, c *cli.Command) error {
	w := c.Root().Writer
	if os.Getenv("CIRCLE_PR_REPONAME") != "" {
		fmt.Fprintln(w, "Skipping the release preview: it does not run in CircleCI for pull requests from forks")
		return nil
	}
	repo, err := loadRepo()
	if err != nil {
		return err
	}
	remote, err := repo.Git.RemoteURL(ctx)
	if err != nil {
		return fmt.Errorf("read the origin URL: %w", err)
	}
	plan, err := release.Preview(ctx, repo, &release.PreviewOptions{
		Base: c.String("base"), Head: c.String("head"), Remote: remote, Log: w, NewLinks: github.NewLinks,
	})
	switch {
	case err != nil:
		return fmt.Errorf("preview release: %w", err)
	case plan == nil:
		fmt.Fprintln(w, "No changes to release")
	default:
		printPlan(c, plan)
	}
	return nil
}

// releaseRollback is the Action for "release rollback".
func releaseRollback(ctx context.Context, c *cli.Command) error {
	repo, branch, err := openRepo(ctx)
	if err != nil {
		return err
	}
	url, f, err := openForge(ctx, c, repo, branch)
	if err != nil {
		return err
	}
	_, err = release.Rollback(ctx, repo, &release.RollbackOptions{
		Tag: c.String("tag"), Remote: url, Forge: f, Env: os.Environ(),
		Force: c.Bool("force"), DryRun: c.Bool("dry-run"), Log: c.Root().Writer,
	})
	return err
}
