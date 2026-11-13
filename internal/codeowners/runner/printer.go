// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Printing the results of the CODEOWNERS checks.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package runner

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/getoutreach/devbase/v2/internal/codeowners/check"
)

// Printer writes the results of checks as text, with colors when it may use them.
type Printer struct {
	// w is where the text is written.
	w io.Writer

	// noColor disables the colors for the text of this printer only.
	noColor bool
}

// NewPrinter returns a Printer that writes to w. The printer writes color escape codes unless
// noColor is true. The caller decides noColor, for example from the terminal and NO_COLOR.
func NewPrinter(w io.Writer, noColor bool) *Printer {
	return &Printer{w: w, noColor: noColor}
}

// PrintCheckResult prints the name, duration, issues, and error of one check.
func (p *Printer) PrintCheckResult(name string, duration time.Duration, out check.Output, checkErr error) {
	header := p.style(color.Bold).FprintfFunc()
	issueBody := p.style(color.FgWhite).FprintfFunc()

	header(p.w, "==> Executing %s (%v)\n", name, duration)

	for _, issue := range out.Issues {
		p.severityStyle(issue.Severity).Fprintf(p.w, "    [%s]", strings.ToLower(issue.Severity.String()[:3]))
		if issue.LineNo != 0 {
			issueBody(p.w, " line %d:", issue.LineNo)
		}
		issueBody(p.w, " %s\n", issue.Message)
	}

	switch {
	case checkErr != nil:
		p.style(color.FgRed).Fprint(p.w, "    [Internal Error]")
		issueBody(p.w, " %s\n", checkErr)
	case len(out.Issues) == 0:
		p.style(color.FgGreen).Fprintln(p.w, "    Check OK")
	}
}

// PrintSummary prints how many checks ran and how many of them did not pass.
func (p *Printer) PrintSummary(allChecks, failedChecks int) {
	failures := "no"
	if failedChecks > 0 {
		failures = strconv.Itoa(failedChecks)
	}

	p.style().Fprintf(p.w, "\n%d check(s) executed, %s failure(s)\n", allChecks, failures)
}

// style returns a color with the attributes. Only the printer decides if the color is on, so the
// NO_COLOR environment variable and the global setting of the color package do not change it.
func (p *Printer) style(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	if p.noColor {
		c.DisableColor()
	} else {
		c.EnableColor()
	}

	return c
}

// severityStyle returns the color of the tag of an issue with the severity.
func (p *Printer) severityStyle(severity check.Severity) *color.Color {
	switch severity {
	case check.Warning:
		return p.style(color.FgYellow)
	case check.Error:
		return p.style(color.FgRed)
	default:
		return p.style()
	}
}
