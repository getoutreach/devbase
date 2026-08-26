// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 no-unreachable-types rule.

// unreachable_types.go implements 1 (of 10) Tier 3 custom rules:
// no-unreachable-types, which forbids a type or directive definition
// that no root operation type's fields can ever reach, matching
// @graphql-eslint's rule of the same name.
//
// reachableTypeNames ports @graphql-eslint's own getReachableTypes: a
// breadth-first walk seeded from the schema's root operation types, the
// schema definition's own applied directives, and any directive
// definition usable at a request-execution location (@graphql-eslint's
// own RequestDirectiveLocations), following every named type a reached
// definition's fields, arguments, interfaces, union members, and applied
// directives refer to. An interface is a deliberate special case,
// carried over from @graphql-eslint unchanged: reaching one reaches
// every known implementation of it instead of the interface's own body,
// since graphql-js resolves an interface-typed field through whichever
// concrete object actually implements it.
//
// Unlike alphabetize and no-case-insensitive-enum-values-duplicates,
// this rule reports a base definition and each of its extensions
// separately, even though validator.ValidateSchemaDocument merges an
// extension's Fields and EnumValues into the base Definition for the
// reachability walk itself: @graphql-eslint's own tests expect one
// violation per `type`/`extend type` occurrence sharing an unreachable
// name, not one deduplicated per name (see unreachable_types_test.go).

package lint

import (
	"slices"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// requestDirectiveLocations are the 8 directive locations
// @graphql-eslint's own RequestDirectiveLocations treats as "usable
// while executing a request": a directive definition usable at one of
// these is reachable on its own, without anything in the schema
// referring to it.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var requestDirectiveLocations = set.Of(
	ast.LocationQuery, ast.LocationMutation, ast.LocationSubscription, ast.LocationField,
	ast.LocationFragmentDefinition, ast.LocationFragmentSpread, ast.LocationInlineFragment,
	ast.LocationVariableDefinition,
)

// hasRequestLocation reports whether dd is usable at one of
// requestDirectiveLocations.
func hasRequestLocation(dd *ast.DirectiveDefinition) bool {
	return requestDirectiveLocations.ContainsAny(slices.Values(dd.Locations))
}

// reachableTypeNames returns the name of every type and directive
// reachable from schema's root operation types, per this file's package
// doc comment.
func reachableTypeNames(schema *ast.Schema) set.Set[string] {
	reached := set.Of[string]()

	var collect func(name string)
	collect = func(name string) {
		if !reached.Insert(name) {
			return
		}

		if def, ok := schema.Types[name]; ok {
			if def.Kind == ast.Interface {
				// Reaching an interface reaches every known
				// implementation of it instead of the interface's own
				// body; see this file's package doc comment.
				for _, impl := range schema.PossibleTypes[name] {
					collect(impl.Name)
				}
				return
			}
			for _, iface := range def.Interfaces {
				collect(iface)
			}
			for _, dir := range def.Directives {
				collect(dir.Name)
			}
			for _, f := range def.Fields {
				collect(f.Type.Name())
				for _, arg := range f.Arguments {
					collect(arg.Type.Name())
					for _, dir := range arg.Directives {
						collect(dir.Name)
					}
				}
				for _, dir := range f.Directives {
					collect(dir.Name)
				}
			}
			for _, member := range def.Types {
				collect(member)
			}
			for _, ev := range def.EnumValues {
				for _, dir := range ev.Directives {
					collect(dir.Name)
				}
			}
			return
		}

		if dd, ok := schema.Directives[name]; ok {
			for _, arg := range dd.Arguments {
				collect(arg.Type.Name())
				for _, dir := range arg.Directives {
					collect(dir.Name)
				}
			}
		}
	}

	for _, root := range []*ast.Definition{schema.Query, schema.Mutation, schema.Subscription} {
		if root != nil {
			collect(root.Name)
		}
	}
	for _, dir := range schema.SchemaDirectives {
		collect(dir.Name)
	}
	for name, dd := range schema.Directives {
		if hasRequestLocation(dd) {
			// Marked reachable directly, not via collect: a directive
			// usable at a request-execution location needs nothing in
			// the schema to reference it, and @graphql-eslint itself
			// never walks its arguments for this case either.
			reached.Insert(name)
		}
	}

	return reached
}

// unreachableTypeKindLabels formats a type-definition or type-extension
// kind the way @graphql-eslint's own no-unreachable-types message does:
// for example "Object type", "Input object type" -- not typeKindLabel
// (descriptions.go), which is lowercase with no "type" suffix, for a
// different message shape.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var unreachableTypeKindLabels = map[ast.DefinitionKind]string{
	ast.Scalar:      "Scalar type",
	ast.Object:      "Object type",
	ast.Interface:   "Interface type",
	ast.Union:       "Union type",
	ast.Enum:        "Enum type",
	ast.InputObject: "Input object type",
}

// unreachableSite is one type or directive definition/extension
// no-unreachable-types checks: its own name, the label its kind reports
// under, and the position its violation (if any) is reported at.
type unreachableSite struct {
	pos   *ast.Position
	name  string
	label string
}

// unreachableSitesByFile collects every unreachableSite in doc --
// walking doc.Definitions and doc.Extensions separately, unlike
// forEachDefinition (descriptions.go), since a base definition and each
// of its extensions are each their own site here -- grouped by the
// *ast.Source it was written in, with each group sorted by pos.Start.
// inScope restricts the walk to sites actually written in one of the
// repository's own files.
func unreachableSitesByFile(doc *ast.SchemaDocument, inScope set.Set[*ast.Source]) map[*ast.Source][]unreachableSite {
	sites := make(map[*ast.Source][]unreachableSite)
	add := func(pos *ast.Position, name, label string) {
		if pos == nil || !inScope.Contains(pos.Src) {
			return
		}
		sites[pos.Src] = append(sites[pos.Src], unreachableSite{pos: pos, name: name, label: label})
	}

	for _, def := range doc.Definitions {
		add(def.Position, def.Name, unreachableTypeKindLabels[def.Kind])
	}
	for _, ext := range doc.Extensions {
		add(ext.Position, ext.Name, unreachableTypeKindLabels[ext.Kind])
	}
	for _, dd := range doc.Directives {
		add(dd.Position, dd.Name, "Directive")
	}

	sortGroupsByPosition(sites, func(s unreachableSite) *ast.Position { return s.pos })
	return sites
}

// noUnreachableTypesViolations reports a RuleNoUnreachableTypes
// violation for every site in sites whose name is not in reachable.
func noUnreachableTypesViolations(sites []unreachableSite, reachable set.Set[string]) []Violation {
	violations := make([]Violation, 0, len(sites))
	for _, s := range sites {
		if reachable.Contains(s.name) {
			continue
		}
		violations = append(violations, Violation{
			err:  gqlerror.ErrorPosf(s.pos, "%s `%s` is unreachable.", s.label, s.name),
			Rule: config.RuleNoUnreachableTypes,
		})
	}
	return violations
}

// tier3NoUnreachableTypes runs no-unreachable-types against parsed, in
// fileSources order. It does not run unless cfg enables it
// (config.Lint.Enabled).
func tier3NoUnreachableTypes(fileSources []*ast.Source, inScope set.Set[*ast.Source],
	parsed *parsedSchema, cfg *config.Lint,
) []Violation {
	if !cfg.Enabled(config.RuleNoUnreachableTypes) {
		return nil
	}

	reachable := reachableTypeNames(parsed.schema)
	sitesByFile := unreachableSitesByFile(parsed.doc, inScope)

	var violations []Violation
	for _, src := range fileSources {
		violations = append(violations, noUnreachableTypesViolations(sitesByFile[src], reachable)...)
	}
	return violations
}
