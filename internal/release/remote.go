// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Turns a git remote URL into the https URL of the
// repository.

package release

import (
	"fmt"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// RepoURLFromRemote turns a git remote URL (https, ssh or scp style)
// into an https repository URL without credentials or ".git".
func RepoURLFromRemote(remote string) (string, error) {
	u, err := giturls.Parse(strings.TrimSpace(remote))
	if err != nil {
		return "", fmt.Errorf("parse remote URL: %w", err)
	}
	return "https://" + u.Hostname() + "/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), nil
}
