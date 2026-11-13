// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Parsing the path of git remote URLs.

// Package giturl parses git remote URLs.
package giturl

import (
	"fmt"
	"net/url"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// Path returns the path of the git URL rawURL, which is "owner/repository" with an optional
// ".git" suffix for a GitHub repository. The path of an HTTPS URL or of an SCP-like URL, such as
// git@github.com:owner/repository.git, has no leading "/". The path of another URL, such as
// ssh://git@github.com/owner/repository, keeps it.
func Path(rawURL string) (string, error) {
	if strings.HasPrefix(rawURL, "https://") {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("parse HTTPS URL: %w", err)
		}

		return strings.TrimPrefix(u.Path, "/"), nil
	}

	u, err := giturls.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse SSH URL: %w", err)
	}

	return u.Path, nil
}
