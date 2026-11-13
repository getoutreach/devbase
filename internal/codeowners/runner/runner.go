// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Running the CODEOWNERS checks in parallel.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

// Package runner runs CODEOWNERS checks and prints their results.
package runner

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/getoutreach/devbase/v2/internal/codeowners/check"
)

// result is what one check returned.
type result struct {
	// out is the output of the check.
	out check.Output

	// err is the error of the check.
	err error

	// duration is how long the check ran.
	duration time.Duration
}

// Run runs the checks in parallel, then prints the result of each check in the order of checks,
// and a summary. The run failed when a check returned an error that does not wrap
// check.ErrSkipped, or when a check found an issue that is at least as serious as failureLevel.
func Run(ctx context.Context, p *Printer, in check.Input, failureLevel check.Severity, checks ...check.Checker) (failed bool) {
	results := make([]result, len(checks))

	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Go(func() {
			start := time.Now()
			out, err := c.Check(ctx, in)
			results[i] = result{out: out, err: err, duration: time.Since(start)}
		})
	}
	wg.Wait()

	notPassed := 0
	for i, r := range results {
		p.PrintCheckResult(checks[i].Name(), r.duration, r.out, r.err)

		checkFailed := r.err != nil && !errors.Is(r.err, check.ErrSkipped)
		if len(r.out.Issues) > 0 || checkFailed {
			notPassed++
		}

		failed = failed || checkFailed || hasIssueAtLeast(r.out, failureLevel)
	}

	p.PrintSummary(len(checks), notPassed)

	return failed
}

// hasIssueAtLeast reports whether out has an issue that is at least as serious as level.
func hasIssueAtLeast(out check.Output, level check.Severity) bool {
	return slices.ContainsFunc(out.Issues, func(i check.Issue) bool { return i.Severity.IsAtLeast(level) })
}
