// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds CODEOWNERS patterns that shadow earlier patterns.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
)

// AvoidShadowing validates that no pattern of the CODEOWNERS file shadows a pattern that comes
// before it. The last matching pattern wins, so entries should go from least-specific to most-specific.
type AvoidShadowing struct{}

// NewAvoidShadowing returns a new AvoidShadowing instance.
func NewAvoidShadowing() *AvoidShadowing {
	return &AvoidShadowing{}
}

// Check reports the patterns that shadow earlier patterns.
func (*AvoidShadowing) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	var previousEntries []codeowners.Entry
	for _, entry := range in.Entries {
		re, err := wildCardToRegexp(endWithSlash(entry.Pattern))
		if err != nil {
			return Output{}, fmt.Errorf("compile pattern %s into a regexp: %w", entry.Pattern, err)
		}

		var shadowed []codeowners.Entry
		for _, previous := range previousEntries {
			if re.MatchString(endWithSlash(previous.Pattern)) {
				shadowed = append(shadowed, previous)
			}
		}

		if len(shadowed) > 0 {
			out.Report(fmt.Sprintf("Pattern %q shadows the following patterns:\n%s\n"+
				"Entries should go from least-specific to most-specific.", entry.Pattern, listShadowed(shadowed)),
				WithEntry(entry))
		}

		previousEntries = append(previousEntries, entry)
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*AvoidShadowing) Name() string {
	return "[Experimental] Avoid Shadowing Checker"
}

// listShadowed formats the entries as a bullet point list.
func listShadowed(es []codeowners.Entry) string {
	points := make([]string, len(es))
	for i, e := range es {
		points[i] = fmt.Sprintf("            * %d: %q", e.LineNo, e.Pattern)
	}

	return strings.Join(points, "\n")
}

// endWithSlash adds a trailing slash to a string if it doesn't already end with one.
// This is useful when matching CODEOWNERS pattern because the trailing slash is optional.
func endWithSlash(s string) string {
	if !strings.HasSuffix(s, "/") {
		return s + "/"
	}

	return s
}

// wildCardToRegexp converts a wildcard pattern to a regular expression pattern.
func wildCardToRegexp(pattern string) (*regexp.Regexp, error) {
	var result strings.Builder
	for i, literal := range strings.Split(pattern, "*") {
		// Replace * with .*
		if i > 0 {
			result.WriteString(".*")
		}

		// Quote any regular expression meta characters in the literal text.
		result.WriteString(regexp.QuoteMeta(literal))
	}

	re, err := regexp.Compile("^" + result.String() + "$")
	if err != nil {
		return nil, fmt.Errorf("compile regexp: %w", err)
	}

	return re, nil
}
