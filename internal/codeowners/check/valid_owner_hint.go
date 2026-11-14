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
		note: "The authenticated user must also have write, maintain, or admin privileges on the repository. " +
			"For a repository that an organization owns, the user must also be a member of the organization.",
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
		permission: `organization permission "Members" (read) and the repository permission "Metadata" (read)`,
	}
)

// hint describes the access that the endpoint needs. grantedScopes is the X-OAuth-Scopes header
// value of a classic token; when it is not empty, the hint also tells which scopes are missing, or
// which scopes of the token give the access.
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
		if present, missing := matchScopes(granted, e.scopes); len(missing) > 0 {
			b.WriteString(", and this token is missing " + formatScopes(missing))
		} else {
			b.WriteString(", and this token has " + formatScopes(present))
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

// parentScope returns the scope that directly includes scope, or "" when no scope does. Only the
// scopes that the owners check needs are listed.
func parentScope(scope string) string {
	switch scope {
	case "public_repo":
		return "repo"
	case "read:org":
		return "write:org"
	case "write:org":
		return "admin:org"
	default:
		return ""
	}
}

// matchScopes returns, for each group of needed scopes, the scope in granted that gives the access
// of the group, as a group of one scope. It returns the groups that no granted scope gives access
// for as missing.
func matchScopes(granted []string, needed [][]string) (present, missing [][]string) {
	// grant returns the scope in granted that is scope or includes it, or "" when there is none.
	grant := func(scope string) string {
		for ; scope != ""; scope = parentScope(scope) {
			if slices.Contains(granted, scope) {
				return scope
			}
		}

		return ""
	}

	for _, group := range needed {
		var found string
		for _, scope := range group {
			if found = grant(scope); found != "" {
				break
			}
		}

		if found == "" {
			missing = append(missing, group)
		} else {
			present = append(present, []string{found})
		}
	}

	return present, missing
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
