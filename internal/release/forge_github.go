// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: GitHub-specific parts. Per the RFC, only the forge
// implementation knows about GitHub; the rest of the package uses
// interfaces such as NoteLinks.

package release

import (
	"fmt"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// GitHubLinks builds github.com style links for a repository.
type GitHubLinks struct {
	// RepoURL is the https URL of the repository, without ".git".
	RepoURL string
}

// Compare returns the compare URL between two tags.
func (l GitHubLinks) Compare(from, to string) string {
	return l.RepoURL + "/compare/" + from + "..." + to
}

// Commit returns the URL of a commit.
func (l GitHubLinks) Commit(hash string) string {
	return l.RepoURL + "/commit/" + hash
}

// Issue returns the URL of an issue.
func (l GitHubLinks) Issue(number string) string {
	return l.RepoURL + "/issues/" + number
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
