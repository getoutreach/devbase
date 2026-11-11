// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: GitHub-specific parts. Per the RFC, only the forge
// implementation knows about GitHub; the rest of the package uses
// interfaces such as NoteLinks.

package release

import (
	"cmp"
	"fmt"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// GitHubLinks builds github.com style links for a repository.
type GitHubLinks struct {
	// Host is the scheme and host, such as "https://github.com".
	Host string

	// Owner and Repo name the repository.
	Owner, Repo string
}

// NewGitHubLinks returns the links for an https repository URL without
// ".git", as RepoURLFromRemote returns it. As in
// release-notes-generator, the first path element is the owner and the
// rest is the repository.
func NewGitHubLinks(repoURL string) GitHubLinks {
	scheme, rest, _ := strings.Cut(repoURL, "://")
	host, path, _ := strings.Cut(rest, "/")
	owner, repo, _ := strings.Cut(path, "/")
	return GitHubLinks{Host: scheme + "://" + host, Owner: owner, Repo: repo}
}

// Compare returns the compare URL between two tags.
func (l GitHubLinks) Compare(from, to string) string {
	return l.repoURL() + "/compare/" + from + "..." + to
}

// Commit returns the URL of a commit.
func (l GitHubLinks) Commit(hash string) string {
	return l.repoURL() + "/commit/" + hash
}

// Issue returns the URL of an issue. An empty owner or repo means the
// owner or repository of l.
func (l GitHubLinks) Issue(owner, repo, id string) string {
	return l.Host + "/" + cmp.Or(owner, l.Owner) + "/" + cmp.Or(repo, l.Repo) + "/issues/" + id
}

// User returns the URL of a user.
func (l GitHubLinks) User(name string) string {
	return l.Host + "/" + name
}

// repoURL returns the URL of the repository.
func (l GitHubLinks) repoURL() string {
	return l.Host + "/" + l.Owner + "/" + l.Repo
}

// RepoURLFromRemote turns a git remote URL (https, ssh or scp style)
// into an https repository URL without credentials or ".git".
func RepoURLFromRemote(remote string) (string, error) {
	u, err := giturls.Parse(strings.TrimSpace(remote))
	if err != nil {
		return "", fmt.Errorf("parse remote URL: %w", err)
	}
	return "https://" + u.Hostname() + "/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), nil
}
