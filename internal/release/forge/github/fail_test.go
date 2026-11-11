// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of Fail, ported from test/fail.test.js and
// test/get-fail-comment.test.js of @semantic-release/github 12.0.10.

package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// errorsRegexp matches the error list of three errors in a failure
// comment. Upstream it has the details of each error, which Go errors
// do not have.
var errorsRegexp = regexp.MustCompile("---\n\n### Error message 1\n\n" + noDetails + "\n\n---\n\n" +
	"### Error message 2\n\n" + noDetails + "\n\n---\n\n### Error message 3\n\n" + noDetails + "\n\n---")

// noDetails is the text of an error without details.
const noDetails = "Unfortunately this error doesn't have any additional information."

// These are the errors of the failure tests.
var (
	// errMessage1 is the first error.
	errMessage1 = errors.New("Error message 1")

	// errMessage2 is the second error.
	errMessage2 = errors.New("Error message 2")

	// errMessage3 is the third error.
	errMessage3 = errors.New("Error message 3")
)

// threeErrors are the errors of the failure tests.
func threeErrors() []error {
	return []error{errMessage1, errMessage2, errMessage3}
}

// TestFailNewIssue ports "Open a new issue with the list of errors" of
// fail.test.js, with a renamed repository.
func TestFailNewIssue(t *testing.T) {
	const owner, repo = "test_user_2", "test_repo_2"
	f := newFakeGitHub(t)
	f.expect(
		repoRoute(owner+"/"+repo),
		srIssuesRoute(owner, repo),
		route{
			method: http.MethodPost,
			path:   "/repos/test_user_2/test_repo_2/issues",
			check: func(t *testing.T, _ *http.Request, body []byte) {
				t.Helper()
				var data map[string]any
				assert.Check(t, json.Unmarshal(body, &data))
				assert.Check(t, cmp.Equal(data["title"], "The automated release is failing 🚨"))
				assert.Check(t, cmp.Regexp(errorsRegexp, data["body"].(string)))
				assert.Check(t, cmp.Regexp("\n\n"+regexp.QuoteMeta(issueID)+"$", data["body"].(string)))
				assert.Check(t, cmp.DeepEqual(data["labels"], []any{"semantic-release", "semantic-release"}))
				assert.Check(t, cmp.Len(data, 3))
			},
			status:   http.StatusCreated,
			response: map[string]any{"html_url": "https://github.com/issues/1", "number": 1},
		},
	)
	var logs []string
	err := f.client(&logs).Fail(context.Background(), "master", threeErrors())
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, []string{"Created issue #1: https://github.com/issues/1."})
}

// TestFailExistingIssue ports "Update the first existing issue with the
// list of errors" of fail.test.js.
func TestFailExistingIssue(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(
		repoRoute("test_user/test_repo"),
		srIssuesRoute("test_user", "test_repo", failureIssues()...),
		route{
			method: http.MethodPost,
			path:   "/repos/test_user/test_repo/issues/2/comments",
			check: func(t *testing.T, _ *http.Request, body []byte) {
				t.Helper()
				var data map[string]any
				assert.Check(t, json.Unmarshal(body, &data))
				assert.Check(t, cmp.Regexp(errorsRegexp, data["body"].(string)))
				assert.Check(t, cmp.Len(data, 1))
			},
			response: map[string]any{"html_url": "https://github.com/issues/2", "number": 2},
		},
	)
	var logs []string
	err := f.client(&logs).Fail(context.Background(), "master", threeErrors())
	assert.NilError(t, err)
	assert.DeepEqual(t, logs, []string{
		"Found existing semantic-release issue #2.",
		"Added comment to issue #2: https://github.com/issues/2.",
	})
}

// TestFailError checks that a failed API call wraps forge.ErrForge.
func TestFailError(t *testing.T) {
	f := newFakeGitHub(t)
	f.expect(route{method: http.MethodGet, path: "/repos/test_user/test_repo", status: http.StatusNotFound})
	var logs []string
	err := f.client(&logs).Fail(context.Background(), "master", threeErrors())
	assert.Assert(t, errors.Is(err, forge.ErrForge), err)
}

// TestFailComment ports get-fail-comment.test.js. Go errors have no
// details and no pluginName, so every error has the text of "Comment
// with missing error details and no pluginName"; the full text is
// checked against the output of get-fail-comment.js in testdata.
func TestFailComment(t *testing.T) {
	t.Run("Comment with mutiple errors", func(t *testing.T) {
		comment := failComment("master", threeErrors())
		assert.Check(t, cmp.Regexp("the `master` branch", comment))
		assert.Check(t, cmp.Regexp(errorsRegexp, comment))
	})
	t.Run("Comment with one error", func(t *testing.T) {
		comment := failComment("master", []error{errMessage1})
		assert.Check(t, cmp.Regexp("the `master` branch", comment))
		assert.Check(t, cmp.Regexp("---\n\n### Error message 1\n\n"+noDetails+"\n\n---", comment))
	})
	t.Run("Comment with missing error details and no pluginName", func(t *testing.T) {
		comment := failComment("master", []error{errMessage1})
		assert.Check(t, cmp.Regexp("the `master` branch", comment))
		assert.Check(t, cmp.Regexp("---\n\n### Error message 1\n\n"+noDetails+"\n\n---", comment))
	})
	t.Run("full text", func(t *testing.T) {
		want, err := os.ReadFile("testdata/fail-comment.md")
		assert.NilError(t, err)
		assert.Equal(t, failComment("master", []error{errMessage1, nil, errMessage2}), string(want))
	})
}
