// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Comments on the pull requests and issues of a release, as
// a port of success.js and get-success-comment.js of
// @semantic-release/github with the default options.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	gh "github.com/google/go-github/v84/github"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// chunkSize is the number of commits or issues in one GraphQL query.
const chunkSize = 100

// homeURL is the semantic-release URL in the comments.
const homeURL = "https://github.com/semantic-release/semantic-release"

// issue is an issue or a pull request, with the fields that the default
// options use of the issue that buildIssuesOrPRsFromResponseNode builds.
type issue struct {
	// Typename is "PullRequest" for a pull request.
	Typename string `json:"__typename"`

	// Number is the issue or pull request number.
	Number int `json:"number"`

	// Body is the description.
	Body string `json:"body"`
}

// pullRequest reports whether the issue is a pull request, as
// pull_request is set for one.
func (i *issue) pullRequest() bool {
	return i.Typename == "PullRequest"
}

// associatedPRs is the associatedPullRequests field of a commit.
type associatedPRs struct {
	// PageInfo tells whether there are more pages.
	PageInfo struct {
		// EndCursor is the cursor of the next page.
		EndCursor string `json:"endCursor"`

		// HasNextPage is true when there are more pages.
		HasNextPage bool `json:"hasNextPage"`
	} `json:"pageInfo"`

	// Nodes are the pull requests.
	Nodes []issue `json:"nodes"`
}

// commitNode is a commit of the getAssociatedPRs query.
type commitNode struct {
	// OID is the commit hash.
	OID string `json:"oid"`

	// AssociatedPullRequests are the pull requests of the commit.
	AssociatedPullRequests *associatedPRs `json:"associatedPullRequests"`
}

// Success comments on and labels the pull requests and issues of a
// release, and closes the open failure issues, as success.js does with
// the default options. The comments go out one after another, not in
// parallel. A comment that fails with 403 or 404 is logged and skipped;
// the other failures are returned together at the end.
func (c *Client) Success(ctx context.Context, s *forge.Success) error {
	owner, repo, err := c.repository(ctx)
	if err != nil {
		return err
	}

	var errs []error
	if len(s.Commits) == 0 {
		c.logf("No commits found in release")
		c.logf("Skip commenting on issues and pull requests.")
	} else {
		errs, err = c.commentOnIssues(ctx, owner, repo, s)
		if err != nil {
			return err
		}
	}

	srIssues, err := c.findSRIssues(ctx, owner, repo)
	if err != nil {
		return err
	}
	for _, i := range srIssues {
		r, _, err := c.gh.Issues.Edit(ctx, owner, repo, i.Number, &gh.IssueRequest{State: gh.Ptr("closed")})
		if err != nil {
			errs = append(errs, apiError(fmt.Sprintf("close issue #%d", i.Number), err))
			c.logf("Failed to close the issue #%d.", i.Number)
			continue
		}
		c.logf("Closed issue #%d: %s.", i.Number, r.GetHTMLURL())
	}
	return errors.Join(errs...)
}

// commentOnIssues comments on and labels the pull requests of the
// release commits and the issues that they close. It returns the errors
// of the comments to report at the end, or an error that stops Success.
func (c *Client) commentOnIssues(ctx context.Context, owner, repo string, s *forge.Success) ([]error, error) {
	shas := make([]string, len(s.Commits))
	for i, commit := range s.Commits {
		shas[i] = commit.Hash
	}
	var releaseInfos []forge.Published
	for _, r := range s.Releases {
		if r.Name != "" {
			releaseInfos = append(releaseInfos, r)
		}
	}

	associated, err := c.associatedPRs(ctx, owner, repo, shas)
	if err != nil {
		return nil, err
	}
	var prs []issue
	for _, pr := range uniqueIssues(associated) {
		ok, err := c.isReleasePR(ctx, owner, repo, pr.Number, shas)
		if err != nil {
			return nil, err
		}
		if ok {
			prs = append(prs, pr)
		}
	}

	// "Parse the release commits message and PRs body to find resolved
	// issues/PRs via comment keyworkds"
	messages := make([]string, 0, len(prs)+len(s.Commits))
	for _, pr := range prs {
		messages = append(messages, pr.Body)
	}
	for _, commit := range s.Commits {
		messages = append(messages, commit.Message)
	}
	var numbers []int
	for _, msg := range messages {
		if msg == "" {
			continue
		}
		for _, a := range parseCloseActions(msg) {
			if a.slug != "" && a.slug != owner+"/"+repo {
				continue
			}
			if n, err := strconv.Atoi(a.issue); err == nil && !slices.Contains(numbers, n) {
				numbers = append(numbers, n)
			}
		}
	}
	issues, err := c.relatedIssues(ctx, owner, repo, numbers)
	if err != nil {
		return nil, err
	}

	label := "released"
	if s.Channel != "" {
		label += " on @" + s.Channel
	}
	var errs []error
	for _, i := range uniqueIssues(append(prs, issues...)) {
		if err := c.commentOnIssue(ctx, owner, repo, &i, successComment(&i, releaseInfos, s.Version), label); err != nil {
			errs = append(errs, err)
		}
	}
	return errs, nil
}

// commentOnIssue adds the success comment and the released label to an
// issue or pull request. It logs and skips a 403 or 404 error, and
// returns the other errors.
func (c *Client) commentOnIssue(ctx context.Context, owner, repo string, i *issue, body, label string) error {
	kind := "issue"
	if i.pullRequest() {
		kind = "PR"
	}
	err := func() error {
		comment, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, i.Number, &gh.IssueComment{Body: gh.Ptr(body)})
		if err != nil {
			return err
		}
		c.logf("Added comment to %s #%d: %s", kind, i.Number, comment.GetHTMLURL())
		if _, _, err := c.gh.Issues.AddLabelsToIssue(ctx, owner, repo, i.Number, []string{label}); err != nil {
			return err
		}
		c.logf("Added labels %q to %s #%d", []string{label}, kind, i.Number)
		return nil
	}()
	switch status := statusCode(err); {
	case err == nil:
		return nil
	case status == http.StatusForbidden:
		c.logf("Not allowed to add a comment to the issue/PR #%d.", i.Number)
		return nil
	case status == http.StatusNotFound:
		c.logf("Failed to add a comment to the issue/PR #%d as it doesn't exist.", i.Number)
		return nil
	default:
		c.logf("Failed to add a comment to the issue/PR #%d.", i.Number)
		return apiError(fmt.Sprintf("comment on issue/PR #%d", i.Number), err)
	}
}

// associatedPRs returns the pull requests of the commits, in chunks of
// 100 commits, with the further pages of a commit that has more than
// 100 pull requests.
func (c *Client) associatedPRs(ctx context.Context, owner, repo string, shas []string) ([]issue, error) {
	var prs []issue
	for chunk := range slices.Chunk(shas, chunkSize) {
		var data struct {
			Repository json.RawMessage `json:"repository"`
		}
		vars := map[string]any{"owner": owner, "repo": repo}
		if err := c.graphql(ctx, associatedPRsQuery(chunk), vars, &data); err != nil {
			return nil, err
		}
		commits, err := orderedValues(data.Repository)
		if err != nil {
			return nil, err
		}
		for _, raw := range commits {
			var commit commitNode
			if err := json.Unmarshal(raw, &commit); err != nil {
				return nil, fmt.Errorf("decode associated pull requests: %w", err)
			}
			page := commit.AssociatedPullRequests
			if page == nil || len(page.Nodes) == 0 {
				continue
			}
			prs = append(prs, page.Nodes...)
			more, err := c.morePRs(ctx, owner, repo, commit.OID, page)
			if err != nil {
				return nil, err
			}
			prs = append(prs, more...)
		}
	}
	return prs, nil
}

// morePRs returns the pull requests of the pages of a commit after page.
// Upstream reads the commit hash from an undefined variable here, which
// throws a ReferenceError; this uses the hash of the commit, as the
// upstream test "multipaged associatedPRs" expects.
func (c *Client) morePRs(ctx context.Context, owner, repo, sha string, page *associatedPRs) ([]issue, error) {
	var prs []issue
	for page.PageInfo.HasNextPage {
		var data struct {
			Repository struct {
				Commit commitNode `json:"commit"`
			} `json:"repository"`
		}
		vars := map[string]any{"owner": owner, "repo": repo, "sha": sha, "cursor": page.PageInfo.EndCursor}
		if err := c.graphql(ctx, commitAssociatedPRsQuery, vars, &data); err != nil {
			return nil, err
		}
		page = data.Repository.Commit.AssociatedPullRequests
		if page == nil {
			break
		}
		prs = append(prs, page.Nodes...)
	}
	return prs, nil
}

// isReleasePR reports whether a pull request has a commit of the
// release, or was merged as one.
func (c *Client) isReleasePR(ctx context.Context, owner, repo string, number int, shas []string) (bool, error) {
	opts := &gh.ListOptions{}
	for {
		commits, resp, err := c.gh.PullRequests.ListCommits(ctx, owner, repo, number, opts)
		if err != nil {
			return false, apiError(fmt.Sprintf("list commits of PR #%d", number), err)
		}
		for _, commit := range commits {
			if slices.Contains(shas, commit.GetSHA()) {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return false, apiError(fmt.Sprintf("get PR #%d", number), err)
	}
	return slices.Contains(shas, pr.GetMergeCommitSHA()), nil
}

// relatedIssues returns the issues and pull requests with numbers, in
// chunks of 100.
func (c *Client) relatedIssues(ctx context.Context, owner, repo string, numbers []int) ([]issue, error) {
	var issues []issue
	for chunk := range slices.Chunk(numbers, chunkSize) {
		var data struct {
			Repository json.RawMessage `json:"repository"`
		}
		vars := map[string]any{"owner": owner, "repo": repo}
		if err := c.graphql(ctx, relatedIssuesQuery(chunk), vars, &data); err != nil {
			return nil, err
		}
		nodes, err := orderedValues(data.Repository)
		if err != nil {
			return nil, err
		}
		for _, raw := range nodes {
			var i *issue
			if err := json.Unmarshal(raw, &i); err != nil {
				return nil, fmt.Errorf("decode related issues: %w", err)
			}
			if i != nil {
				issues = append(issues, *i)
			}
		}
	}
	return issues, nil
}

// uniqueIssues returns the first issue of each number, as uniqBy does.
func uniqueIssues(issues []issue) []issue {
	var out []issue
	for _, i := range issues {
		if !slices.ContainsFunc(out, func(o issue) bool { return o.Number == i.Number }) {
			out = append(out, i)
		}
	}
	return out
}

// orderedValues returns the values of a JSON object in their order, as
// Object.values does, or nothing for null.
func orderedValues(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == jsonNull {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("decode GraphQL object: %w", err)
	}
	var values []json.RawMessage
	for dec.More() {
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("decode GraphQL object key: %w", err)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("decode GraphQL object value: %w", err)
		}
		values = append(values, v)
	}
	return values, nil
}

// successComment returns the default success comment, as
// get-success-comment.js does.
func successComment(i *issue, releaseInfos []forge.Published, version string) string {
	what := "issue has been resolved"
	if i.pullRequest() {
		what = "PR is included"
	}
	var b strings.Builder
	fmt.Fprintf(&b, ":tada: This %s in version %s :tada:", what, version)
	switch len(releaseInfos) {
	case 0:
	case 1:
		b.WriteString("\n\nThe release is available on " + linkify(releaseInfos[0]))
	default:
		b.WriteString("\n\nThe release is available on:")
		for _, r := range releaseInfos {
			b.WriteString("\n- " + linkify(r))
		}
	}
	b.WriteString("\n\nYour **[semantic-release](" + homeURL + ")** bot :package::rocket:")
	return b.String()
}

// linkify returns a Markdown link to a release, or its name in code
// without a URL.
func linkify(r forge.Published) string {
	if r.URL != "" {
		return "[" + r.Name + "](" + r.URL + ")"
	}
	return "`" + r.Name + "`"
}

// baseFields are the fields of issues and pull requests in the GraphQL
// queries of success.js.
const baseFields = `
  __typename
  id
  title
  body
  url
  number
  createdAt
  updatedAt
  closedAt
  comments {
    totalCount
  }
  state
  author {
    login
    url
    avatarUrl
    __typename
  }
  authorAssociation
  activeLockReason
  labels(first: 40) {
    nodes {
      id
      url
      name
      color
      description
      isDefault
    }
  }
  milestone {
    url
    id
    number
    state
    title
    description
    creator {
      login
      url
      avatarUrl
    }
    createdAt
    closedAt
    updatedAt
  }
  locked
`

// relatedIssuesQuery returns the getRelatedIssues query of
// buildRelatedIssuesQuery.
func relatedIssuesQuery(numbers []int) string {
	var b strings.Builder
	for _, n := range numbers {
		fmt.Fprintf(&b, `issue%d: issueOrPullRequest(number: %d) {
              ... on Issue {
                %s
              }
              ... on PullRequest {
                %s
                mergeable
                changedFiles
                mergedAt
                isDraft
                mergedBy {
                  login
                  avatarUrl
                  url
                }
                commits {
                  totalCount
                }
              }
            }`, n, n, baseFields, baseFields)
	}
	return `#graphql
    query getRelatedIssues($owner: String!, $repo: String!) {
      repository(owner: $owner, name: $repo) {
        ` + b.String() + `
      }
    }
  `
}

// associatedPRsQuery returns the getAssociatedPRs query of
// buildAssociatedPRsQuery.
func associatedPRsQuery(shas []string) string {
	var b strings.Builder
	for _, sha := range shas {
		fmt.Fprintf(&b, `commit%s: object(oid: "%s") {
            ...on Commit {
              oid
              associatedPullRequests(first: 100) {
                pageInfo {
                  endCursor
                  hasNextPage
                }
                nodes {
                  %s
                  mergeable
                  changedFiles
                  mergedAt
                  isDraft
                  mergedBy {
                    login
                    avatarUrl
                    url
                  }
                  commits {
                    totalCount
                  }
                }
              }
            }
          }`, sha[:min(12, len(sha))], sha, baseFields)
	}
	return `#graphql
    query getAssociatedPRs($owner: String!, $repo: String!) {
      repository(owner: $owner, name: $repo) {
        ` + b.String() + `
      }
    }
  `
}

// commitAssociatedPRsQuery is the getCommitAssociatedPRs query of
// loadSingleCommitAssociatedPRs, for the pages after the first.
const commitAssociatedPRsQuery = `#graphql
  query getCommitAssociatedPRs($owner: String!, $repo: String!, $sha: String!, $cursor: String) {
    repository(owner: $owner, name: $repo) {
      commit: object(oid: $sha) {
        ...on Commit {
          oid
          associatedPullRequests(after: $cursor, first: 100) {
            pageInfo {
              endCursor
              hasNextPage
            }
            nodes {
              ` + baseFields + `
              mergeable
              changedFiles
              mergedAt
              isDraft
              mergedBy {
                login
                avatarUrl
                url
              }
              commits {
                totalCount
              }
            }
          }
        }
      }
    }
  }
`
