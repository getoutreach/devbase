// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Lists the released versions of a stencil module.

package stencilupgrade

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-github/v84/github"
)

// errNotGitHubModule is returned for a module that is not on GitHub.
var errNotGitHubModule = errors.New("module is not a github.com module")

// releasesPerPage is the largest page size of the GitHub API.
const releasesPerPage = 100

// Release is one release of a module.
type Release struct {
	// Tag is the release tag, for example "v1.2.3".
	Tag string

	// Prerelease is true when GitHub marks the release as a pre-release.
	Prerelease bool
}

// VersionLister finds the released versions of a module.
type VersionLister interface {
	// Releases returns the releases of module, in no defined order.
	Releases(ctx context.Context, module string) ([]Release, error)

	// DefaultBranch returns the name of the default branch of module.
	DefaultBranch(ctx context.Context, module string) (string, error)
}

// GitHubOption configures a lister made by NewGitHubLister.
type GitHubOption func(*githubLister)

// WithBaseURL sets the GitHub API host, for example a test server. The
// "/api/v3/" path is added to it.
func WithBaseURL(u string) GitHubOption {
	return func(l *githubLister) { l.baseURL = u }
}

// githubLister is a VersionLister backed by the GitHub API.
type githubLister struct {
	client  *github.Client
	baseURL string
}

// NewGitHubLister returns a VersionLister that reads releases from
// GitHub. An empty token makes unauthenticated requests. A module must be
// "github.com/<owner>/<repo>[/...]", and only the first three parts are
// used. The lister returns an error for any other module.
func NewGitHubLister(token string, opts ...GitHubOption) VersionLister {
	l := &githubLister{}
	for _, o := range opts {
		o(l)
	}

	client := github.NewClient(nil)
	if token != "" {
		client = client.WithAuthToken(token)
	}
	if l.baseURL != "" {
		// A bad URL is reported by the first request, because the
		// constructor returns no error.
		if c, err := client.WithEnterpriseURLs(l.baseURL, l.baseURL); err == nil {
			client = c
		}
	}
	l.client = client
	return l
}

// Releases implements VersionLister.
func (l *githubLister) Releases(ctx context.Context, module string) ([]Release, error) {
	owner, repo, err := splitModule(module)
	if err != nil {
		return nil, err
	}

	var releases []Release
	opts := &github.ListOptions{PerPage: releasesPerPage}
	for {
		page, resp, err := l.client.Repositories.ListReleases(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list releases of %s/%s: %w", owner, repo, err)
		}
		for _, r := range page {
			releases = append(releases, Release{Tag: r.GetTagName(), Prerelease: r.GetPrerelease()})
		}
		if resp.NextPage == 0 {
			return releases, nil
		}
		opts.Page = resp.NextPage
	}
}

// DefaultBranch implements VersionLister.
func (l *githubLister) DefaultBranch(ctx context.Context, module string) (string, error) {
	owner, repo, err := splitModule(module)
	if err != nil {
		return "", err
	}

	r, _, err := l.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("get repository %s/%s: %w", owner, repo, err)
	}
	return r.GetDefaultBranch(), nil
}

// splitModule returns the GitHub owner and repository of module.
func splitModule(module string) (owner, repo string, err error) {
	parts := strings.Split(module, "/")
	if len(parts) < 3 || parts[0] != "github.com" {
		return "", "", fmt.Errorf("%q: %w", module, errNotGitHubModule)
	}
	return url.PathEscape(parts[1]), url.PathEscape(parts[2]), nil
}
