// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Ports of the conventional-commits-parser 6.4.0 and
// conventional-commits-filter 5.0.0 unit tests. The parser has the
// conventionalcommits 9.3.1 options, so a test written for other options
// uses the same input, or the input with the preset keywords, and the
// value that the Node.js parser returns with these options.

package release

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// The header and body lines that many CommitParser.spec.ts tests use.
const (
	specSubject = "broadcast $destroy event on scope destruction"
	specHeader  = "feat(scope): " + specSubject
	specBody1   = "perf testing shows that in chrome this change adds 5-15% overhead"
	specBody2   = "when destroying 10k nested scopes where each scope has a $destroy listener"
	specBody    = specBody1 + "\n" + specBody2
)

// specCommit sets the header fields that the parser gives for specHeader
// in c and returns c.
func specCommit(c *Commit) Commit {
	c.Header = specHeader
	c.Type = "feat"
	c.Scope = "scope"
	c.Subject = specSubject
	return *c
}

// TestUpstreamParseCommit ports the CommitParser.spec.ts tests. "Adapted"
// cases change the spec's custom keywords to the preset ones: "BREAKING
// AMEND" to "BREAKING CHANGE", kill and handle to close and fix, and
// "gh-" to "#". The merge commit cases have no mergePattern, so the merge
// line is the header. A name has the describe() title, if any, and the
// it() title.
func TestUpstreamParseCommit(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    Commit
	}{
		{
			name:    "CommitParser.spec.ts: should work",
			message: "feat(ng-list): Allow custom separator\nbla bla bla\n\nCloses #123\nCloses #25\nFixes #33\n",
			want: Commit{
				Header:  "feat(ng-list): Allow custom separator",
				Type:    "feat",
				Scope:   "ng-list",
				Subject: "Allow custom separator",
				Body:    "bla bla bla",
				Footer:  "Closes #123\nCloses #25\nFixes #33",
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: "#123"},
					{Action: "Closes", Prefix: "#", Issue: "25", Raw: "#25"},
					{Action: "Fixes", Prefix: "#", Issue: "33", Raw: "#33"},
				},
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse references from header",
			message: "Subject #1",
			want: Commit{
				Header: "Subject #1",
				References: []Reference{
					{Prefix: "#", Issue: "1", Raw: "Subject #1"},
				},
			},
		},
		{
			name:    "CommitParser.spec.ts: should deduplicate references when the same issue appears multiple times",
			message: "feat(ng-list): Allow custom separator\nCloses #123\nCloses #123\nFixes #123\n",
			want: Commit{
				Header:  "feat(ng-list): Allow custom separator",
				Type:    "feat",
				Scope:   "ng-list",
				Subject: "Allow custom separator",
				Footer:  "Closes #123\nCloses #123\nFixes #123",
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: "#123"},
					{Action: "Fixes", Prefix: "#", Issue: "123", Raw: "#123"},
				},
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse slash in the header with default headerPattern option",
			message: "feat(hello/world): message",
			want: Commit{
				Header:  "feat(hello/world): message",
				Type:    "feat",
				Scope:   "hello/world",
				Subject: "message",
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse @ in the header with default headerPattern option",
			message: "feat(@org/hello-world): message",
			want: Commit{
				Header:  "feat(@org/hello-world): message",
				Type:    "feat",
				Scope:   "@org/hello-world",
				Subject: "message",
			},
		},
		{
			name:    "CommitParser.spec.ts: should not be subject to ReDos",
			message: "b" + strings.Repeat("\r\n", 1000000) + "b",
			want: Commit{
				Header: "b",
				Body:   "b",
			},
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should trim extra newlines",
			message: "\n\n\n\n\n\n\n" +
				specHeader + "\n\n\n\n\n\n" +
				specBody1 + "\n\n\n\n" +
				specBody2 + "\n\n\n\n\n\n" +
				"BREAKING CHANGE: some breaking change\n\n\n\n\n" +
				"BREAKING CHANGE: An awesome breaking change\n\n\n" +
				"```\n" +
				"code here\n" +
				"```\n\n" +
				"Closes #1\n\n\n\n" +
				"closed #25\n\n\n\n\n",
			want: specCommit(&Commit{
				Body: specBody1 + "\n\n\n\n" + specBody2,
				Footer: "BREAKING CHANGE: some breaking change\n\n\n\n\n" +
					"BREAKING CHANGE: An awesome breaking change\n\n\n" +
					"```\n" +
					"code here\n" +
					"```\n\n" +
					"Closes #1\n\n\n\n" +
					"closed #25",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change"},
					{Title: "BREAKING CHANGE", Text: "An awesome breaking change\n\n\n```\ncode here\n```"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "closed", Prefix: "#", Issue: "25", Raw: "#25"},
				},
			}),
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should keep spaces",
			message: " " + specHeader + " \n" +
				" " + specBody1 + " \n\n" +
				" " + specBody2 + " \n" +
				"         BREAKING CHANGE: some breaking change         \n\n" +
				"   BREAKING CHANGE: An awesome breaking change\n\n\n" +
				"```\n" +
				"code here\n" +
				"```\n\n" +
				"    Closes   #1\n",
			want: Commit{
				Header: " " + specHeader + " ",
				Body:   " " + specBody1 + " \n\n" + " " + specBody2 + " ",
				Footer: "         BREAKING CHANGE: some breaking change         \n\n" +
					"   BREAKING CHANGE: An awesome breaking change\n\n\n" +
					"```\n" +
					"code here\n" +
					"```\n\n" +
					"    Closes   #1",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change         "},
					{Title: "BREAKING CHANGE", Text: "An awesome breaking change\n\n\n```\ncode here\n```"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
				},
			},
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should ignore gpg signature lines",
			message: "gpg: Signature made Thu Oct 22 12:19:30 2020 EDT\n" +
				"gpg:                using RSA key ABCDEF1234567890\n" +
				"gpg: Good signature from \"Author <author@example.com>\" [ultimate]\n" +
				specHeader + "\n" +
				specBody + "\n" +
				"BREAKING CHANGE: some breaking change\n" +
				"Closes #1\n",
			want: specCommit(&Commit{
				Body:   specBody,
				Footer: "BREAKING CHANGE: some breaking change\nCloses #1",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
				},
			}),
		},
		{
			name: "CommitParser.spec.ts: should ignore scissor line when no comment char is given",
			message: "this is some header before a scissors-line\n" +
				"# ------------------------ >8 ------------------------\n" +
				"this is a line that should not be truncated\n",
			want: Commit{
				Header: "this is some header before a scissors-line",
				Body: "# ------------------------ >8 ------------------------\n" +
					"this is a line that should not be truncated",
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse merge as null if options.mergePattern is not defined",
			message: "Merge branch 'feature'\nHEADER",
			want: Commit{
				Header: "Merge branch 'feature'",
				Body:   "HEADER",
			},
		},
		{
			name: "CommitParser.spec.ts: should not parse conventional header if pull request header present " +
				"and mergePattern is not set",
			message: "Merge pull request #1 from user/feature/feature-name\n" + specHeader,
			want: Commit{
				Header: "Merge pull request #1 from user/feature/feature-name",
				Body:   specHeader,
				References: []Reference{
					{Prefix: "#", Issue: "1", Raw: "Merge pull request #1"},
				},
			},
		},
		{
			// Also it('should understand header parts in GitHub like pull request') and
			// it('should understand merge parts in GitHub like pull request').
			name: "CommitParser.spec.ts: github style: should parse header in GitHub like pull request",
			message: "Merge pull request #1 from user/feature/feature-name\n\n" +
				specHeader + "\n\n" +
				specBody,
			want: Commit{
				Header: "Merge pull request #1 from user/feature/feature-name",
				Body:   specHeader + "\n\n" + specBody,
				References: []Reference{
					{Prefix: "#", Issue: "1", Raw: "Merge pull request #1"},
				},
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse header if merge header is missing",
			message: specHeader,
			want:    specCommit(&Commit{}),
		},
		{
			// Also it('should understand header parts in GitLab like merge request') and
			// it('should understand merge parts in GitLab like merge request').
			name: "CommitParser.spec.ts: gitlab style: should parse header in GitLab like merge request",
			message: "Merge branch 'feature/feature-name' into 'master'\r\n" +
				"\r\n" +
				specHeader + "\r\n" +
				"\r\n" +
				specBody1 + "\r\n" +
				specBody2 + "\r\n" +
				"\r\n" +
				"See merge request !1",
			want: Commit{
				Header: "Merge branch 'feature/feature-name' into 'master'",
				Body:   specHeader + "\n\n" + specBody + "\n\nSee merge request !1",
			},
		},
		{
			// Also it('should understand header parts in Azure DevOps like merge request') and
			// it('should understand merge parts in Azure DevOps like merge request').
			name: "CommitParser.spec.ts: azure devops style: should parse header in Azure DevOps like merge request",
			message: "Merged PR 1: " + specHeader + "\r\n" +
				"\r\n" +
				specBody1 + "\r\n" +
				specBody2 + "\r\n" +
				"\r\n" +
				"See merge request !1",
			want: Commit{
				Header: "Merged PR 1: " + specHeader,
				Body:   specBody + "\n\nSee merge request !1",
			},
		},
		{
			name:    "CommitParser.spec.ts: should allow \":\" in scope",
			message: "feat(ng:list): Allow custom separator",
			want: Commit{
				Header:  "feat(ng:list): Allow custom separator",
				Type:    "feat",
				Scope:   "ng:list",
				Subject: "Allow custom separator",
			},
		},
		{
			// Also describe('body'): it('should be null if not found') and
			// describe('footer'): it('should be null if not found').
			name:    "CommitParser.spec.ts: should parse header part as null if not captured",
			message: "header",
			want: Commit{
				Header: "header",
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse header if the subject match breakingHeaderPattern",
			message: "feat(hello/world)!: message",
			want: Commit{
				Header:  "feat(hello/world)!: message",
				Type:    "feat",
				Scope:   "hello/world",
				Subject: "message",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "message"},
				},
			},
		},
		{
			// Also it('should understand header parts'), it('should parse body'),
			// it('should parse footer'), it('should parse important notes') and
			// it('should parse references'). Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should parse header",
			message: specHeader + "\n" +
				specBody + "\n" +
				"BREAKING CHANGE: some breaking change\n" +
				"Closes #1, #123\n" +
				"closed #25\n" +
				"fix #33, Closes #100, Fixed #3 closes repo#77\n" +
				"closes stevemao/conventional-commits-parser#1",
			want: specCommit(&Commit{
				Body: specBody,
				Footer: "BREAKING CHANGE: some breaking change\n" +
					"Closes #1, #123\n" +
					"closed #25\n" +
					"fix #33, Closes #100, Fixed #3 closes repo#77\n" +
					"closes stevemao/conventional-commits-parser#1",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
					{Action: "closed", Prefix: "#", Issue: "25", Raw: "#25"},
					{Action: "fix", Prefix: "#", Issue: "33", Raw: "#33"},
					{Action: "Closes", Prefix: "#", Issue: "100", Raw: "#100"},
					{Action: "Fixed", Prefix: "#", Issue: "3", Raw: "#3"},
					{Action: "closes", Repository: "repo", Prefix: "#", Issue: "77", Raw: "repo#77"},
					{
						Action: "closes", Owner: "stevemao", Repository: "conventional-commits-parser", Prefix: "#", Issue: "1",
						Raw: "stevemao/conventional-commits-parser#1",
					},
				},
			}),
		},
		{
			// Adapted to the preset keywords.
			name:    "CommitParser.spec.ts: should reference an issue with an owner",
			message: "fixed angular/angular.js#1",
			want: Commit{
				Header: "fixed angular/angular.js#1",
				References: []Reference{
					{
						Action: "fixed", Owner: "angular", Repository: "angular.js", Prefix: "#", Issue: "1",
						Raw: "angular/angular.js#1",
					},
				},
			},
		},
		{
			// Adapted to the preset keywords.
			name:    "CommitParser.spec.ts: should reference an issue with a repository",
			message: "fixed angular.js#1",
			want: Commit{
				Header: "fixed angular.js#1",
				References: []Reference{
					{Action: "fixed", Repository: "angular.js", Prefix: "#", Issue: "1", Raw: "angular.js#1"},
				},
			},
		},
		{
			// Adapted to the preset keywords.
			name:    "CommitParser.spec.ts: should reference an issue without both",
			message: "fixed #1",
			want: Commit{
				Header: "fixed #1",
				References: []Reference{
					{Action: "fixed", Prefix: "#", Issue: "1", Raw: "#1"},
				},
			},
		},
		{
			// Adapted to the preset keywords.
			name:    "CommitParser.spec.ts: header: should reference an issue without an action",
			message: "This is #1",
			want: Commit{
				Header: "This is #1",
				References: []Reference{
					{Prefix: "#", Issue: "1", Raw: "This is #1"},
				},
			},
		},
		{
			// Also it('should parse references as empty if not found').
			name:    "CommitParser.spec.ts: should return empty notes",
			message: "chore: some chore\n",
			want: Commit{
				Header:  "chore: some chore",
				Type:    "chore",
				Subject: "some chore",
			},
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should parse important notes with more than one paragraphs",
			message: specHeader + "\n" +
				specBody + "\n" +
				"BREAKING CHANGE:\n" +
				"some breaking change\n" +
				"some other breaking change\n" +
				"Closes #1, #123\n" +
				"closed #25\n" +
				"fix #33, Closes #100, Fixed #3",
			want: specCommit(&Commit{
				Body: specBody,
				Footer: "BREAKING CHANGE:\n" +
					"some breaking change\n" +
					"some other breaking change\n" +
					"Closes #1, #123\n" +
					"closed #25\n" +
					"fix #33, Closes #100, Fixed #3",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change\nsome other breaking change"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
					{Action: "closed", Prefix: "#", Issue: "25", Raw: "#25"},
					{Action: "fix", Prefix: "#", Issue: "33", Raw: "#33"},
					{Action: "Closes", Prefix: "#", Issue: "100", Raw: "#100"},
					{Action: "Fixed", Prefix: "#", Issue: "3", Raw: "#3"},
				},
			}),
		},
		{
			name: "CommitParser.spec.ts: should parse important notes that start with asterisks (for squash commits)",
			message: "fix(core): report duplicate template bindings in templates\n\n" +
				"Fixes #7315\n\n" +
				"* BREAKING CHANGE:\n\n" +
				"Previously multiple template bindings on one element\n" +
				"(ex. `<div *ngIf='..' *ngFor='...'>`) were allowed but most of the time\n" +
				"were leading to undesired result. It is possible that a small number\n" +
				"of applications will see template parse errors that shuld be fixed by\n" +
				"nesting elements or using `<template>` tags explicitly.\n" +
				"Closes #9462",
			want: Commit{
				Header:  "fix(core): report duplicate template bindings in templates",
				Type:    "fix",
				Scope:   "core",
				Subject: "report duplicate template bindings in templates",
				Footer: "Fixes #7315\n\n" +
					"* BREAKING CHANGE:\n\n" +
					"Previously multiple template bindings on one element\n" +
					"(ex. `<div *ngIf='..' *ngFor='...'>`) were allowed but most of the time\n" +
					"were leading to undesired result. It is possible that a small number\n" +
					"of applications will see template parse errors that shuld be fixed by\n" +
					"nesting elements or using `<template>` tags explicitly.\n" +
					"Closes #9462",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "Previously multiple template bindings on one element\n" +
						"(ex. `<div *ngIf='..' *ngFor='...'>`) were allowed but most of the time\n" +
						"were leading to undesired result. It is possible that a small number\n" +
						"of applications will see template parse errors that shuld be fixed by\n" +
						"nesting elements or using `<template>` tags explicitly."},
				},
				References: []Reference{
					{Action: "Fixes", Prefix: "#", Issue: "7315", Raw: "#7315"},
					{Action: "Closes", Prefix: "#", Issue: "9462", Raw: "#9462"},
				},
			},
		},
		{
			name: "CommitParser.spec.ts: should not treat it as important notes if there are texts after `noteKeywords`",
			message: "fix(core): report duplicate template bindings in templates\n\n" +
				"Fixes #7315\n\n" +
				"BREAKING CHANGES:\n\n" +
				"Previously multiple template bindings on one element\n" +
				"(ex. `<div *ngIf='..' *ngFor='...'>`) were allowed but most of the time\n" +
				"were leading to undesired result. It is possible that a small number\n" +
				"of applications will see template parse errors that shuld be fixed by\n" +
				"nesting elements or using `<template>` tags explicitly.\n\n" +
				"Closes #9462",
			want: Commit{
				Header:  "fix(core): report duplicate template bindings in templates",
				Type:    "fix",
				Scope:   "core",
				Subject: "report duplicate template bindings in templates",
				Footer: "Fixes #7315\n\n" +
					"BREAKING CHANGES:\n\n" +
					"Previously multiple template bindings on one element\n" +
					"(ex. `<div *ngIf='..' *ngFor='...'>`) were allowed but most of the time\n" +
					"were leading to undesired result. It is possible that a small number\n" +
					"of applications will see template parse errors that shuld be fixed by\n" +
					"nesting elements or using `<template>` tags explicitly.\n\n" +
					"Closes #9462",
				References: []Reference{
					{Action: "Fixes", Prefix: "#", Issue: "7315", Raw: "#7315"},
					{Action: "Closes", Prefix: "#", Issue: "9462", Raw: "#9462"},
				},
			},
		},
		{
			name: "CommitParser.spec.ts: should ignore URL references",
			message: specHeader + "\n" +
				"relates to #1 \n" +
				"according to https://github.com/org/repo#my-readme-section, \n" +
				specBody1 + "\n" +
				"see https://github.com/org/repo/issues/123#issuecomment-44453102 for more details.",
			want: specCommit(&Commit{
				Footer: "relates to #1 \n" +
					"according to https://github.com/org/repo#my-readme-section, \n" +
					specBody1 + "\n" +
					"see https://github.com/org/repo/issues/123#issuecomment-44453102 for more details.",
				References: []Reference{
					{Prefix: "#", Issue: "1", Raw: "relates to #1"},
				},
			}),
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: footer: should reference an issue without an action",
			message: specHeader + "\n" +
				specBody + "\n" +
				"Closes #1, #123\n" +
				"what\n" +
				"* #25\n" +
				"* #33, maybe #100, not sure about #3\n",
			want: specCommit(&Commit{
				Body:   specBody,
				Footer: "Closes #1, #123\nwhat\n* #25\n* #33, maybe #100, not sure about #3",
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
					{Prefix: "#", Issue: "25", Raw: "* #25"},
					{Prefix: "#", Issue: "33", Raw: "* #33"},
					{Prefix: "#", Issue: "100", Raw: ", maybe #100"},
					{Prefix: "#", Issue: "3", Raw: ", not sure about #3"},
				},
			}),
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should put everything after references in footer",
			message: specHeader + "\n" +
				specBody + "\n" +
				"Closes #1, #123\n" +
				"what\n" +
				"closed #25\n" +
				"fix #33, Closes #100, Fixed #3\n" +
				"other",
			want: specCommit(&Commit{
				Body:   specBody,
				Footer: "Closes #1, #123\nwhat\nclosed #25\nfix #33, Closes #100, Fixed #3\nother",
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
					{Action: "closed", Prefix: "#", Issue: "25", Raw: "#25"},
					{Action: "fix", Prefix: "#", Issue: "33", Raw: "#33"},
					{Action: "Closes", Prefix: "#", Issue: "100", Raw: "#100"},
					{Action: "Fixed", Prefix: "#", Issue: "3", Raw: "#3"},
				},
			}),
		},
		{
			// The first of two tests with this title. Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should parse properly if important notes comes after references",
			message: specHeader + "\n" +
				specBody + "\n" +
				"Closes #1, #123\n" +
				"BREAKING CHANGE: some breaking change\n",
			want: specCommit(&Commit{
				Body:   specBody,
				Footer: "Closes #1, #123\nBREAKING CHANGE: some breaking change",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
				},
			}),
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should parse properly if important notes comes with more than one paragraphs " +
				"after references",
			message: specHeader + "\n" +
				specBody + "\n" +
				"Closes #1, #123\n" +
				"BREAKING CHANGE: some breaking change\n" +
				"some other breaking change",
			want: specCommit(&Commit{
				Body:   specBody,
				Footer: "Closes #1, #123\nBREAKING CHANGE: some breaking change\nsome other breaking change",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change\nsome other breaking change"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
				},
			}),
		},
		{
			// The second of two tests with this title. Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should parse properly if important notes comes after references 2",
			message: specHeader + "\n" +
				specBody + "\n" +
				"Closes #1, #123\n" +
				"other\n" +
				"BREAKING CHANGE: some breaking change\n",
			want: specCommit(&Commit{
				Body:   specBody,
				Footer: "Closes #1, #123\nother\nBREAKING CHANGE: some breaking change",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change"},
				},
				References: []Reference{
					{Action: "Closes", Prefix: "#", Issue: "1", Raw: "#1"},
					{Action: "Closes", Prefix: "#", Issue: "123", Raw: ", #123"},
				},
			}),
		},
		{
			name:    "CommitParser.spec.ts: should add the subject as note if it match breakingHeaderPattern",
			message: "feat!: breaking change feature",
			want: Commit{
				Header:  "feat!: breaking change feature",
				Type:    "feat",
				Subject: "breaking change feature",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "breaking change feature"},
				},
			},
		},
		{
			// Adapted to the preset keywords.
			name:    "CommitParser.spec.ts: should not duplicate notes if the subject match breakingHeaderPattern",
			message: "feat!: breaking change feature\nBREAKING CHANGE: some breaking change",
			want: Commit{
				Header:  "feat!: breaking change feature",
				Type:    "feat",
				Subject: "breaking change feature",
				Footer:  "BREAKING CHANGE: some breaking change",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "some breaking change"},
				},
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse hash",
			message: "My commit message\n-hash-\n9b1aff905b638aa274a5fc8f88662df446d374bd",
			want: Commit{
				Hash:   "9b1aff905b638aa274a5fc8f88662df446d374bd",
				Header: "My commit message",
			},
		},
		{
			// Adapted to the preset keywords.
			name: "CommitParser.spec.ts: should parse meta after notes",
			message: "build!: first build setup\n\n" +
				"BREAKING CHANGE: New build system.\n\n" +
				"-hash-\n" +
				"4937825901dacca88609da354f0e8a8c84ae04ea\n" +
				"-gitTags-\n\n" +
				"-committerDate-\n" +
				"2023-09-16 21:13:23 +0400\n",
			want: Commit{
				Hash:    "4937825901dacca88609da354f0e8a8c84ae04ea",
				Header:  "build!: first build setup",
				Type:    "build",
				Subject: "first build setup",
				Footer:  "BREAKING CHANGE: New build system.",
				Notes: []Note{
					{Title: "BREAKING CHANGE", Text: "New build system."},
				},
			},
		},
		{
			name: "CommitParser.spec.ts: should parse sideNotes",
			message: "My commit message\n" +
				"-sideNotes-\n" +
				"It should warn the correct unfound file names.\n" +
				"Also it should continue if one file cannot be found.\n" +
				"Tests are added for these",
			want: Commit{
				Header: "My commit message",
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse committer name and email",
			message: "My commit message\n-committerName-\nSteve Mao\n- committerEmail-\ntest@github.com",
			want: Commit{
				Header: "My commit message",
			},
		},
		{
			name: "CommitParser.spec.ts: should parse revert",
			message: "Revert \"throw an error if a callback is passed to animate methods\"\n\n" +
				"This reverts commit 9bb4d6ccbe80b7704c6b7f53317ca8146bc103ca.",
			want: Commit{
				Header: "Revert \"throw an error if a callback is passed to animate methods\"",
				Body:   "This reverts commit 9bb4d6ccbe80b7704c6b7f53317ca8146bc103ca.",
				Revert: &Revert{
					Header: "throw an error if a callback is passed to animate methods",
					Hash:   "9bb4d6ccbe80b7704c6b7f53317ca8146bc103ca",
				},
			},
		},
		{
			name:    "CommitParser.spec.ts: should parse revert even if a field is missing",
			message: "Revert \"\"\n\nThis reverts commit .",
			want: Commit{
				Header: "Revert \"\"",
				Body:   "This reverts commit .",
				Revert: &Revert{Header: "\"", Hash: ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.Message = tt.message
			assert.DeepEqual(t, ParseCommit(RawCommit{Message: tt.message}), tt.want)
		})
	}
}

// TestUpstreamNoteRegex ports the regex.spec.ts notes tests that apply to
// the preset note keywords.
func TestUpstreamNoteRegex(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			// Adapted to the preset keywords.
			name: "regex.spec.ts: should match a simple note",
			line: "BREAKING CHANGE: This is so important.",
			want: []string{"BREAKING CHANGE: This is so important.", "BREAKING CHANGE", "This is so important."},
		},
		{
			// Adapted to the preset keywords.
			name: "regex.spec.ts: notes: should be case insensitive",
			line: "breaking change: This is so important.",
			want: []string{"breaking change: This is so important.", "breaking change", "This is so important."},
		},
		{
			// Adapted to the preset keywords.
			name: "regex.spec.ts: should not accidentally match in a sentence",
			line: "This is a breaking change: So important.",
		},
		{
			name: "regex.spec.ts: should not match if there is text after `noteKeywords`",
			line: "BREAKING CHANGES: Wow.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, noteRegex.FindStringSubmatch(tt.line), tt.want)
		})
	}
}

// TestUpstreamReferenceSentences ports the regex.spec.ts references tests.
// Each sentence is the action and text groups of the parser's references
// regex, or of its /()(.+)/ fallback, as the Node.js parser returns them
// with the preset reference actions.
func TestUpstreamReferenceSentences(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []sentence
	}{
		{
			name: "regex.spec.ts: should match a simple reference",
			line: "closes #1",
			want: []sentence{{action: "closes", text: "#1"}},
		},
		{
			name: "regex.spec.ts: references: should be case insensitive",
			line: "ClOsEs #1",
			want: []sentence{{action: "ClOsEs", text: "#1"}},
		},
		{
			// The preset has "closes", which matches after "close" fails.
			name: "regex.spec.ts: should not match if keywords does not present",
			line: "Closes #1",
			want: []sentence{{action: "Closes", text: "#1"}},
		},
		{
			// The preset has no "amends", so the line is one sentence with no
			// action.
			name: "regex.spec.ts: should take multiple reference keywords",
			line: "amends #1",
			want: []sentence{{text: "amends #1"}},
		},
		{
			name: "regex.spec.ts: should match multiple references",
			line: "Closes #1 amends #2; closes bug #4",
			want: []sentence{{action: "Closes", text: "#1 amends #2; "}, {action: "closes", text: "bug #4"}},
		},
		{
			name: "regex.spec.ts: should match references with mixed content, like JIRA tickets",
			line: "Closes #JIRA-123 amends #MY-OTHER-PROJECT-123; closes bug #4",
			want: []sentence{
				{action: "Closes", text: "#JIRA-123 amends #MY-OTHER-PROJECT-123; "},
				{action: "closes", text: "bug #4"},
			},
		},
		{
			// The preset has "Closes", so the regex finds one sentence.
			name: "regex.spec.ts: references: should reference an issue without an action",
			line: "gh-1, prefix-3, Closes gh-6",
			want: []sentence{{action: "Closes", text: "gh-6"}},
		},
		{
			name: "regex.spec.ts: references: should ignore whitespace",
			line: "closes #1, amends #2, fixes #3",
			want: []sentence{{action: "closes", text: "#1, amends #2, "}, {action: "fixes", text: "#3"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, sentencePairs(referenceSentences(tt.line)), sentencePairs(tt.want))
		})
	}
}

// sentencePairs returns the action and text of each sentence, because
// assert.DeepEqual cannot compare the unexported sentence fields.
func sentencePairs(ss []sentence) [][2]string {
	pairs := make([][2]string, 0, len(ss))
	for _, s := range ss {
		pairs = append(pairs, [2]string{s.action, s.text})
	}
	return pairs
}

// TestUpstreamReferenceParts ports the regex.spec.ts referenceParts tests.
// The preset issue prefix is "#", as in those tests.
func TestUpstreamReferenceParts(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []Reference
	}{
		{
			name: "regex.spec.ts: should match simple reference parts",
			text: "#1",
			want: []Reference{{Prefix: "#", Issue: "1", Raw: "#1"}},
		},
		{
			name: "regex.spec.ts: should reference an issue in parenthesis",
			text: "#27), pinned shelljs to version that works with nyc (#30)",
			want: []Reference{
				{Prefix: "#", Issue: "27", Raw: "#27"},
				{Prefix: "#", Issue: "30", Raw: "), pinned shelljs to version that works with nyc (#30"},
			},
		},
		{
			name: "regex.spec.ts: should match reference parts with something else",
			text: "something else #1",
			want: []Reference{{Prefix: "#", Issue: "1", Raw: "something else #1"}},
		},
		{
			name: "regex.spec.ts: should match reference parts with a repository",
			text: "repo#1",
			want: []Reference{{Repository: "repo", Prefix: "#", Issue: "1", Raw: "repo#1"}},
		},
		{
			name: "regex.spec.ts: should match JIRA-123 like reference parts",
			text: "#JIRA-123",
			want: []Reference{{Prefix: "#", Issue: "JIRA-123", Raw: "#JIRA-123"}},
		},
		{
			name: "regex.spec.ts: should match ClickUp-style reference parts ending with letters",
			text: "#CU-123abc",
			want: []Reference{{Prefix: "#", Issue: "CU-123abc", Raw: "#CU-123abc"}},
		},
		{
			name: "regex.spec.ts: should not match MY-€#%#&-123 mixed symbol reference parts",
			text: "#MY-€#%#&-123",
		},
		{
			name: "regex.spec.ts: should match reference parts with multiple references",
			text: "#1 #2, something #3; repo#4",
			want: []Reference{
				{Prefix: "#", Issue: "1", Raw: "#1"},
				{Prefix: "#", Issue: "2", Raw: " #2"},
				{Prefix: "#", Issue: "3", Raw: ", something #3"},
				{Repository: "repo", Prefix: "#", Issue: "4", Raw: "; repo#4"},
			},
		},
		{
			// Only "#" is a prefix here, so the one match ends at "#2".
			name: "regex.spec.ts: should match issues with customized prefix",
			text: "closes gh-1, amends #2, fixes prefix-3",
			want: []Reference{{Prefix: "#", Issue: "2", Raw: "closes gh-1, amends #2"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, referencesIn(tt.text), tt.want)
		})
	}
}

// TestUpstreamURLRegex ports the regex.spec.ts url test.
func TestUpstreamURLRegex(t *testing.T) {
	// regex.spec.ts it('should match any URLs')
	for _, u := range []string{
		"https://github.com/org/repo/issues/123#issuecomment-444531023",
		"https://github.com/org/repo#my-readme-section",
		"https://example.com/",
		"https://example.com/1#1",
	} {
		assert.Assert(t, urlRegex.MatchString(u), u)
	}
}

// TestUpstreamFilterReverted ports the filters.spec.ts tests. Commit has
// no raw field, so a commit with raw data uses its raw header and hash,
// which are the values that findRevertCommit compares.
func TestUpstreamFilterReverted(t *testing.T) {
	tests := []struct {
		name    string
		commits []Commit
		want    []string // hashes of the commits that stay
	}{
		{
			name: "filters.spec.ts: should filter reverted commits",
			commits: []Commit{
				{Header: "commit to revert 1", Hash: "r_1", Revert: &Revert{Header: "commit to revert 1", Hash: "1"}},
				{Header: "commit 3", Hash: "3"},
				{Header: "commit to revert 2", Hash: "r_2", Revert: &Revert{Header: "commit to revert 2", Hash: "2"}},
				{Header: "commit 4", Hash: "4"},
				{Header: "commit to revert 1", Hash: "1"},
				{Header: "commit 5", Hash: "5"},
				{Header: "commit to revert 2", Hash: "2"},
				{Header: "commit 6", Hash: "6"},
			},
			want: []string{"3", "4", "5", "6"},
		},
		{
			name: "filters.spec.ts: should filter reverted commits that stands after revert commit",
			commits: []Commit{
				{
					Header: "revert: feat(): a very important feature\n",
					Hash:   "207abfa16885ef5ff88dfc6cdde694bb3fd03104\n",
					Revert: &Revert{Header: "feat(): amazing new module", Hash: "048fe156c9eddbe566f040f64ca6be1f55b16a23"},
				},
				{
					Header: "revert: feat(): amazing new module\n",
					Hash:   "789d898b5f8422d7f65cc25135af2c1a95a125ac\n",
					Revert: &Revert{Header: "feat(): amazing new module", Hash: "56185b7356766d2b30cfa2406b257080272e0b7a"},
				},
				{Header: "feat(): amazing new module\n", Hash: "56185b7356766d2b30cfa2406b257080272e0b7a\n"},
				{Header: "feat(): new feature\n", Hash: "815a3f0717bf1dfce007bd076420c609504edcf3\n"},
				{Header: "chore: first commit\n", Hash: "74a3e4d6d25dee2c0d6483a0a3887417728cbe0a\n"},
			},
			want: []string{
				"207abfa16885ef5ff88dfc6cdde694bb3fd03104\n",
				"815a3f0717bf1dfce007bd076420c609504edcf3\n",
				"74a3e4d6d25dee2c0d6483a0a3887417728cbe0a\n",
			},
		},
		{
			name: "filters.spec.ts: should filter revert of revert",
			commits: []Commit{
				{
					Header: "revert: feat(): a very important feature\n",
					Hash:   "207abfa16885ef5ff88dfc6cdde694bb3fd03104\n",
					Revert: &Revert{Header: "feat(): amazing new module", Hash: "048fe156c9eddbe566f040f64ca6be1f55b16a23"},
				},
				{
					Header: "revert: revert: feat(): amazing new module\n",
					Hash:   "098\n",
					Revert: &Revert{Header: "revert: feat(): amazing new module", Hash: "789d898b5f8422d7f65cc25135af2c1a95a125ac"},
				},
				{
					Header: "revert: feat(): amazing new module\n",
					Hash:   "789d898b5f8422d7f65cc25135af2c1a95a125ac\n",
					Revert: &Revert{Header: "feat(): amazing new module", Hash: "56185b7356766d2b30cfa2406b257080272e0b7a"},
				},
				{Header: "amazing new module\n", Hash: "56185b"},
				{Header: "feat(): new feature\n", Hash: "815a3f0"},
				{Header: "chore: first commit\n", Hash: "74a3e4d6d25"},
			},
			want: []string{"207abfa16885ef5ff88dfc6cdde694bb3fd03104\n", "56185b", "815a3f0", "74a3e4d6d25"},
		},
		{
			name: "filters.spec.ts: should fall back on commit if raw is undefined",
			commits: []Commit{
				{
					Header: "revert: feat(): amazing new module\n",
					Hash:   "789d898b5f8422d7f65cc25135af2c1a95a125ac\n",
					Revert: &Revert{Header: "feat(): amazing new module", Hash: "56185b7356766d2b30cfa2406b257080272e0b7a"},
				},
				{Header: "feat(): amazing new module\n", Hash: "56185b7356766d2b30cfa2406b257080272e0b7a\n"},
				{Header: "feat(): new feature\n", Hash: "815a3f0717bf1dfce007bd076420c609504edcf3\n"},
				{Header: "chore: first commit\n", Hash: "74a3e4d6d25dee2c0d6483a0a3887417728cbe0a\n"},
			},
			want: []string{"815a3f0717bf1dfce007bd076420c609504edcf3\n", "74a3e4d6d25dee2c0d6483a0a3887417728cbe0a\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range filterReverted(tt.commits) {
				got = append(got, c.Hash)
			}
			assert.DeepEqual(t, got, tt.want)
		})
	}
}
