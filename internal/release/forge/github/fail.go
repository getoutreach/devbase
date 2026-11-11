// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The failure issue of a branch, as a port of fail.js,
// find-sr-issues.js and get-fail-comment.js of @semantic-release/github
// with the default options.

package github

import (
	"context"
	"slices"
	"strings"

	gh "github.com/google/go-github/v84/github"
)

// issueID marks the issues that the plugin opens (ISSUE_ID).
const issueID = "<!-- semantic-release:github -->"

// failLabel is the label of the failure issue (RELEASE_FAIL_LABEL).
const failLabel = "semantic-release"

// failTitle is the default title of the failure issue.
const failTitle = "The automated release is failing 🚨"

// failLabels returns the labels of the failure issue: the default
// labels option, ["semantic-release"], and RELEASE_FAIL_LABEL, which
// fail.js and find-sr-issues.js concatenate, so the label is there two
// times.
func failLabels() []string {
	return []string{"semantic-release", failLabel}
}

// srIssue is an open issue of findSRIssues.
type srIssue struct {
	// Number is the issue number.
	Number int `json:"number"`

	// Body is the issue description.
	Body string `json:"body"`
}

// srIssuesQuery is the getSRIssues query of find-sr-issues.js.
const srIssuesQuery = `#graphql
  query getSRIssues($owner: String!, $repo: String!, $filter: IssueFilters) {
    repository(owner: $owner, name: $repo) {
      issues(first: 100, states: OPEN, filterBy: $filter) {
        nodes {
          number
          title
          body
        }
      }
    }
  }
`

// Fail comments on the open failure issue of the repository with errs,
// or opens one, as fail.js does with the default options. An error has
// only a message, err.Error(): Go errors have no equivalent of the
// details and pluginName of a SemanticReleaseError.
func (c *Client) Fail(ctx context.Context, branch string, errs []error) error {
	owner, repo, err := c.repository(ctx)
	if err != nil {
		return err
	}
	body := failComment(branch, errs)
	issues, err := c.findSRIssues(ctx, owner, repo)
	if err != nil {
		return err
	}

	if len(issues) > 0 {
		number := issues[0].Number
		c.logf("Found existing semantic-release issue #%d.", number)
		comment, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, &gh.IssueComment{Body: gh.Ptr(body)})
		if err != nil {
			return apiError("comment on the failure issue", err)
		}
		c.logf("Added comment to issue #%d: %s.", number, comment.GetHTMLURL())
		return nil
	}

	labels := failLabels()
	req := &gh.IssueRequest{Title: gh.Ptr(failTitle), Body: gh.Ptr(body + "\n\n" + issueID), Labels: &labels}
	created, _, err := c.gh.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return apiError("create the failure issue", err)
	}
	c.logf("Created issue #%d: %s.", created.GetNumber(), created.GetHTMLURL())
	return nil
}

// findSRIssues returns the open issues with the failure labels whose
// description has the ISSUE_ID marker, as find-sr-issues.js does.
func (c *Client) findSRIssues(ctx context.Context, owner, repo string) ([]srIssue, error) {
	var data struct {
		Repository struct {
			Issues struct {
				Nodes []srIssue `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": owner, "repo": repo, "filter": map[string]any{"labels": failLabels()}}
	if err := c.graphql(ctx, srIssuesQuery, vars, &data); err != nil {
		return nil, err
	}
	var issues []srIssue
	for _, i := range data.Repository.Issues.Nodes {
		if !strings.Contains(i.Body, issueID) {
			continue
		}
		if !slices.ContainsFunc(issues, func(o srIssue) bool { return o.Number == i.Number }) {
			issues = append(issues, i)
		}
	}
	return issues, nil
}

// failComment returns the default failure comment of
// get-fail-comment.js, with err.Error() as the message of each error and
// no details. It leaves out nil errors.
func failComment(branch string, errs []error) string {
	const (
		faqURL      = homeURL + "/blob/master/docs/support/FAQ.md"
		getHelpURL  = homeURL + "#get-help"
		usageDocURL = homeURL + "/blob/master/docs/usage/README.md"
		newIssueURL = homeURL + "/issues/new"
	)
	formatted := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			formatted = append(formatted, "### "+err.Error()+"\n\n"+
				"Unfortunately this error doesn't have any additional information.")
		}
	}
	return "## :rotating_light: The automated release from the `" + branch + "` branch failed. :rotating_light:\n" +
		"\n" +
		"I recommend you give this issue a high priority, so other packages depending on you can benefit from your bug fixes " +
		"and new features again.\n" +
		"\n" +
		"You can find below the list of errors reported by **semantic-release**. Each one of them has to be resolved in order " +
		"to automatically publish your package. I’m sure you can fix this 💪.\n" +
		"\n" +
		"Errors are usually caused by a misconfiguration or an authentication problem. With each error reported below you " +
		"will find explanation and guidance to help you to resolve it.\n" +
		"\n" +
		"Once all the errors are resolved, **semantic-release** will release your package the next time you push a commit " +
		"to the `" + branch + "` branch. You can also manually restart the failed CI job that runs **semantic-release**.\n" +
		"\n" +
		"If you are not sure how to resolve this, here are some links that can help you:\n" +
		"- [Usage documentation](" + usageDocURL + ")\n" +
		"- [Frequently Asked Questions](" + faqURL + ")\n" +
		"- [Support channels](" + getHelpURL + ")\n" +
		"\n" +
		"If those don’t help, or if this issue is reporting something you think isn’t right, you can always ask the humans " +
		"behind **[semantic-release](" + newIssueURL + ")**.\n" +
		"\n" +
		"---\n" +
		"\n" +
		strings.Join(formatted, "\n\n---\n\n") + "\n" +
		"\n" +
		"---\n" +
		"\n" +
		"Good luck with your project ✨\n" +
		"\n" +
		"Your **[semantic-release](" + homeURL + ")** bot :package::rocket:"
}
