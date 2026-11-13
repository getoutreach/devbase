// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Parsing and redacting git remote URLs.

// Package giturl parses and redacts git remote URLs.
package giturl

import (
	"errors"
	"net/url"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// ErrInvalidURL is the error that Path returns for a URL that cannot be parsed. It has no other
// text, because the text of a parse error can contain parts of the URL, such as a password.
var ErrInvalidURL = errors.New("the URL cannot be parsed")

// Path returns the path of the git URL rawURL, which is "owner/repository" with an optional
// ".git" suffix for a GitHub repository. The path of an HTTPS URL or of an SCP-like URL, such as
// git@github.com:owner/repository.git, has no leading "/". The path of another URL, such as
// ssh://git@github.com/owner/repository, keeps it. When rawURL cannot be parsed, the error is
// ErrInvalidURL.
func Path(rawURL string) (string, error) {
	if strings.HasPrefix(rawURL, "https://") {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", ErrInvalidURL
		}

		return strings.TrimPrefix(u.Path, "/"), nil
	}

	u, err := giturls.Parse(rawURL)
	if err != nil {
		return "", ErrInvalidURL
	}

	return u.Path, nil
}

// Redact returns the git URL rawURL without its user information, which can hold a password or a
// token. It removes the text up to the last "@" before the first "/" after the scheme, which
// includes all the user information that git can read from the URL.
func Redact(rawURL string) string {
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
