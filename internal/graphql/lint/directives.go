// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 2 gap-fill passes for unique-directive-names-per-location
// and possible-type-extension.

// directives.go fills the two gaps left in schema validation once Files'
// Tier 1 pass has already accepted a schema: a non-repeatable directive
// used more than once at one syntactic location, and a type extension whose
// base type was never actually defined.
//
// gqlparser's own directive-uniqueness check does not apply to type-level
// directives (on a type, interface, union, enum, input object, or scalar
// keyword) at all: it merges a type's base definition and every extension
// of it into one directive list before validating, and -- since a base
// definition and its extensions are distinct locations in the general case
// -- skips the once-per-location check for that merged list entirely,
// rather than risk rejecting directives legitimately repeated across
// separate extensions. graphql-js's UniqueDirectivesPerLocationRule, which
// @graphql-eslint delegates to for SDL documents, takes the opposite,
// narrower view: it groups a type's base definition and all of its
// extensions together as a single location, so a non-repeatable directive
// used twice anywhere in that group -- whether on the base definition, on
// one extension, or split across two -- is a violation.
// gapFillDirectivesPerLocation reproduces that grouping by reading
// parsed.schema.Types[name].Directives and parsed.schema.SchemaDirectives:
// validator.ValidateSchemaDocument already merges a type's base definition
// with every extension of it (and the schema definition with every schema
// extension) into those fields while building parsed.schema, which is
// exactly the grouping this rule needs.
//
// gqlparser also does not report a type extension whose base type was never
// defined: LoadSchema silently synthesizes an empty placeholder type for it
// instead of erroring, so any extension of a nonexistent type validates as
// if the type existed. gapFillPossibleTypeExtension detects this by
// comparing each extension's target name against the set of types actually
// defined (not merely extended) somewhere among the schema's own sources,
// gqlparser's built-in prelude, and any federation- or scalars-synthesized
// prelude scripts/devbase.yaml configured -- parsed.schema.Types can't
// answer this, since it holds the synthesized placeholder indistinguishably
// from a real definition, so this pass reads parsed.doc.Definitions and
// parsed.doc.Extensions instead.
//
// Unlike Tier 1, neither rule runs unless scripts/devbase.yaml enables it
// (config.Lint.Enabled): @graphql-eslint's own upstream config
// generation excludes both from every preset it ships, including the
// permissive "all" preset (isDisabledForAllConfig in
// packages/plugin/src/rules/graphql-js-validation.js, @graphql-eslint/
// eslint-plugin@3.20.1), so a repo adopting devbase graphql lint should
// see the same clean slate it would adopting @graphql-eslint fresh,
// rather than two rules no @graphql-eslint config anyone was using ever
// enabled.

package lint

import (
	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// gapFillDirectivesPerLocation reports a RuleUniqueDirectiveNamesPerLocation
// violation for every non-repeatable directive used more than once at a
// single location: the schema or a named type. It reports nothing unless
// cfg enables the rule -- see config.Lint.Enabled.
func gapFillDirectivesPerLocation(parsed *parsedSchema, cfg *config.Lint) []Violation {
	if !cfg.Enabled(config.RuleUniqueDirectiveNamesPerLocation) {
		return nil
	}

	violations := nonRepeatableDuplicates(parsed.schema, parsed.schema.SchemaDirectives)

	checked := make(map[string]bool, len(parsed.doc.Definitions))
	checkType := func(name string) {
		if checked[name] {
			return
		}
		checked[name] = true
		violations = append(violations, nonRepeatableDuplicates(parsed.schema, parsed.schema.Types[name].Directives)...)
	}
	for _, def := range parsed.doc.Definitions {
		checkType(def.Name)
	}
	for _, ext := range parsed.doc.Extensions {
		checkType(ext.Name)
	}

	return violations
}

// nonRepeatableDuplicates reports a violation for every directive in dirs,
// past the first use of its name, whose definition (looked up in schema) is
// not repeatable. dirs is assumed to all come from one location, per
// gapFillDirectivesPerLocation.
func nonRepeatableDuplicates(schema *ast.Schema, dirs ast.DirectiveList) []Violation {
	if len(dirs) < 2 {
		return nil
	}

	var violations []Violation
	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		def := schema.Directives[dir.Name]
		if def == nil || def.IsRepeatable {
			continue
		}
		if seen[dir.Name] {
			violations = append(violations, Violation{
				err: gqlerror.ErrorPosf(dir.Position,
					`The directive "@%s" can only be used once at this location.`, dir.Name),
				Rule: config.RuleUniqueDirectiveNamesPerLocation,
			})
			continue
		}
		seen[dir.Name] = true
	}
	return violations
}

// gapFillPossibleTypeExtension reports a RulePossibleTypeExtension
// violation for every `extend <kind> Name { ... }` whose base type Name is
// never actually defined in parsed.doc.Definitions -- which, as
// parseAndValidate builds it, includes the repository's own SDL,
// gqlparser's built-in prelude, and any federation- or
// scalars-synthesized prelude. It reports nothing unless cfg enables the
// rule -- see config.Lint.Enabled.
func gapFillPossibleTypeExtension(parsed *parsedSchema, cfg *config.Lint) []Violation {
	if !cfg.Enabled(config.RulePossibleTypeExtension) {
		return nil
	}

	defined := make(map[string]bool, len(parsed.doc.Definitions))
	for _, def := range parsed.doc.Definitions {
		defined[def.Name] = true
	}

	var violations []Violation
	for _, ext := range parsed.doc.Extensions {
		if defined[ext.Name] {
			continue
		}
		violations = append(violations, Violation{
			err:  gqlerror.ErrorPosf(ext.Position, `Cannot extend type "%s" because it is not defined.`, ext.Name),
			Rule: config.RulePossibleTypeExtension,
		})
	}
	return violations
}
