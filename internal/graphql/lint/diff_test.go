// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for New, the violation-set comparison behind
// --diff mode.

package lint

import (
	"testing"

	"gotest.tools/v3/assert"
)

// TestNew covers New's baseline/current comparison: an unchanged
// violation is dropped, a violation absent from the baseline is kept,
// a repeated violation is kept only for its excess occurrences beyond
// the baseline count, and a pure position shift (same file, rule, and
// message) doesn't make a violation "new".
func TestNew(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", "type Foo { a: String }\ntype Foo { b: String }\n")

	baseline, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(baseline), 1)

	t.Run("drops a violation unchanged since the baseline", func(t *testing.T) {
		current, err := Files([]string{path}, nil)
		assert.NilError(t, err)
		assert.Equal(t, len(New(baseline, current)), 0)
	})

	t.Run("keeps a violation whose position shifted but content didn't", func(t *testing.T) {
		writeFile(t, dir, "schema.graphql", "\ntype Foo { a: String }\ntype Foo { b: String }\n")
		current, err := Files([]string{path}, nil)
		assert.NilError(t, err)
		// current's violation is at a different line than baseline's,
		// but the shift alone must not count as a new violation.
		assert.Equal(t, len(New(baseline, current)), 0)
	})

	t.Run("keeps a violation absent from the baseline", func(t *testing.T) {
		current, err := Files([]string{path}, nil)
		assert.NilError(t, err)
		current = append(current, Violation{err: baseline[0].err, Rule: "other-rule"})

		newViolations := New(baseline, current)
		assert.Equal(t, len(newViolations), 1)
		assert.Equal(t, newViolations[0].Rule, "other-rule")
	})

	t.Run("keeps only the excess occurrences of a repeated violation", func(t *testing.T) {
		current := append([]Violation{}, baseline...)
		current = append(current, baseline...)

		assert.Equal(t, len(New(baseline, current)), 1)
	})
}
