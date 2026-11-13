// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the CODEOWNERS check result printer.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package runner_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/getoutreach/devbase/v2/internal/codeowners/check"
	"github.com/getoutreach/devbase/v2/internal/codeowners/runner"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

// TestPrinterPrintCheckResult checks the text that PrintCheckResult writes.
func TestPrinterPrintCheckResult(t *testing.T) {
	t.Run("prints all reported issues", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintCheckResult("Foo Checker", time.Second, check.Output{
			Issues: []check.Issue{
				{Severity: check.Error, LineNo: 42, Message: "Simulate error in line 42"},
				{Severity: check.Warning, LineNo: 2020, Message: "Simulate warning in line 2020"},
				{Severity: check.Error, Message: "Error without line number"},
				{Severity: check.Warning, Message: "Warning without line number"},
			},
		}, errInternal)

		golden.Assert(t, buf.String(), "check_result_issues.golden")
	})

	t.Run("prints the OK status when there are no issues", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintCheckResult("Foo Checker", time.Second, check.Output{}, nil)

		golden.Assert(t, buf.String(), "check_result_ok.golden")
	})

	t.Run("does not print the OK status when there are issues and no error", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintCheckResult("Foo Checker", time.Second, check.Output{
			Issues: []check.Issue{{Severity: check.Warning, Message: "Just a warning"}},
		}, nil)

		golden.Assert(t, buf.String(), "check_result_warning_only.golden")
	})

	t.Run("prints the reason of a skipped check after its issues", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintCheckResult("Foo Checker", time.Second, check.Output{
			Issues: []check.Issue{{Severity: check.Error, LineNo: 3, Message: "Found before the skip"}},
		}, fmt.Errorf("%w: the rate limit is low", check.ErrSkipped))

		golden.Assert(t, buf.String(), "check_result_skipped.golden")
	})

	t.Run("prints an issue with an unknown severity", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintCheckResult("Foo Checker", time.Second, check.Output{
			Issues: []check.Issue{{Severity: check.Severity(0), Message: "No severity"}},
		}, nil)

		assert.Assert(t, strings.Contains(buf.String(), "    [???] No severity\n"), buf.String())
	})
}

// TestPrinterPrintSummary checks the text that PrintSummary writes.
func TestPrinterPrintSummary(t *testing.T) {
	t.Run("prints the number of failures", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintSummary(20, 10)

		golden.Assert(t, buf.String(), "summary_failures.golden")
	})

	t.Run("prints no when there are no failures", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintSummary(20, 0)

		golden.Assert(t, buf.String(), "summary_no_failures.golden")
	})
}

// TestPrinterColor checks that only the noColor argument decides if the printer writes color
// escape codes, whatever the environment is.
func TestPrinterColor(t *testing.T) {
	out := check.Output{Issues: []check.Issue{{Severity: check.Error, Message: "bad"}}}

	t.Run("writes escape codes when colors are on", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, false).PrintCheckResult("Foo Checker", time.Second, out, nil)

		assert.Assert(t, strings.Contains(buf.String(), "\x1b["), "no escape code in %q", buf.String())
	})

	t.Run("writes no escape codes when colors are off", func(t *testing.T) {
		var buf bytes.Buffer

		runner.NewPrinter(&buf, true).PrintCheckResult("Foo Checker", time.Second, out, nil)

		assert.Assert(t, !strings.Contains(buf.String(), "\x1b["), "escape code in %q", buf.String())
	})
}
