// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Types shared by the CODEOWNERS checks.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

// Package check validates the entries of a CODEOWNERS file.
package check

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
)

// The errors that the checks wrap.
var (
	// ErrInvalidSeverity is wrapped by the error that ParseSeverity returns for an unknown severity.
	ErrInvalidSeverity = errors.New("not a valid severity")

	// ErrSkipped is wrapped by the error of a check that skipped all or part of its work for a
	// reason that does not make the run fail, such as a low GitHub rate limit. Wrap it as
	// fmt.Errorf("%w: <reason>", ErrSkipped), so that the runner can print the reason.
	ErrSkipped = errors.New("skipped")
)

// Severity is how serious an issue is. A smaller value is more serious.
type Severity int

// The severities of an issue.
const (
	// Error is a problem that makes the CODEOWNERS file not work as intended.
	Error Severity = iota + 1

	// Warning is a problem that might make the CODEOWNERS file not work as intended.
	Warning
)

// ParseSeverity converts a case-insensitive severity name to a Severity. The accepted names are
// "error", "err", "warning", and "warn".
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(s) {
	case "error", "err":
		return Error, nil
	case "warning", "warn":
		return Warning, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidSeverity, s)
	}
}

// String returns the name of the severity.
func (s Severity) String() string {
	switch s {
	case Error:
		return "Error"
	case Warning:
		return "Warning"
	default:
		return ""
	}
}

// IsAtLeast reports whether s is as serious as level, or more serious.
func (s Severity) IsAtLeast(level Severity) bool {
	return s <= level
}

// Issue is a problem that a check found.
type Issue struct {
	// Severity is how serious the issue is.
	Severity Severity

	// LineNo is the line of the CODEOWNERS file that has the issue, or 0 when the issue is not about one line.
	LineNo uint64

	// Message describes the issue.
	Message string
}

// Input is what a check validates.
type Input struct {
	// RepoDir is the root directory of the repository.
	RepoDir string

	// Entries are the entries of the CODEOWNERS file.
	Entries []codeowners.Entry

	// TrustRepoDir makes git treat RepoDir as a safe directory, even when another user owns it.
	TrustRepoDir bool

	// Staged limits the files of the repository to the files in the git index, so the files check
	// ignores untracked files. The notowned check always uses the git index.
	Staged bool
}

// Output is what a check found.
type Output struct {
	// Issues are the problems that the check found.
	Issues []Issue
}

// IssueOption changes an Issue that Output.Report creates.
type IssueOption func(*Issue)

// WithSeverity sets the severity of the issue. The default severity is Error.
func WithSeverity(s Severity) IssueOption {
	return func(i *Issue) {
		i.Severity = s
	}
}

// WithEntry sets the line of the issue to the line of the entry.
func WithEntry(e codeowners.Entry) IssueOption {
	return func(i *Issue) {
		i.LineNo = e.LineNo
	}
}

// Report adds an issue to the output.
func (o *Output) Report(msg string, opts ...IssueOption) {
	i := Issue{Severity: Error, Message: msg}
	for _, opt := range opts {
		opt(&i)
	}

	o.Issues = append(o.Issues, i)
}

// Checker validates the entries of a CODEOWNERS file.
type Checker interface {
	// Name returns the human-readable name of the check.
	Name() string

	// Check validates in. It returns the context's error when the context is done.
	Check(ctx context.Context, in Input) (Output, error)
}
