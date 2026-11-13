// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that validates the owners of CODEOWNERS entries against the GitHub repository.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/mail"
	"slices"
	"strings"

	"github.com/google/go-github/v84/github"
)

// ErrInvalidRepository is wrapped by the error that NewValidOwner returns for a repository name
// that is not in the form "owner/repository".
var ErrInvalidRepository = errors.New("wrong repository name")

// ValidOwnerConfig configures the ValidOwner check.
type ValidOwnerConfig struct {
	// Repository is the GitHub repository, in the form "owner/repository", that the owners must
	// have write access to.
	Repository string

	// IgnoredOwners are owners that are not validated, such as @ghost, the placeholder user of
	// deleted GitHub accounts.
	IgnoredOwners []string

	// AllowUnownedPatterns allows patterns without owners. For example:
	//
	//	/infra/oncall-rotator/                    @sre-team
	//	/infra/oncall-rotator/oncall-config.yml
	//
	// Here, no one owns the file /infra/oncall-rotator/oncall-config.yml.
	AllowUnownedPatterns bool

	// OwnersMustBeTeams requires every owner to be a team of the organization that owns the repository.
	OwnersMustBeTeams bool
}

// ValidOwner validates that each owner is a GitHub user or team that can review pull requests in
// the repository, or an email address.
//
// A user must be a collaborator of the repository, and a team must have access to the
// repository, either directly or through a parent team. Both need write access: push, maintain,
// or admin. ValidOwner is not safe for concurrent use.
type ValidOwner struct {
	// gh is the GitHub API client.
	gh *github.Client

	// org is the owner of the repository.
	org string

	// repo is the name of the repository, without its owner.
	repo string

	// ignoredOwners are the owners that are not validated.
	ignoredOwners []string

	// allowUnownedPatterns allows patterns without owners.
	allowUnownedPatterns bool

	// ownersMustBeTeams requires every owner to be a team.
	ownersMustBeTeams bool

	// scopes is the X-OAuth-Scopes header value that CheckSatisfied got. It is empty for a
	// fine-grained token or a GitHub App.
	scopes string

	// collaborators are the collaborators of the repository, or nil until they are fetched.
	collaborators []*github.User

	// teams are the teams that have direct access to the repository, or nil until they are fetched.
	teams []*github.Team
}

// NewValidOwner returns a new ValidOwner instance. gh is used by CheckSatisfied, and by Check
// when an owner is a GitHub user or team.
func NewValidOwner(cfg ValidOwnerConfig, gh *github.Client) (*ValidOwner, error) {
	org, repo, ok := strings.Cut(cfg.Repository, "/")
	if !ok || org == "" || repo == "" || strings.Contains(repo, "/") {
		return nil, fmt.Errorf("%w: expected the pattern 'owner/repository', got %q", ErrInvalidRepository, cfg.Repository)
	}

	return &ValidOwner{
		gh:                   gh,
		org:                  org,
		repo:                 repo,
		ignoredOwners:        cfg.IgnoredOwners,
		allowUnownedPatterns: cfg.AllowUnownedPatterns,
		ownersMustBeTeams:    cfg.OwnersMustBeTeams,
	}, nil
}

// Name returns the human-readable name of the check.
func (*ValidOwner) Name() string {
	return "Valid Owner Checker"
}

// CheckSatisfied checks that the repository can be read with the GitHub API client. It also
// keeps the scopes of a classic token for the hints in API error messages. It does not require
// any scope, because fine-grained tokens and GitHub Apps have none.
func (v *ValidOwner) CheckSatisfied(ctx context.Context) error {
	_, resp, err := v.gh.Repositories.Get(ctx, v.org, v.repo)
	if resp != nil {
		v.scopes = resp.Header.Get("X-Oauth-Scopes")
	}

	if statusCode(err) == http.StatusNotFound {
		return fmt.Errorf("repository %s/%s not found, or it's private and token doesn't have enough permission: %w. %s",
			v.org, v.repo, err, repositoryEndpoint.hint(v.scopes))
	}
	if err != nil {
		return v.apiError("get the repository "+v.fullName(), &repositoryEndpoint, err)
	}

	return nil
}

// Check validates the owners of the entries. Allowed owner syntax:
//
//	@username
//	@org/team-name
//	user@example.com
//
// Source: https://help.github.com/articles/about-code-owners/#codeowners-syntax
//
// Each owner is validated once. When a GitHub API call fails, Check returns the issues found so
// far and the error.
func (v *ValidOwner) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	// Owners compare case-insensitively, like GitHub logins, organization names, and team slugs.
	// The ignored owners count as checked.
	checked := map[string]bool{}
	for _, owner := range v.ignoredOwners {
		checked[strings.ToLower(owner)] = true
	}

	for _, entry := range in.Entries {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}

		if len(entry.Owners) == 0 && !v.allowUnownedPatterns {
			out.Report("Missing owner, at least one owner is required", WithEntry(entry), WithSeverity(Warning))
			continue
		}

		for _, owner := range entry.Owners {
			key := strings.ToLower(owner)
			if checked[key] {
				continue
			}
			checked[key] = true

			msg, err := v.validate(ctx, owner)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Output{}, ctxErr
			}
			if err != nil {
				return out, err
			}
			if msg != "" {
				out.Report(msg, WithEntry(entry))
			}
		}
	}

	return out, nil
}

// validate returns the issue with owner, or "" when owner is valid.
func (v *ValidOwner) validate(ctx context.Context, owner string) (string, error) {
	switch {
	case isGitHubTeam(owner):
		return v.validateTeam(ctx, owner)
	case v.ownersMustBeTeams:
		return fmt.Sprintf("Only team owners allowed and %q is not a team", owner), nil
	case isGitHubUser(owner):
		return v.validateUser(ctx, owner)
	case isEmailAddress(owner):
		return "", nil
	default:
		return fmt.Sprintf("Not valid owner definition %q", owner), nil
	}
}

// validateUser returns the issue with the user owner, or "" when the user can review pull requests.
func (v *ValidOwner) validateUser(ctx context.Context, owner string) (string, error) {
	if v.collaborators == nil {
		opts := &github.ListCollaboratorsOptions{Affiliation: "all", ListOptions: github.ListOptions{PerPage: 100}}
		users, err := collect(v.gh.Repositories.ListCollaboratorsIter(ctx, v.org, v.repo, opts))
		if err != nil {
			return "", v.apiError("list the collaborators of "+v.fullName(), &collaboratorsEndpoint, err)
		}
		v.collaborators = users
	}

	login := strings.TrimPrefix(owner, "@")
	i := slices.IndexFunc(v.collaborators, func(u *github.User) bool {
		// GitHub compares logins case-insensitively.
		return strings.EqualFold(u.GetLogin(), login)
	})
	switch {
	case i < 0:
		return fmt.Sprintf("User %q is not a member of the organization %q or has no access to repository %q", owner, v.org, v.repo), nil
	case !canWrite(v.collaborators[i].GetPermissions()):
		return fmt.Sprintf("User %q cannot review PRs on %q as they don't have write permissions.", login, v.repo), nil
	default:
		return "", nil
	}
}

// validateTeam returns the issue with the team owner, or "" when the team can review pull requests.
func (v *ValidOwner) validateTeam(ctx context.Context, owner string) (string, error) {
	org, slug, _ := strings.Cut(strings.TrimPrefix(owner, "@"), "/")

	// GitHub compares organization names and team slugs case-insensitively.
	if !strings.EqualFold(org, v.org) {
		return fmt.Sprintf("Team %q does not belong to %q organization.", owner, v.org), nil
	}

	if v.teams == nil {
		teams, err := collect(v.gh.Repositories.ListTeamsIter(ctx, v.org, v.repo, &github.ListOptions{PerPage: 100}))
		if err != nil {
			return "", v.apiError("list the teams of "+v.fullName(), &repoTeamsEndpoint, err)
		}
		v.teams = teams
	}

	var writable bool
	if i := slices.IndexFunc(v.teams, func(t *github.Team) bool { return strings.EqualFold(t.GetSlug(), slug) }); i >= 0 {
		p := v.teams[i].Permissions
		if p == nil {
			// The permissions map is optional. The permission field is required, and names one permission.
			p = map[string]bool{v.teams[i].GetPermission(): true}
		}
		writable = p["push"] || p["maintain"] || p["admin"]
	} else {
		// The repository teams do not include child teams that inherit access from a parent team.
		repo, _, err := v.gh.Teams.IsTeamRepoBySlug(ctx, v.org, slug, v.org, v.repo)
		switch {
		case statusCode(err) == http.StatusNotFound:
			return fmt.Sprintf("Team %q does not exist in organization %q or has no access to repository %q", owner, org, v.repo), nil
		case err != nil:
			return "", v.apiError(fmt.Sprintf("check the access of team %q to %s", slug, v.fullName()), &teamRepoEndpoint, err)
		}
		writable = canWrite(repo.GetPermissions())
	}

	if !writable {
		return fmt.Sprintf("Team %q cannot review PRs on %q as neither it nor any parent team has write permissions.", slug, v.repo), nil
	}

	return "", nil
}

// apiError returns the error for a failed call to ep. op describes the call.
func (v *ValidOwner) apiError(op string, ep *endpoint, err error) error {
	var rateErr *github.RateLimitError
	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &rateErr) || errors.As(err, &abuseErr) {
		return fmt.Errorf("%s: GitHub rate limit reached: %w", op, err)
	}

	switch statusCode(err) {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s: GitHub authorization is required: %w. %s", op, err, ep.hint(v.scopes))
	case http.StatusForbidden, http.StatusNotFound:
		return fmt.Errorf("%s: %w. %s", op, err, ep.hint(v.scopes))
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// fullName returns the repository in the form "owner/repository".
func (v *ValidOwner) fullName() string {
	return v.org + "/" + v.repo
}

// collect returns the items of seq, or the first error. The result is not nil when there is no error.
func collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	items := []T{}
	for item, err := range seq {
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

// canWrite reports whether p allows to push to the repository: push, maintain, or admin.
func canWrite(p *github.RepositoryPermissions) bool {
	return p.GetPush() || p.GetMaintain() || p.GetAdmin()
}

// statusCode returns the HTTP status code of a GitHub API error, or 0 when err has none.
func statusCode(err error) int {
	var respErr *github.ErrorResponse
	if errors.As(err, &respErr) && respErr.Response != nil {
		return respErr.Response.StatusCode
	}

	return 0
}

// isEmailAddress reports whether s is an email address.
func isEmailAddress(s string) bool {
	_, err := mail.ParseAddress(s)
	return err == nil
}

// isGitHubTeam reports whether s has the syntax of a team: @org/team-name.
func isGitHubTeam(s string) bool {
	org, team, ok := strings.Cut(s, "/")
	return strings.HasPrefix(org, "@") && ok && team != "" && !strings.Contains(team, "/")
}

// isGitHubUser reports whether s has the syntax of a user: @username.
func isGitHubUser(s string) bool {
	return !strings.Contains(s, "/") && strings.HasPrefix(s, "@")
}
