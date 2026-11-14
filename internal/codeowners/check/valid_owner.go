// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that validates the owners of CODEOWNERS entries against the GitHub repository.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"slices"
	"strings"

	"github.com/google/go-github/v84/github"
)

// ErrInvalidRepository is wrapped by the error that NewValidOwner returns for a repository name
// that is not in the form "owner/repository".
var ErrInvalidRepository = errors.New("wrong repository name")

// errRateLimitReserve is wrapped by the error for an API call that the check did not make, because
// fewer requests remain in the rate limit than the reserve.
var errRateLimitReserve = errors.New("rate-limit reserve reached")

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

	// RateLimitReserve is the number of GitHub API requests that the check leaves to the other
	// clients of the same rate limit. When CheckSatisfied finds fewer remaining requests, Check
	// skips the validation. When a later response shows fewer remaining requests, Check makes no
	// further call, and returns the issues that it found before. Zero disables the reserve.
	RateLimitReserve int
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

	// rateLimitReserve is the number of requests that the check leaves to the other clients of the
	// rate limit.
	rateLimitReserve int

	// remaining is the number of requests that remain in the rate limit, from the last API response
	// that had the X-RateLimit-Remaining header. It is only valid when hasRemaining is true.
	remaining int

	// hasRemaining reports whether an API response had the X-RateLimit-Remaining header.
	hasRemaining bool

	// skip is the error, which wraps ErrSkipped, that Check returns before any API call, because
	// CheckSatisfied found a low rate limit. It is nil otherwise.
	skip error

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
		rateLimitReserve:     cfg.RateLimitReserve,
	}, nil
}

// Name returns the human-readable name of the check.
func (*ValidOwner) Name() string {
	return "Valid Owner Checker"
}

// CheckSatisfied checks that the repository can be read with the GitHub API client. It also
// keeps the scopes of a classic token for the hints in API error messages. It does not require
// any scope, because fine-grained tokens and GitHub Apps have none.
//
// When the call fails because of a rate limit, or when fewer requests remain than the reserve,
// CheckSatisfied returns nil and Check skips the validation.
func (v *ValidOwner) CheckSatisfied(ctx context.Context) error {
	_, resp, err := v.gh.Repositories.Get(ctx, v.org, v.repo)
	v.record(resp)
	if resp != nil {
		v.scopes = resp.Header.Get("X-Oauth-Scopes")
	}

	switch {
	case isRateLimit(err):
		v.skip = fmt.Errorf("%w: GitHub rate limit reached: %w", ErrSkipped, err)
	case statusCode(err) == http.StatusNotFound:
		return fmt.Errorf("repository %s/%s not found, or it's private and token doesn't have enough permission: %w. %s",
			v.org, v.repo, err, repositoryEndpoint.hint(v.scopes))
	case err != nil:
		return v.apiError("get the repository "+v.fullName(), &repositoryEndpoint, err)
	case v.checkReserve() != nil:
		v.skip = fmt.Errorf("%w: only %d GitHub API requests remain before the rate limit resets, fewer than the reserve of %d",
			ErrSkipped, v.remaining, v.rateLimitReserve)
	}

	return nil
}

// Check validates the owners of the entries. Allowed owner syntax:
//
//	@username
//	@org/team-name
//	user@example.com
//
// Source: "CODEOWNERS syntax" in:
// https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners
//
// Each owner is validated once. When a GitHub API call fails, or when the rate-limit reserve
// stops a call, Check returns the issues found so far and the error. The error wraps ErrSkipped
// when CheckSatisfied found a low rate limit, when the reserve stops a call, or when a call fails
// because of a rate limit.
func (v *ValidOwner) Check(ctx context.Context, in Input) (Output, error) {
	if v.skip != nil {
		return Output{}, v.skip
	}

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
			if err := ctx.Err(); err != nil {
				return Output{}, err
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
		users, err := listAll(v, &opts.ListOptions, func() ([]*github.User, *github.Response, error) {
			return v.gh.Repositories.ListCollaborators(ctx, v.org, v.repo, opts)
		})
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
	case !collaboratorCanWrite(v.collaborators[i]):
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
		opts := &github.ListOptions{PerPage: 100}
		teams, err := listAll(v, opts, func() ([]*github.Team, *github.Response, error) {
			return v.gh.Repositories.ListTeams(ctx, v.org, v.repo, opts)
		})
		switch {
		case statusCode(err) == http.StatusForbidden:
			// The list needs the "Administration" (read) permission. Without it, an empty list makes
			// each team use the team repository call below.
			teams = []*github.Team{}
		case err != nil:
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
		// The repository teams may not include a child team that inherits access from a parent team,
		// so check the team directly.
		if err := v.checkReserve(); err != nil {
			return "", v.apiError(fmt.Sprintf("check the access of team %q to %s", slug, v.fullName()), &teamRepoEndpoint, err)
		}
		repo, resp, err := v.gh.Teams.IsTeamRepoBySlug(ctx, v.org, slug, v.org, v.repo)
		v.record(resp)
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

// apiError returns the error for a failed call to ep in Check. op describes the call. The error of
// a rate limit, or of a call that the rate-limit reserve stopped, wraps ErrSkipped.
func (v *ValidOwner) apiError(op string, ep *endpoint, err error) error {
	switch {
	case errors.Is(err, errRateLimitReserve):
		return fmt.Errorf("%w: the check is incomplete: %s: %w", ErrSkipped, op, err)
	case isRateLimit(err):
		return fmt.Errorf("%w: the check is incomplete: %s: GitHub rate limit reached: %w", ErrSkipped, op, err)
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

// record keeps the number of remaining requests of the rate limit from resp, when resp has it.
func (v *ValidOwner) record(resp *github.Response) {
	if resp != nil && resp.Header.Get(github.HeaderRateRemaining) != "" {
		v.remaining, v.hasRemaining = resp.Rate.Remaining, true
	}
}

// checkReserve returns an error that wraps errRateLimitReserve when the last response showed fewer
// remaining requests than the reserve.
func (v *ValidOwner) checkReserve() error {
	if !v.hasRemaining || v.remaining >= v.rateLimitReserve {
		return nil
	}

	return fmt.Errorf("%w: %d requests left, reserve %d", errRateLimitReserve, v.remaining, v.rateLimitReserve)
}

// listAll returns the items of all the pages of a list call. list returns the page that opts
// selects. Before each page, listAll returns the error of v.checkReserve, if any. The result is not
// nil when there is no error.
func listAll[T any](v *ValidOwner, opts *github.ListOptions, list func() ([]T, *github.Response, error)) ([]T, error) {
	items := []T{}
	for {
		if err := v.checkReserve(); err != nil {
			return nil, err
		}

		page, resp, err := list()
		v.record(resp)
		if err != nil {
			return nil, err
		}

		items = append(items, page...)
		if resp.NextPage == 0 {
			return items, nil
		}
		opts.Page = resp.NextPage
	}
}

// canWrite reports whether p allows to push to the repository: push, maintain, or admin.
func canWrite(p *github.RepositoryPermissions) bool {
	return p.GetPush() || p.GetMaintain() || p.GetAdmin()
}

// collaboratorCanWrite reports whether the collaborator u can push to the repository. The
// permissions of a collaborator are optional in the API response, so without them the role name,
// which is required, decides: write, maintain, or admin.
func collaboratorCanWrite(u *github.User) bool {
	if p := u.GetPermissions(); p != nil {
		return canWrite(p)
	}

	return slices.Contains([]string{"write", "maintain", "admin"}, strings.ToLower(u.GetRoleName()))
}

// isRateLimit reports whether err is the error of a primary or a secondary GitHub rate limit.
func isRateLimit(err error) bool {
	var rateLimitError *github.RateLimitError
	var abuseError *github.AbuseRateLimitError

	return errors.As(err, &rateLimitError) || errors.As(err, &abuseError)
}

// statusCode returns the HTTP status code of a GitHub API error, or 0 when err has none.
func statusCode(err error) int {
	var responseError *github.ErrorResponse
	if errors.As(err, &responseError) && responseError.Response != nil {
		return responseError.Response.StatusCode
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
