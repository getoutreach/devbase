// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of Success, ported from test/success.test.js,
// test/find-sr-issue.test.js and test/get-success-comment.test.js of
// @semantic-release/github 12.0.10.

package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// githubRelease is the GitHub release of the success tests.
var githubRelease = forge.Published{Name: "GitHub release", URL: "https://github.com/release"}

// prNode is a pull request node of a GraphQL response.
func prNode(number int, body string) map[string]any {
	node := map[string]any{"number": number, "__typename": "PullRequest", "state": "closed"}
	if body != "" {
		node["body"] = body
	}
	return node
}

// issueNode is an issue node of a GraphQL response.
func issueNode(number int) map[string]any {
	return map[string]any{
		"__typename": "Issue",
		"number":     number,
		"body":       "",
		"title":      "issue title",
		"url":        fmt.Sprintf("https://github.com/owner/repo/issues/%d", number),
		"state":      "CLOSED",
	}
}

// commitPRs is the associatedPullRequests of a commit in a GraphQL
// response.
func commitPRs(oid string, hasNextPage bool, cursor string, prs ...map[string]any) map[string]any {
	if prs == nil {
		prs = []map[string]any{}
	}
	return map[string]any{
		"oid": oid,
		"associatedPullRequests": map[string]any{
			"pageInfo": map[string]any{"endCursor": cursor, "hasNextPage": hasNextPage},
			"nodes":    prs,
		},
	}
}

// associatedPRsRoute is the getAssociatedPRs query for owner/repo, which
// answers with the commits, keyed by their alias. JSON encodes the keys
// in sorted order, which is the commit order of the tests.
func associatedPRsRoute(owner, repo string, commits map[string]any) route {
	return route{
		method:   http.MethodPost,
		path:     "/graphql",
		graphql:  "getAssociatedPRs",
		vars:     map[string]any{"owner": owner, "repo": repo},
		response: map[string]any{"data": map[string]any{"repository": commits}},
	}
}

// relatedIssuesRoute is the getRelatedIssues query for owner/repo with
// the issue numbers, which answers with issues.
func relatedIssuesRoute(owner, repo string, numbers []int, issues map[string]any) route {
	return route{
		method:  http.MethodPost,
		path:    "/graphql",
		graphql: "getRelatedIssues",
		vars:    map[string]any{"owner": owner, "repo": repo},
		check: func(t *testing.T, _ *http.Request, body []byte) {
			t.Helper()
			t.Helper()
			for _, n := range numbers {
				assert.Check(t, cmp.Contains(string(body), fmt.Sprintf("issue%d: issueOrPullRequest(number: %d)", n, n)))
			}
			assert.Check(t, cmp.Equal(strings.Count(string(body), ": issueOrPullRequest("), len(numbers)))
		},
		response: map[string]any{"data": map[string]any{"repository": issues}},
	}
}

// prCommitsRoute is the list of the commits of a pull request.
func prCommitsRoute(owner, repo string, number int, shas ...string) route {
	commits := make([]map[string]any, 0, len(shas))
	for _, sha := range shas {
		commits = append(commits, map[string]any{"sha": sha})
	}
	return route{method: http.MethodGet, path: fmt.Sprintf("/repos/%s/%s/pulls/%d/commits", owner, repo, number), response: commits}
}

// prRoute is the GET of a pull request of test_user/test_repo with its
// merge commit.
func prRoute(number int, mergeSHA string) route {
	return route{
		method:   http.MethodGet,
		path:     fmt.Sprintf("/repos/test_user/test_repo/pulls/%d", number),
		response: map[string]any{"number": number, "merge_commit_sha": mergeSHA},
	}
}

// commentRoutes are the comment and the "released" label of an issue or
// pull request.
func commentRoutes(owner, repo string, number int) []route {
	return []route{
		{
			method:   http.MethodPost,
			path:     fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number),
			response: map[string]any{"html_url": fmt.Sprintf("https://github.com/successcomment-%d", number)},
		},
		{
			method:   http.MethodPost,
			path:     fmt.Sprintf("/repos/%s/%s/issues/%d/labels", owner, repo, number),
			body:     []string{"released"},
			response: []map[string]any{},
		},
	}
}

// commentLogs are the logs of commentRoutes.
func commentLogs(kind string, number int) []string {
	return []string{
		fmt.Sprintf("Added comment to %s #%d: https://github.com/successcomment-%d", kind, number, number),
		fmt.Sprintf(`Added labels ["released"] to %s #%d`, kind, number),
	}
}

// closeRoute is the PATCH that closes a failure issue.
func closeRoute(number, status int) route {
	return route{
		method:   http.MethodPatch,
		path:     fmt.Sprintf("/repos/test_user/test_repo/issues/%d", number),
		body:     map[string]any{"state": "closed"},
		status:   status,
		response: map[string]any{"html_url": fmt.Sprintf("https://github.com/issues/%d", number)},
	}
}

// failureIssues are the open failure issues of the tests: #1 has no
// ISSUE_ID marker.
func failureIssues() []map[string]any {
	return []map[string]any{
		{"number": 1, "body": "Issue 1 body", "title": failTitle},
		{"number": 2, "body": "Issue 2 body\n\n" + issueID, "title": failTitle},
		{"number": 3, "body": "Issue 3 body\n\n" + issueID, "title": failTitle},
	}
}

// TestSuccessCommentsAndLabels ports "Add comment and labels to PRs
// associated with release commits and issues solved by PR/commits
// comments" of success.test.js, with a renamed repository.
func TestSuccessCommentsAndLabels(t *testing.T) {
	const owner, repo = "test_user_2", "test_repo_2"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			"commit123": commitPRs("123", false, "NI", prNode(1, "")),
			"commit456": commitPRs("456", false, "NI", prNode(2, "Fixes #3")),
		}),
		prCommitsRoute(owner, repo, 1, "123"),
		prCommitsRoute(owner, repo, 2, "456"),
		relatedIssuesRoute(owner, repo, []int{3, 1, 4}, map[string]any{"issue3": issueNode(3), "issue4": issueNode(4)}),
	)
	comments := commentRoutes(owner, repo, 1)
	comments[0].body = map[string]any{"body": ":tada: This PR is included in version 1.0.0 :tada:\n\n" +
		"The release is available on [GitHub release](https://github.com/release)\n\n" +
		"Your **[semantic-release](https://github.com/semantic-release/semantic-release)** bot :package::rocket:"}
	f.expect(comments...)
	f.expect(commentRoutes(owner, repo, 2)...)
	f.expect(commentRoutes(owner, repo, 3)...)
	f.expect(commentRoutes(owner, repo, 4)...)
	f.expect(srIssuesRoute(owner, repo))

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.0.0",
		Commits: []forge.Commit{
			{Hash: "123", Message: "Commit 1 message\n\n Fix #1"},
			{Hash: "456", Message: "Commit 2 message"},
			{Hash: "789", Message: "Commit 3 message Closes https://github.com/" + owner + "/" + repo + "/issues/4"},
		},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	want := make([]string, 0, 8)
	want = append(want, commentLogs("PR", 1)...)
	want = append(want, commentLogs("PR", 2)...)
	want = append(want, commentLogs("issue", 3)...)
	want = append(want, commentLogs("issue", 4)...)
	assert.DeepEqual(t, logs, want)
}

// TestSuccessMultipagedPRs ports "Add comment and labels to PRs
// associated with release commits and issues (multipaged
// associatedPRs)" of success.test.js. Upstream reads the commit hash of
// the next page from an undefined variable, so this case throws a
// ReferenceError there; the port uses the hash of the commit. The
// upstream test has no routes for the commits of PRs #1, #2 and #5 and
// expects no comment on them, so here they have no release commit.
func TestSuccessMultipagedPRs(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			"commit123": commitPRs("123", true, "YE", prNode(1, "")),
			"commit456": commitPRs("456", false, "NI", prNode(2, "Fixes #3")),
			"commit789": commitPRs("789", false, "NI", prNode(5, "")),
		}),
		route{
			method:  http.MethodPost,
			path:    "/graphql",
			graphql: "getCommitAssociatedPRs",
			vars:    map[string]any{"owner": owner, "repo": repo, "sha": "123", "cursor": "YE"},
			response: map[string]any{"data": map[string]any{"repository": map[string]any{
				"commit": commitPRs("123", false, "NE", prNode(6, "")),
			}}},
		},
		prCommitsRoute(owner, repo, 1, "rebased_sha"),
		prRoute(1, "unrelated_sha"),
		prCommitsRoute(owner, repo, 6, "123"),
		prCommitsRoute(owner, repo, 2, "rebased_sha"),
		prRoute(2, "unrelated_sha"),
		prCommitsRoute(owner, repo, 5, "rebased_sha"),
		prRoute(5, "unrelated_sha"),
		relatedIssuesRoute(owner, repo, []int{1, 4}, map[string]any{"issue3": issueNode(3), "issue4": issueNode(4)}),
	)
	f.expect(commentRoutes(owner, repo, 6)...)
	f.expect(commentRoutes(owner, repo, 3)...)
	f.expect(commentRoutes(owner, repo, 4)...)
	f.expect(srIssuesRoute(owner, repo))

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.0.0",
		Commits: []forge.Commit{
			{Hash: "123", Message: "Commit 1 message\n\n Fix #1"},
			{Hash: "456", Message: "Commit 2 message"},
			{Hash: "789", Message: "Commit 3 message Closes https://github.com/" + owner + "/" + repo + "/issues/4"},
		},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	want := make([]string, 0, 6)
	want = append(want, commentLogs("PR", 6)...)
	want = append(want, commentLogs("issue", 3)...)
	want = append(want, commentLogs("issue", 4)...)
	assert.DeepEqual(t, logs, want)
}

// TestSuccessManyCommits ports "Make multiple search queries if
// necessary" of success.test.js.
func TestSuccessManyCommits(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	commits := make([]forge.Commit, 0, 7)
	for _, c := range "abcdefg" {
		commits = append(commits, forge.Commit{Hash: strings.Repeat(string(c), 40), Message: "Commit message"})
	}
	alias := func(i int) string { return "commit" + commits[i].Hash[:12] }
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			alias(0): commitPRs(commits[0].Hash, false, "NI", prNode(1, "")),
			alias(1): commitPRs(commits[1].Hash, false, "NI", prNode(2, "")),
			alias(2): commitPRs(commits[2].Hash, false, "NI", prNode(3, "")),
			alias(4): commitPRs(commits[4].Hash, false, "NI", prNode(4, "")),
			alias(5): commitPRs(commits[5].Hash, false, "NI", prNode(5, "")),
			alias(6): commitPRs(commits[6].Hash, false, "NI", prNode(6, "")),
		}),
	)
	for n := 1; n <= 6; n++ {
		f.expect(prCommitsRoute(owner, repo, n, commits[n-1].Hash))
	}
	var want []string
	for n := 1; n <= 6; n++ {
		f.expect(commentRoutes(owner, repo, n)...)
		want = append(want, commentLogs("PR", n)...)
	}
	f.expect(srIssuesRoute(owner, repo))

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.0.0", Commits: commits, Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, want)
}

// TestSuccessChunks checks that the commits go to GraphQL in chunks of
// 100, as inChunks does: 101 commits make two queries.
func TestSuccessChunks(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	commits := make([]forge.Commit, 0, 101)
	for i := range 101 {
		commits = append(commits, forge.Commit{Hash: fmt.Sprintf("%040d", i), Message: "Commit message"})
	}
	chunk := func(n int) func(t *testing.T, r *http.Request, body []byte) {
		return func(t *testing.T, _ *http.Request, body []byte) {
			t.Helper()
			assert.Check(t, cmp.Equal(strings.Count(string(body), ": object(oid: "), n))
		}
	}
	first := associatedPRsRoute(owner, repo, map[string]any{})
	first.check = chunk(100)
	second := associatedPRsRoute(owner, repo, map[string]any{})
	second.check = chunk(1)
	f := newFakeGitHub(t)
	f.expect(repoRoute(owner+"/"+repo), first, second, srIssuesRoute(owner, repo))

	var logs []string
	assert.NilError(t, f.client(&logs).Success(context.Background(), &forge.Success{Version: "1.0.0", Commits: commits}))
}

// TestSuccessUnrelatedPR ports "Do not add comment and labels for
// unrelated PR returned by search (compare sha and merge_commit_sha)" of
// success.test.js.
func TestSuccessUnrelatedPR(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			"commit123": commitPRs("123", false, "NI", prNode(1, "")),
			"commit456": commitPRs("456", false, "NI", prNode(2, "")),
		}),
		prCommitsRoute(owner, repo, 1, "rebased_sha"),
		prRoute(1, "123"),
		prCommitsRoute(owner, repo, 2, "rebased_sha"),
		prRoute(2, "unrelated_sha"),
	)
	f.expect(commentRoutes(owner, repo, 1)...)
	f.expect(srIssuesRoute(owner, repo))

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version:  "1.0.0",
		Commits:  []forge.Commit{{Hash: "123", Message: "Commit 1 message"}, {Hash: "456", Message: "Commit 2 message"}},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, commentLogs("PR", 1))
}

// TestSuccessNoPR ports "Do not add comment and labels if no PR is
// associated with release commits" of success.test.js.
func TestSuccessNoPR(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{"commit123": commitPRs("123", false, "NI")}),
		srIssuesRoute(owner, repo),
	)
	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version:  "1.0.0",
		Commits:  []forge.Commit{{Hash: "123", Message: "Commit 1 message"}},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	assert.Assert(t, cmp.Len(logs, 0))
}

// TestSuccessNoCommits ports "Do not add comment and labels if no
// commits is found for release" of success.test.js. Upstream also logs
// a deprecation warning about the successComment option there, which
// our config does not have.
func TestSuccessNoCommits(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(repoRoute(owner+"/"+repo), srIssuesRoute(owner, repo))
	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.1.0", Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, []string{"No commits found in release", "Skip commenting on issues and pull requests."})
}

// TestSuccessOtherRepo ports "Do not add comment and labels to PR/issues
// from other repo" of success.test.js.
func TestSuccessOtherRepo(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			"commit123": commitPRs("123", false, "NI"),
			"commit456": commitPRs("456", false, "NI"),
			"commit789": commitPRs("789", false, "NI"),
		}),
		relatedIssuesRoute(owner, repo, []int{2}, map[string]any{"issue2": issueNode(2)}),
	)
	f.expect(commentRoutes(owner, repo, 2)...)
	f.expect(srIssuesRoute(owner, repo))

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.0.0",
		Commits: []forge.Commit{
			{Hash: "123", Message: "Commit 1 message\n\n Fix other/other#1"},
			{Hash: "456", Message: "Commit 2 message Fix " + owner + "/" + repo + "#2"},
			{Hash: "789", Message: "Commit 3 message Closes other/other#3"},
		},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, commentLogs("issue", 2))
}

// TestSuccessMissingAndForbidden ports "Ignore missing and forbidden
// issues/PRs" of success.test.js: a 404 or 403 on a comment is logged
// and is not an error.
func TestSuccessMissingAndForbidden(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			"commit123": commitPRs("123", false, "NI", prNode(1, "")),
			"commit456": commitPRs("456", false, "NI", prNode(2, "Fixes #4")),
			"commit789": commitPRs("789", false, "NI", prNode(3, "Fixes #5")),
		}),
		prCommitsRoute(owner, repo, 1, "123"),
		prCommitsRoute(owner, repo, 2, "456"),
		prCommitsRoute(owner, repo, 3, "789"),
		relatedIssuesRoute(owner, repo, []int{4, 5, 1}, map[string]any{
			"issue4": issueNode(4), "issue5": issueNode(5), "issue1": issueNode(1),
		}),
	)
	f.expect(commentRoutes(owner, repo, 1)...)
	f.expect(
		route{method: http.MethodPost, path: "/repos/test_user/test_repo/issues/2/comments", status: http.StatusNotFound},
		route{method: http.MethodPost, path: "/repos/test_user/test_repo/issues/3/comments", status: http.StatusForbidden},
	)
	f.expect(commentRoutes(owner, repo, 4)...)
	f.expect(commentRoutes(owner, repo, 5)...)
	f.expect(srIssuesRoute(owner, repo))

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.0.0",
		Commits: []forge.Commit{
			{Hash: "123", Message: "Commit 1 message\n\n Fix #1"},
			{Hash: "456", Message: "Commit 2 message"},
			{Hash: "789", Message: "Commit 3 message"},
		},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	want := commentLogs("PR", 1)
	want = append(want,
		"Failed to add a comment to the issue/PR #2 as it doesn't exist.",
		"Not allowed to add a comment to the issue/PR #3.")
	want = append(want, commentLogs("issue", 4)...)
	want = append(want, commentLogs("issue", 5)...)
	assert.DeepEqual(t, logs, want)
}

// TestSuccessErrors ports "Ignore errors when adding comments and
// closing issues" of success.test.js: the errors come back together at
// the end, and each wraps forge.ErrForge.
func TestSuccessErrors(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{
			"commit123": commitPRs("123", false, "NI", prNode(1, "")),
			"commit456": commitPRs("456", false, "NI", prNode(2, "")),
		}),
		prCommitsRoute(owner, repo, 1, "123"),
		prCommitsRoute(owner, repo, 2, "456"),
		route{method: http.MethodPost, path: "/repos/test_user/test_repo/issues/1/comments", status: http.StatusBadRequest},
	)
	f.expect(commentRoutes(owner, repo, 2)...)
	f.expect(
		srIssuesRoute(owner, repo, failureIssues()...),
		closeRoute(2, http.StatusInternalServerError),
		closeRoute(3, 0),
	)

	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version:  "1.0.0",
		Commits:  []forge.Commit{{Hash: "123", Message: "Commit 1 message"}, {Hash: "456", Message: "Commit 2 message"}},
		Releases: []forge.Published{githubRelease},
	})
	var joined interface{ Unwrap() []error }
	assert.Assert(t, errors.As(err, &joined), err)
	errs := joined.Unwrap()
	assert.Assert(t, cmp.Len(errs, 2))
	assert.Check(t, errors.Is(errs[0], forge.ErrForge))
	assert.Check(t, cmp.Equal(statusCode(errs[0]), http.StatusBadRequest))
	assert.Check(t, errors.Is(errs[1], forge.ErrForge))
	assert.Check(t, cmp.Equal(statusCode(errs[1]), http.StatusInternalServerError))
	want := make([]string, 0, 5)
	want = append(want, "Failed to add a comment to the issue/PR #1.")
	want = append(want, commentLogs("PR", 2)...)
	want = append(want, "Failed to close the issue #2.", "Closed issue #3: https://github.com/issues/3.")
	assert.DeepEqual(t, logs, want)
}

// TestSuccessCloseIssues ports "Close open issues when a release is
// successful" of success.test.js.
func TestSuccessCloseIssues(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{"commit123": commitPRs("123", false, "NI")}),
		srIssuesRoute(owner, repo, failureIssues()...),
		closeRoute(2, 0),
		closeRoute(3, 0),
	)
	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version:  "1.0.0",
		Commits:  []forge.Commit{{Hash: "123", Message: "Commit 1 message"}},
		Releases: []forge.Published{githubRelease},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, []string{"Closed issue #2: https://github.com/issues/2.", "Closed issue #3: https://github.com/issues/3."})
}

// TestSuccessChannelLabel checks the default releasedLabels of
// resolve-config.js on a channel: "released on @<channel>".
func TestSuccessChannelLabel(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		associatedPRsRoute(owner, repo, map[string]any{"commit123": commitPRs("123", false, "NI", prNode(1, ""))}),
		prCommitsRoute(owner, repo, 1, "123"),
	)
	comments := commentRoutes(owner, repo, 1)
	comments[1].body = []string{"released on @next"}
	f.expect(comments...)
	f.expect(srIssuesRoute(owner, repo))
	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "2.0.0-next.1", Channel: "next", Commits: []forge.Commit{{Hash: "123", Message: "feat: x"}},
	})
	assert.NilError(t, err)
}

// TestSuccessGraphQLErrors checks that a GraphQL response with errors
// fails, as @octokit/graphql throws a GraphqlResponseError for it.
func TestSuccessGraphQLErrors(t *testing.T) {
	const owner, repo = "test_user", "test_repo"
	f := newFakeGitHub(t)
	failing := associatedPRsRoute(owner, repo, nil)
	failing.response = map[string]any{"data": nil, "errors": []map[string]any{{"message": "Something went wrong"}}}
	f.expect(repoRoute(owner+"/"+repo), failing)
	var logs []string
	err := f.client(&logs).Success(context.Background(), &forge.Success{
		Version: "1.0.0", Commits: []forge.Commit{{Hash: "123", Message: "m"}},
	})
	assert.Assert(t, errors.Is(err, forge.ErrForge), err)
	assert.ErrorContains(t, err, "Request failed due to following response errors:\n - Something went wrong")
}

// TestFindSRIssues ports the cases of find-sr-issue.test.js: only the
// open issues with the ISSUE_ID marker count.
func TestFindSRIssues(t *testing.T) {
	tests := []struct {
		// name is the upstream test title.
		name string

		// issues are the open issues with the labels.
		issues []map[string]any

		// want are the expected issues.
		want []srIssue
	}{
		{
			name:   "Filter out issues without ID",
			issues: failureIssues(),
			want: []srIssue{
				{Number: 2, Body: "Issue 2 body\n\n<!-- semantic-release:github -->"},
				{Number: 3, Body: "Issue 3 body\n\n<!-- semantic-release:github -->"},
			},
		},
		{name: "Return empty array if not issues found"},
		{
			name: "Return empty array if not issues has matching ID",
			issues: []map[string]any{
				{"number": 1, "body": "Issue 1 body", "title": failTitle},
				{"number": 2, "body": "Issue 2 body", "title": failTitle},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			f.expect(srIssuesRoute("test_user", "test_repo", tc.issues...))
			var logs []string
			got, err := f.client(&logs).findSRIssues(context.Background(), "test_user", "test_repo")
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tc.want)
		})
	}
}

// TestSuccessComment ports get-success-comment.test.js.
func TestSuccessComment(t *testing.T) {
	const footer = "\n\nYour **[semantic-release](https://github.com/semantic-release/semantic-release)** bot :package::rocket:"
	npm := forge.Published{Name: "npm release", URL: "https://npm.com/release"}
	tests := []struct {
		// name is the upstream test title.
		name string

		// pr is true for a pull request.
		pr bool

		// releases are the releaseInfos.
		releases []forge.Published

		// want is the expected comment.
		want string
	}{
		{
			name:     "Comment for issue with multiple releases",
			releases: []forge.Published{githubRelease, npm},
			want: ":tada: This issue has been resolved in version 1.0.0 :tada:\n\nThe release is available on:\n" +
				"- [GitHub release](https://github.com/release)\n- [npm release](https://npm.com/release)" + footer,
		},
		{
			name:     "Comment for PR with multiple releases",
			pr:       true,
			releases: []forge.Published{githubRelease, npm},
			want: ":tada: This PR is included in version 1.0.0 :tada:\n\nThe release is available on:\n" +
				"- [GitHub release](https://github.com/release)\n- [npm release](https://npm.com/release)" + footer,
		},
		{
			name:     "Comment with missing release URL",
			releases: []forge.Published{githubRelease, {Name: "npm release"}},
			want: ":tada: This issue has been resolved in version 1.0.0 :tada:\n\nThe release is available on:\n" +
				"- [GitHub release](https://github.com/release)\n- `npm release`" + footer,
		},
		{
			name:     "Comment with one release",
			releases: []forge.Published{githubRelease},
			want: ":tada: This issue has been resolved in version 1.0.0 :tada:\n\n" +
				"The release is available on [GitHub release](https://github.com/release)" + footer,
		},
		{
			name: "Comment with no release object",
			want: ":tada: This issue has been resolved in version 1.0.0 :tada:" + footer,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i := &issue{Number: 1}
			if tc.pr {
				i.Typename = "PullRequest"
			}
			assert.Equal(t, successComment(i, tc.releases, "", "1.0.0"), tc.want)
		})
	}
}

// TestSuccessCommentUnit checks that the comment of a unit release
// names the unit.
func TestSuccessCommentUnit(t *testing.T) {
	i := &issue{Number: 1, Typename: "PullRequest"}
	got := successComment(i, nil, "@acme/widgets-client", "2.4.1")
	want := ":tada: This PR is included in `@acme/widgets-client` version 2.4.1 :tada:"
	assert.Assert(t, strings.HasPrefix(got, want+"\n\n"), got)
}

// TestQueries checks the GraphQL queries against the text of the
// queries that success.js and find-sr-issues.js send, in testdata.
func TestQueries(t *testing.T) {
	for file, got := range map[string]string{
		"getAssociatedPRs.graphql":       associatedPRsQuery([]string{"123", "4567890abcdef0123"}),
		"getRelatedIssues.graphql":       relatedIssuesQuery([]int{3}),
		"getCommitAssociatedPRs.graphql": commitAssociatedPRsQuery,
		"getSRIssues.graphql":            srIssuesQuery,
	} {
		want, err := os.ReadFile("testdata/" + file)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(got, string(want)), file)
	}
}
