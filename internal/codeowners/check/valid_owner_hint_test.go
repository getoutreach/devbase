// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the access hints of the owners check.

package check

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestEndpointHint(t *testing.T) {
	const (
		repository = `Access needed for GET /repos/{owner}/{repo}: a classic token needs the scope "repo" for a private repository`
		repoPerm   = `; a fine-grained token or a GitHub App needs the repository permission "Metadata" (read).`

		collaborators     = `Access needed for GET /repos/{owner}/{repo}/collaborators: a classic token needs the scopes "repo" and "read:org"`
		collaboratorsPerm = `; a fine-grained token or a GitHub App needs the repository permission "Metadata" (read).` +
			` The authenticated user must also have write, maintain, or admin privileges on the repository.` +
			` For a repository that an organization owns, the user must also be a member of the organization.`

		repoTeams = `Access needed for GET /repos/{owner}/{repo}/teams: a classic token needs the scope "repo" or "public_repo"` +
			` ("public_repo" only for a public repository)`
		repoTeamsPerm = `; a fine-grained token or a GitHub App needs the repository permission "Administration" (read).`

		teamRepo = `Access needed for GET /orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}: a classic token needs the scope` +
			` "repo" or "admin:org" for a private repository`
		teamRepoPerm = `; a fine-grained token or a GitHub App needs the organization permission "Members" (read)` +
			` and the repository permission "Metadata" (read).`
	)

	tests := map[string]struct {
		ep     endpoint
		scopes string
		want   string
	}{
		"repository, no scopes header": {
			ep:   repositoryEndpoint,
			want: repository + repoPerm,
		},
		"repository, classic token without repo": {
			ep:     repositoryEndpoint,
			scopes: "read:org",
			want:   repository + `, and this token is missing "repo"` + repoPerm,
		},
		"collaborators, no scopes header": {
			ep:   collaboratorsEndpoint,
			want: collaborators + collaboratorsPerm,
		},
		"collaborators, classic token without read:org": {
			ep:     collaboratorsEndpoint,
			scopes: "repo, user",
			want:   collaborators + `, and this token is missing "read:org"` + collaboratorsPerm,
		},
		"collaborators, classic token without any needed scope": {
			ep:     collaboratorsEndpoint,
			scopes: "gist",
			want:   collaborators + `, and this token is missing "repo" and "read:org"` + collaboratorsPerm,
		},
		"collaborators, admin:org includes read:org": {
			ep:     collaboratorsEndpoint,
			scopes: "admin:org,repo",
			want:   collaborators + `, and this token has them` + collaboratorsPerm,
		},
		"collaborators, write:org includes read:org": {
			ep:     collaboratorsEndpoint,
			scopes: "repo, write:org",
			want:   collaborators + `, and this token has them` + collaboratorsPerm,
		},
		"repository teams, no scopes header": {
			ep:   repoTeamsEndpoint,
			want: repoTeams + repoTeamsPerm,
		},
		"repository teams, public_repo": {
			ep:     repoTeamsEndpoint,
			scopes: "public_repo",
			want:   repoTeams + `, and this token has them` + repoTeamsPerm,
		},
		"repository teams, classic token without a repository scope": {
			ep:     repoTeamsEndpoint,
			scopes: "read:org",
			want:   repoTeams + `, and this token is missing "repo" or "public_repo"` + repoTeamsPerm,
		},
		"team repository, no scopes header": {
			ep:   teamRepoEndpoint,
			want: teamRepo + teamRepoPerm,
		},
		"team repository, read:org only": {
			ep:     teamRepoEndpoint,
			scopes: "read:org",
			want:   teamRepo + `, and this token is missing "repo" or "admin:org"` + teamRepoPerm,
		},
		"team repository, admin:org": {
			ep:     teamRepoEndpoint,
			scopes: " admin:org ",
			want:   teamRepo + `, and this token has them` + teamRepoPerm,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.ep.hint(tc.scopes), tc.want)
		})
	}
}
