// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the check API types.

package check

import (
	"errors"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"gotest.tools/v3/assert"
)

func TestParseSeverity(t *testing.T) {
	tests := map[string]Severity{
		"error":   Error,
		"err":     Error,
		"ERROR":   Error,
		"Err":     Error,
		"warning": Warning,
		"warn":    Warning,
		"Warning": Warning,
		"WARN":    Warning,
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := ParseSeverity(in)
			assert.NilError(t, err)
			assert.Equal(t, got, want)
		})
	}

	_, err := ParseSeverity("fatal")
	assert.Assert(t, errors.Is(err, ErrInvalidSeverity), "got %v", err)
	assert.ErrorContains(t, err, `"fatal"`)
}

func TestSeverityString(t *testing.T) {
	assert.Equal(t, Error.String(), "Error")
	assert.Equal(t, Warning.String(), "Warning")
	assert.Equal(t, Severity(0).String(), "")
}

func TestSeverityIsAtLeast(t *testing.T) {
	assert.Assert(t, Error.IsAtLeast(Error))
	assert.Assert(t, Error.IsAtLeast(Warning))
	assert.Assert(t, Warning.IsAtLeast(Warning))
	assert.Assert(t, !Warning.IsAtLeast(Error))
}

func TestOutputReport(t *testing.T) {
	var out Output
	out.Report("plain")
	out.Report("with options",
		WithEntry(codeowners.Entry{LineNo: 7}), WithSeverity(Warning))

	assert.DeepEqual(t, out.Issues, []Issue{
		{Severity: Error, Message: "plain"},
		{Severity: Warning, LineNo: 7, Message: "with options"},
	})
}
