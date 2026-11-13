// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Parsing the path of git remote URLs.

// Package giturl parses git remote URLs.
package giturl

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// errNotGitURL is wrapped by the error that Path returns for a URL that is not a git URL.
var errNotGitURL = errors.New("not a git URL")

// Path returns the path of the git URL rawURL, which is "owner/repository" with an optional
// ".git" suffix for a GitHub repository. The path of an HTTPS URL or of an SCP-like URL, such as
// git@github.com:owner/repository.git, has no leading "/". The path of another URL, such as
// ssh://git@github.com/owner/repository, keeps it. The error does not contain rawURL, which can
// hold credentials.
func Path(rawURL string) (string, error) {
	if strings.HasPrefix(rawURL, "https://") {
		u, err := url.Parse(rawURL)
		if err != nil {
			// A *url.Error contains the URL, so only its cause is kept.
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err
			}

			return "", fmt.Errorf("parse HTTPS URL: %w", err)
		}

		return strings.TrimPrefix(u.Path, "/"), nil
	}

	u, err := giturls.Parse(rawURL)
	if err != nil {
		// The error contains the URL.
		return "", fmt.Errorf("parse SSH URL: %w", errNotGitURL)
	}

	return u.Path, nil
}
