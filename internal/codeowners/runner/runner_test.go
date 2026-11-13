// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for running CODEOWNERS checks.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package runner_test

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/getoutreach/devbase/v2/internal/codeowners/check"
	"github.com/getoutreach/devbase/v2/internal/codeowners/runner"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

// durationRE matches the duration that the printer writes in the header of a check.
var durationRE = regexp.MustCompile(`\(\d[^)]*\)`)

// The errors that the fake checks return.
var (
	errBroke    = errors.New("it broke")
	errBoom     = errors.New("boom")
	errInternal = errors.New("some check internal error")
)

// fakeCheck is a Checker that returns a fixed result.
type fakeCheck struct {
	// name is the name of the check.
	name string

	// out is the output of the check.
	out check.Output

	// err is the error of the check.
	err error

	// run is called when the check starts, when it is not nil.
	run func()
}

// Name implements check.Checker.
func (f *fakeCheck) Name() string { return f.name }

// Check implements check.Checker.
func (f *fakeCheck) Check(context.Context, check.Input) (check.Output, error) {
	if f.run != nil {
		f.run()
	}

	return f.out, f.err
}

// runChecks runs the checks and returns the result and the normalized output.
func runChecks(t *testing.T, level check.Severity, checks ...check.Checker) (failed bool, output string) {
	t.Helper()

	var buf bytes.Buffer
	failed = runner.Run(t.Context(), runner.NewPrinter(&buf, true), check.Input{}, level, checks...)

	return failed, durationRE.ReplaceAllString(buf.String(), "(<duration>)")
}

// TestRunPrintsInCheckOrder checks that the output follows the order of the checks, even when a
// later check finishes first.
func TestRunPrintsInCheckOrder(t *testing.T) {
	secondDone := make(chan struct{})

	slow := &fakeCheck{name: "Slow", run: func() {
		select {
		case <-secondDone:
		case <-time.After(10 * time.Second):
		}
	}}
	fast := &fakeCheck{
		name: "Fast",
		out:  check.Output{Issues: []check.Issue{{Severity: check.Warning, LineNo: 3, Message: "a warning"}}},
		run:  func() { close(secondDone) },
	}
	last := &fakeCheck{name: "Last", err: errBroke}

	failed, output := runChecks(t, check.Warning, slow, fast, last)

	assert.Assert(t, failed)
	golden.Assert(t, output, "run_order.golden")
}

// TestRunRunsChecksInParallel checks that a check does not wait for the checks before it.
func TestRunRunsChecksInParallel(t *testing.T) {
	var started sync.WaitGroup
	started.Add(2)

	barrier := func() {
		started.Done()

		done := make(chan struct{})
		go func() {
			started.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the checks did not run in parallel")
		}
	}

	failed, _ := runChecks(t, check.Warning, &fakeCheck{name: "A", run: barrier}, &fakeCheck{name: "B", run: barrier})

	assert.Assert(t, !failed)
}

// TestRunFailed checks which results make the run fail.
func TestRunFailed(t *testing.T) {
	warning := check.Output{Issues: []check.Issue{{Severity: check.Warning, Message: "w"}}}
	errorIssue := check.Output{Issues: []check.Issue{{Severity: check.Error, Message: "e"}}}

	tests := []struct {
		name   string
		level  check.Severity
		check  *fakeCheck
		failed bool
	}{
		{name: "no issues", level: check.Warning, check: &fakeCheck{}, failed: false},
		{name: "warning at warning level", level: check.Warning, check: &fakeCheck{out: warning}, failed: true},
		{name: "warning at error level", level: check.Error, check: &fakeCheck{out: warning}, failed: false},
		{name: "error at warning level", level: check.Warning, check: &fakeCheck{out: errorIssue}, failed: true},
		{name: "error at error level", level: check.Error, check: &fakeCheck{out: errorIssue}, failed: true},
		{name: "check error at error level", level: check.Error, check: &fakeCheck{err: errBoom}, failed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check.name = "Check"

			failed, _ := runChecks(t, tt.level, tt.check)

			assert.Equal(t, failed, tt.failed)
		})
	}
}

// TestRunSummaryCountsChecksWithIssues checks that the summary counts the checks that have an
// issue or an error, even when the issues are below the failure level.
func TestRunSummaryCountsChecksWithIssues(t *testing.T) {
	failed, output := runChecks(t, check.Error,
		&fakeCheck{name: "OK"},
		&fakeCheck{name: "Warn", out: check.Output{Issues: []check.Issue{{Severity: check.Warning, Message: "w"}}}},
		&fakeCheck{name: "Broken", err: errBoom},
	)

	assert.Assert(t, failed)
	golden.Assert(t, output, "run_summary.golden")
}
