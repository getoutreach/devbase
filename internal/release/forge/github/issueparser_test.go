// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of the issue-parser port, against the output of
// issue-parser 7.0.2 in Node.js.

package github

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// TestParseCloseActions checks the close actions of texts against the
// actions.close of issueParser("github", {}) in Node.js, as [slug, issue]
// pairs.
func TestParseCloseActions(t *testing.T) {
	for text, want := range map[string][][2]string{
		"Fix #1":                                    {{"", "1"}},
		"Commit 1 message\n\n Fix #1":               {{"", "1"}},
		"Closes https://github.com/o/r/issues/4":    {{"o/r", "4"}},
		"fixes o/r#2":                               {{"o/r", "2"}},
		"Fix other/other#1":                         {{"other/other", "1"}},
		"`Fix #1` and fixes #2":                     {{"", "2"}},
		"```\nFix #1\n```\nfix #2":                  {{"", "2"}},
		"  ```js\nfix #3\n  ```\nresolved #4":       {{"", "4"}},
		"<!-- fix #1 --> fix #2":                    {{"", "2"}},
		"<code>fix #1</code> resolves #2":           {{"", "2"}},
		"Duplicate of #3":                           nil,
		"fix gh-5":                                  {{"", "5"}},
		"Fixes: #6":                                 {{"", "6"}},
		"closes #7abc":                              nil,
		"FIXES #8, fixes #9":                        {{"", "8"}, {"", "9"}},
		"fix #1 fix #1":                             {{"", "1"}, {"", "1"}},
		"close #10\rclose #11":                      {{"", "10"}, {"", "11"}},
		"fix\t#12":                                  {{"", "12"}},
		"fixes https://github.com/o/r/pull/13":      {{"o/r", "13"}},
		"resolve #14.":                              {{"", "14"}},
		"fixed#15":                                  nil,
		"xfix #16":                                  nil,
		"fix: o/r#17":                               {{"o/r", "17"}},
		"fixes @user #18":                           nil,
		"\\`fix #19\\` `x` fix #20":                 {{"", "19"}, {"", "20"}},
		"fix #21 fix #22":                           {{"", "21"}, {"", "22"}},
		"````\nfix #1\n```\nfix #2\n````\nfix #3":   {{"", "3"}},
		"Closes HTTPS://GITHUB.COM/o/r/issues/23":   {{"o/r", "23"}},
		"cloſe #24":                                 nil,
		"fix https://gitlab.com/o/r/issues/25":      nil,
		"fix #26\r\n```\r\nfix #27\r\n```":          {{"", "26"}},
		"<code><code>fix #28</code> fix #29</code>": {{"", "29"}},
		"fix a/b/c#30":                              {{"a/b/c", "30"}},
		"fix #31_":                                  nil,
		"Closes #32 and #33":                        {{"", "32"}},
	} {
		var got [][2]string
		for _, a := range parseCloseActions(text) {
			got = append(got, [2]string{a.slug, a.issue})
		}
		assert.Check(t, cmp.DeepEqual(got, want), text)
	}
}
