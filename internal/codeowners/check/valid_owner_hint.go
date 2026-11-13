// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Hints about the token access that the GitHub API calls of the owners check need.

package check

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// endpoint is a GitHub REST API endpoint that the owners check calls, with the access that it needs.
type endpoint struct {
	// route is the method and path of the endpoint.
	route string

	// scopes are the scopes that a classic token needs: one scope of each group.
	scopes [][]string

	// scopesNote qualifies scopes, or is empty.
	scopesNote string

	// permission is the permission that a fine-grained token or a GitHub App needs.
	permission string

	// note is more advice, or is empty.
	note string
}

// The endpoints that the owners check calls. The access that they need is from the GitHub REST API documentation.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var (
	// repositoryEndpoint gets the repository.
	repositoryEndpoint = endpoint{
		route:      "GET /repos/{owner}/{repo}",
		scopes:     [][]string{{"repo"}},
		scopesNote: "for a private repository",
		permission: `repository permission "Metadata" (read)`,
	}

	// collaboratorsEndpoint lists the collaborators of the repository.
	collaboratorsEndpoint = endpoint{
		route:      "GET /repos/{owner}/{repo}/collaborators",
		scopes:     [][]string{{"repo"}, {"read:org"}},
		permission: `repository permission "Metadata" (read)`,
		note:       "The authenticated user must also have write, maintain, or admin privileges on the repository.",
	}

	// repoTeamsEndpoint lists the teams of the repository.
	repoTeamsEndpoint = endpoint{
		route:      "GET /repos/{owner}/{repo}/teams",
		scopes:     [][]string{{"repo", "public_repo"}},
		scopesNote: `("public_repo" only for a public repository)`,
		permission: `repository permission "Administration" (read)`,
	}

	// teamRepoEndpoint gets the permissions of a team on the repository, including the permissions
	// that the team inherits from its parent teams.
	teamRepoEndpoint = endpoint{
		route:      "GET /orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}",
		scopes:     [][]string{{"repo", "admin:org"}},
		scopesNote: "for a private repository",
		permission: `organization permission "Members" (read)`,
	}
)

// hint describes the access that the endpoint needs. grantedScopes is the X-OAuth-Scopes header
// value of a classic token; when it is not empty, the hint also tells which scopes are missing.
func (e *endpoint) hint(grantedScopes string) string {
	var b strings.Builder

	label := "scope"
	if len(e.scopes) > 1 {
		label = "scopes"
	}
	fmt.Fprintf(&b, "Access needed for %s: a classic token needs the %s %s", e.route, label, formatScopes(e.scopes))
	if e.scopesNote != "" {
		b.WriteString(" " + e.scopesNote)
	}

	if granted := strings.FieldsFunc(grantedScopes, isScopeSeparator); len(granted) > 0 {
		if missing := missingScopes(granted, e.scopes); len(missing) > 0 {
			b.WriteString(", and this token is missing " + formatScopes(missing))
		} else {
			b.WriteString(", and this token has them")
		}
	}

	fmt.Fprintf(&b, "; a fine-grained token or a GitHub App needs the %s.", e.permission)
	if e.note != "" {
		b.WriteString(" " + e.note)
	}

	return b.String()
}

// isScopeSeparator reports whether r separates the scopes in an X-OAuth-Scopes header value.
func isScopeSeparator(r rune) bool {
	return r == ',' || r == ' '
}

// parentScope returns the scope that includes scope, or "" when no scope does. Only the scopes
// that the owners check needs are listed.
func parentScope(scope string) string {
	switch scope {
	case "public_repo":
		return "repo"
	case "read:org", "write:org":
		return "admin:org"
	default:
		return ""
	}
}

// missingScopes returns the groups of needed scopes that have no scope in granted.
func missingScopes(granted []string, needed [][]string) [][]string {
	has := func(scope string) bool {
		return slices.Contains(granted, scope) || slices.Contains(granted, parentScope(scope))
	}

	var missing [][]string
	for _, group := range needed {
		if !slices.ContainsFunc(group, has) {
			missing = append(missing, group)
		}
	}

	return missing
}

// formatScopes returns groups of scopes as text: the scopes of a group are joined by "or", and
// the groups by "and".
func formatScopes(groups [][]string) string {
	parts := make([]string, len(groups))
	for i, group := range groups {
		quoted := make([]string, len(group))
		for j, scope := range group {
			quoted[j] = strconv.Quote(scope)
		}
		parts[i] = strings.Join(quoted, " or ")
	}

	return strings.Join(parts, " and ")
}
