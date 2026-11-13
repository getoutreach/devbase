// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that validates the syntax of CODEOWNERS entries.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// The patterns that owners are matched against.
var (
	// usernameOrTeamRegexp matches a GitHub username, organization name, or team name.
	// A valid username/organization name has up to 39 characters (per GitHub Join page)
	// and is matched by the following regex: /^[a-z\d](?:[a-z\d]|-(?=[a-z\d])){0,38}$/i
	// A valid team name consists of alphanumerics, underscores and dashes.
	usernameOrTeamRegexp = regexp.MustCompile(`^@(?i:[a-z\d](?:[a-z\d_-]){0,37}[a-z\d](/[a-z\d](?:[a-z\d_-]*)[a-z\d])?)$`)

	// emailRegexp matches an email address. As described in
	// https://davidcel.is/posts/stop-validating-email-addresses-with-regex/
	// it only checks for an '@' and a '.' afterwards.
	emailRegexp = regexp.MustCompile(`.+@.+\..+`)
)

// ValidSyntax reports owners that do not look like a GitHub user, a team, or an email address.
// GitHub skips a line that has invalid syntax.
type ValidSyntax struct{}

// NewValidSyntax returns new ValidSyntax instance.
func NewValidSyntax() *ValidSyntax {
	return &ValidSyntax{}
}

// Check for syntax issues in your CODEOWNERS file.
func (*ValidSyntax) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	for _, entry := range in.Entries {
		if entry.Pattern == "" {
			out.Report("Missing pattern", WithEntry(entry))
		}

		for _, owner := range entry.Owners {
			if strings.HasPrefix(owner, "@") {
				if !usernameOrTeamRegexp.MatchString(owner) {
					out.Report(fmt.Sprintf("Owner '%s' does not look like a GitHub username or team name", owner),
						WithEntry(entry), WithSeverity(Warning))
				}
			} else if !emailRegexp.MatchString(owner) {
				out.Report(fmt.Sprintf("Owner '%s' does not look like an email", owner), WithEntry(entry))
			}
		}
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*ValidSyntax) Name() string {
	return "Valid Syntax Checker"
}
