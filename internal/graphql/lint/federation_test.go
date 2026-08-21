// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Apollo Federation and custom-scalar
// prelude Files merges in per scripts/devbase.yaml.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

// TestFilesFederationImportedDirectivesPass confirms that a schema
// using exactly the directives its own @link imports, matching
// getoutreach/giraffe's real schema, passes with graphql.lint.federation
// set, including a directive (@key) whose SDL references the FieldSet
// scalar.
func TestFilesFederationImportedDirectivesPass(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@key", "@shareable", "@override", "@inaccessible"])

		type Widget @key(fields: "id") {
			id: ID!
			name: String @shareable
			legacyName: String @override(from: "legacy") @inaccessible
		}
	`)

	violations, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestFilesFederationDirectiveNotImportedFails confirms the real
// point of import-list awareness: a directive that exists in the
// Federation spec but was never imported by this schema's own @link
// still fails, with the same "Undefined directive" classification
// Files already reports for any other undeclared directive --
// because the merged prelude only ever defines what was imported.
func TestFilesFederationDirectiveNotImportedFails(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@key"])

		type Widget @key(fields: "id") {
			id: ID!
			name: String @shareable
		}
	`)

	violations, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleKnownDirectives)
	assert.ErrorContains(t, violations[0].err, "Undefined directive shareable.")
}

// TestFilesFederationImportAsRenameHonored confirms that a `@link`
// import renamed via `as` makes the directive available only under
// its alias -- the schema's use of the original, un-renamed name still
// fails as undefined.
func TestFilesFederationImportAsRenameHonored(t *testing.T) {
	dir := t.TempDir()

	t.Run("alias name passes", func(t *testing.T) {
		path := writeFile(t, dir, "alias.graphql", `
			extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: [{name: "@key", as: "@myKey"}])

			type Widget @myKey(fields: "id") {
				id: ID!
			}
		`)
		violations, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
		assert.NilError(t, err)
		assert.Equal(t, len(violations), 0)
	})

	t.Run("original name fails", func(t *testing.T) {
		path := writeFile(t, dir, "original.graphql", `
			extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: [{name: "@key", as: "@myKey"}])

			type Widget @key(fields: "id") {
				id: ID!
			}
		`)
		violations, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
		assert.NilError(t, err)
		assert.Equal(t, len(violations), 1)
		assert.ErrorContains(t, violations[0].err, "Undefined directive key.")
	})
}

// TestFilesFederationVersionMismatchErrors confirms that a schema
// linking a different Federation version than scripts/devbase.yaml
// configures returns ErrFederationVersionMismatch, rather than
// silently passing or failing against the wrong directive signatures.
func TestFilesFederationVersionMismatchErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://specs.apollo.dev/federation/v2.5", import: ["@key"])
		type Widget @key(fields: "id") { id: ID! }
	`)

	_, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.ErrorIs(t, err, ErrFederationVersionMismatch)
}

// TestFilesFederationUnsupportedConfiguredVersionErrors confirms that
// an unrecognized graphql.lint.federation value returns
// ErrUnsupportedFederationVersion, rather than a silent no-op.
func TestFilesFederationUnsupportedConfiguredVersionErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Widget { id: ID! }`)

	_, err := Files([]string{path}, &config.LintConfig{Federation: "v9.9"})
	assert.ErrorIs(t, err, ErrUnsupportedFederationVersion)
}

// TestFilesFederationUnsupportedImportedDirectiveErrors confirms that
// importing a real Federation directive this package has not
// implemented a signature for (added in a later spec version than
// v2.3) returns ErrUnsupportedFederationDirective, rather than
// guessing at a signature.
func TestFilesFederationUnsupportedImportedDirectiveErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@authenticated"])
		type Widget { id: ID! }
	`)

	_, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.ErrorIs(t, err, ErrUnsupportedFederationDirective)
}

// TestFilesFederationUnrelatedLinkIgnored confirms that a `@link` to
// some spec other than Apollo Federation is left untouched -- Files
// neither synthesizes directives for it nor treats its version as a
// Federation version to check.
func TestFilesFederationUnrelatedLinkIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://example.com/some-other-spec/v1.0")
		type Widget { id: ID! }
	`)

	violations, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, "Undefined directive link.")
}

// TestFilesFederationConfiguredButUnusedIsANoop confirms that opting
// into graphql.lint.federation does not affect a schema that never
// links the Federation spec.
func TestFilesFederationConfiguredButUnusedIsANoop(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Widget { id: ID! }`)

	violations, err := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestFilesFederationSyntaxErrorClassifiedSameAsWithoutFederation
// confirms that a plain SDL syntax error, unrelated to federation, is
// reported as the same kind of Violation whether or not
// graphql.lint.federation is configured, even though a configured
// federation setting makes Files parse paths once looking for @link
// before gqlparser.LoadSchema's own parse.
func TestFilesFederationSyntaxErrorClassifiedSameAsWithoutFederation(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Widget { id: ID! `) // missing closing brace

	without, errWithout := Files([]string{path}, nil)
	assert.NilError(t, errWithout)

	with, errWith := Files([]string{path}, &config.LintConfig{Federation: "v2.3"})
	assert.NilError(t, errWith)

	assert.Equal(t, len(without), 1)
	assert.Equal(t, len(with), 1)
	assert.Equal(t, with[0].Rule, without[0].Rule)
	assert.Equal(t, with[0].String(), without[0].String())
}

// TestFilesScalarsConfigDeclaresRuntimeRegisteredScalars confirms
// that graphql.lint.scalars lets a schema use a scalar type that is
// registered at runtime in application code rather than declared via
// SDL, matching giraffe's Datetime/JSON/Number/Relationship scalars.
func TestFilesScalarsConfigDeclaresRuntimeRegisteredScalars(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Widget {
			createdAt: Datetime
			meta: JSON
		}
	`)

	t.Run("without config, undeclared scalar fails", func(t *testing.T) {
		violations, err := Files([]string{path}, nil)
		assert.NilError(t, err)
		assert.Equal(t, len(violations), 1)
		assert.Equal(t, violations[0].Rule, config.RuleKnownTypeNames)
	})

	t.Run("with config, declared scalars pass", func(t *testing.T) {
		violations, err := Files([]string{path}, &config.LintConfig{Scalars: []string{"Datetime", "JSON"}})
		assert.NilError(t, err)
		assert.Equal(t, len(violations), 0)
	})
}
