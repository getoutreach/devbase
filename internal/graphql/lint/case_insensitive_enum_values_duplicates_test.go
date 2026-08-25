// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the no-case-insensitive-enum-values-duplicates
// Tier 3 rule.

// Invalid cases below are ported from @graphql-eslint/eslint-plugin's own
// test suite (packages/plugin/tests/no-case-insensitive-enum-values-duplicates.spec.ts,
// @graphql-eslint/eslint-plugin@3.13.1, the version this port targets);
// that suite has no valid cases of its own.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestNoCaseInsensitiveEnumValuesDuplicatesValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
	}{
		{"distinct values", `enum A { TEST OTHER }`},
		{"distinct values across an extension", `enum A { TEST } extend enum A { OTHER }`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleNoCaseInsensitiveEnumValuesDuplicates))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestNoCaseInsensitiveEnumValuesDuplicatesInvalidCases(t *testing.T) {
	runSingleViolationCases(t, config.RuleNoCaseInsensitiveEnumValuesDuplicates,
		"Case-insensitive enum values duplicates are not allowed! Found: `TesT`.",
		[]singleViolationCase{
			{"on a base enum", `enum A { TEST TesT }`},
			{"on a dangling extension", `extend enum A { TEST TesT }`},
		})
}

// TestNoCaseInsensitiveEnumValuesDuplicatesAcrossFiles is a Go-specific
// edge case: validator.ValidateSchemaDocument merges an extension's
// EnumValues into its base enum's own Definition.EnumValues before Tier 3
// ever runs (see case_insensitive_enum_values_duplicates.go), so a
// duplicate split across a base enum in one file and its extension in
// another is still caught. @graphql-eslint itself cannot see this case:
// it lints one file's own AST at a time.
func TestNoCaseInsensitiveEnumValuesDuplicatesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "base.graphql", `enum A { TEST }`)
	ext := writeFile(t, dir, "ext.graphql", `extend enum A { TesT }`)

	violations, err := Files([]string{base, ext}, enableRule(config.RuleNoCaseInsensitiveEnumValuesDuplicates))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleNoCaseInsensitiveEnumValuesDuplicates)
	assert.ErrorContains(t, violations[0].err, "Found: `TesT`.")
}
