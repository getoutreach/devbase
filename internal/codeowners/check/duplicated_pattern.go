// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Check that finds CODEOWNERS patterns that are defined more than once.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"context"
	"fmt"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
)

// DuplicatedPattern validates if CODEOWNERS file does not contain
// the duplicated lines with the same file pattern.
type DuplicatedPattern struct{}

// NewDuplicatedPattern returns instance of the DuplicatedPattern.
func NewDuplicatedPattern() *DuplicatedPattern {
	return &DuplicatedPattern{}
}

// Check searches for doubles paths(patterns) in CODEOWNERS file. It reports the patterns in the
// order in which they first appear.
func (*DuplicatedPattern) Check(ctx context.Context, in Input) (Output, error) {
	var out Output

	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	var order []string
	patterns := map[string][]codeowners.Entry{}
	for _, entry := range in.Entries {
		if _, seen := patterns[entry.Pattern]; !seen {
			order = append(order, entry.Pattern)
		}
		patterns[entry.Pattern] = append(patterns[entry.Pattern], entry)
	}

	for _, pattern := range order {
		if entries := patterns[pattern]; len(entries) > 1 {
			list := bulletList(entries, func(e codeowners.Entry) string { return fmt.Sprintf("%d: with owners: %s", e.LineNo, e.Owners) })
			out.Report(fmt.Sprintf("Pattern %q is defined %d times in lines:\n%s", pattern, len(entries), list))
		}
	}

	return out, nil
}

// Name returns human readable name of the validator.
func (*DuplicatedPattern) Name() string {
	return "Duplicated Pattern Checker"
}
