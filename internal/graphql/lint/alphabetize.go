// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 alphabetize rule.

// alphabetize.go implements 1 (of 10) Tier 3 custom rules: alphabetize,
// which requires alphabetical order among the fields, enum values,
// and/or arguments its "fields", "values", and "arguments" options
// select, matching @graphql-eslint's rule of the same name.
//
// devbase graphql lint only ever parses schema files (see lint.go's
// Files), so this port narrows @graphql-eslint's own option surface to
// its "schema" configOptions -- "fields", "values", and "arguments" --
// dropping "selections" and "variables" (operation-only) and
// "definitions" (a v4-only option that was already a no-op in every
// repo's config carried into this migration; see RFC 0006's Tier 3
// table).
//
// A type extension's fields or enum values are ordered together with
// its base type's own, via allDefinitions (tier3.go):
// validator.ValidateSchemaDocument already merges an extension's Fields
// and EnumValues into the base Definition's, so this rule sees one
// alphabetical run across a type's base definition and every extension
// of it, wherever each is written. @graphql-eslint itself cannot do
// this: it lints one file's own AST at a time, so it only orders a base
// definition's own fields against each other, and separately orders
// each extension's own fields against each other.

package lint

import (
	"strings"
	"unicode"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// alphabetizeFieldKinds maps alphabetize's "fields" option entries to
// the ast.DefinitionKind each names, restricted to the 3 kinds
// @graphql-eslint's own fieldsEnum allows.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var alphabetizeFieldKinds = map[string]ast.DefinitionKind{
	"ObjectTypeDefinition":      ast.Object,
	"InterfaceTypeDefinition":   ast.Interface,
	"InputObjectTypeDefinition": ast.InputObject,
}

// alphabetizeOptions is alphabetize's scripts/devbase.yaml options,
// decoded from Rule.Options.
type alphabetizeOptions struct {
	// fieldKinds is which of Object, Interface, and InputObject the
	// "fields" option selects.
	fieldKinds set.Set[ast.DefinitionKind]

	// values is whether the "values" option selects EnumTypeDefinition
	// -- the only value @graphql-eslint's own valuesEnum allows.
	values bool

	// fieldArguments and directiveArguments are whether the
	// "arguments" option selects FieldDefinition and DirectiveDefinition,
	// respectively.
	fieldArguments     bool
	directiveArguments bool
}

// parseAlphabetizeOptions decodes opts (Rule.Options) into
// alphabetizeOptions. Any option absent from opts, or opts itself nil,
// leaves the corresponding check disabled, matching @graphql-eslint's
// own behavior of a kind being excluded until its option names it.
func parseAlphabetizeOptions(opts map[string]any) alphabetizeOptions {
	o := alphabetizeOptions{fieldKinds: set.Of[ast.DefinitionKind]()}
	for _, key := range stringSliceOption(opts, "fields") {
		if kind, ok := alphabetizeFieldKinds[key]; ok {
			o.fieldKinds.Insert(kind)
		}
	}
	for _, key := range stringSliceOption(opts, "values") {
		if key == typeDefinitionKindKeys[ast.Enum] {
			o.values = true
		}
	}
	for _, key := range stringSliceOption(opts, "arguments") {
		switch key {
		case fieldDefinitionSelector:
			o.fieldArguments = true
		case directiveDefinitionSelector:
			o.directiveArguments = true
		}
	}
	return o
}

// alphaName is one name alphabetize compares against its predecessor:
// a field, enum value, or argument, at the position its violation (if
// any) should be reported at.
type alphaName struct {
	name string
	pos  *ast.Position
}

// inScopeNames filters names to those written in one of inScope's
// sources, dropping a nil Position (an injected introspection
// meta-field; see descriptions.go) the same way.
func inScopeNames(names []alphaName, inScope set.Set[*ast.Source]) []alphaName {
	out := make([]alphaName, 0, len(names))
	for _, n := range names {
		if n.pos != nil && inScope.Contains(n.pos.Src) {
			out = append(out, n)
		}
	}
	return out
}

// localeCompare mimics JavaScript's default String.prototype.localeCompare
// closely enough for alphabetize's own comparisons, which @graphql-eslint
// itself uses to order names: case-insensitive at the primary level, so
// names group by base letter regardless of case, with a
// lowercase-before-uppercase tiebreak when two names are
// case-insensitively equal -- the opposite of plain byte order, where an
// uppercase letter sorts before its lowercase counterpart.
func localeCompare(a, b string) int {
	if la, lb := strings.ToLower(a), strings.ToLower(b); la != lb {
		if la < lb {
			return -1
		}
		return 1
	}

	ra, rb := []rune(a), []rune(b)
	for i := 0; i < len(ra) && i < len(rb); i++ {
		if ra[i] == rb[i] {
			continue
		}
		if aLower, bLower := unicode.IsLower(ra[i]), unicode.IsLower(rb[i]); aLower != bLower {
			if aLower {
				return -1
			}
			return 1
		}
		if ra[i] < rb[i] {
			return -1
		}
		return 1
	}

	switch {
	case len(ra) < len(rb):
		return -1
	case len(ra) > len(rb):
		return 1
	default:
		return 0
	}
}

// checkAlphaOrder reports a RuleAlphabetize violation for every name in
// names that sorts after its successor per localeCompare.
func checkAlphaOrder(names []alphaName) []Violation {
	var violations []Violation
	for i := 1; i < len(names); i++ {
		prev, curr := names[i-1], names[i]
		if localeCompare(prev.name, curr.name) == 1 {
			violations = append(violations, Violation{
				err:  gqlerror.ErrorPosf(curr.pos, "`%s` should be before `%s`.", curr.name, prev.name),
				Rule: config.RuleAlphabetize,
			})
		}
	}
	return violations
}

// fieldNames returns fields as alphaNames, in order.
func fieldNames(fields ast.FieldList) []alphaName {
	names := make([]alphaName, len(fields))
	for i, f := range fields {
		names[i] = alphaName{name: f.Name, pos: f.Position}
	}
	return names
}

// enumValueNames returns values as alphaNames, in order.
func enumValueNames(values ast.EnumValueList) []alphaName {
	names := make([]alphaName, len(values))
	for i, v := range values {
		names[i] = alphaName{name: v.Name, pos: v.Position}
	}
	return names
}

// argumentNames returns args as alphaNames, in order.
func argumentNames(args ast.ArgumentDefinitionList) []alphaName {
	names := make([]alphaName, len(args))
	for i, a := range args {
		names[i] = alphaName{name: a.Name, pos: a.Position}
	}
	return names
}

// alphabetizeViolations reports every RuleAlphabetize violation in defs
// and directives that opts selects. inScope restricts every check to
// names actually written in one of the repository's own files.
func alphabetizeViolations(defs []scopedDefinition, directives ast.DirectiveDefinitionList,
	inScope set.Set[*ast.Source], opts alphabetizeOptions,
) []Violation {
	var violations []Violation

	for _, sd := range defs {
		def := sd.def
		if opts.fieldKinds.Contains(def.Kind) {
			violations = append(violations, checkAlphaOrder(inScopeNames(fieldNames(def.Fields), inScope))...)
		}
		if opts.values && def.Kind == ast.Enum {
			violations = append(violations, checkAlphaOrder(inScopeNames(enumValueNames(def.EnumValues), inScope))...)
		}
		if opts.fieldArguments && (def.Kind == ast.Object || def.Kind == ast.Interface) {
			for _, f := range def.Fields {
				violations = append(violations, checkAlphaOrder(inScopeNames(argumentNames(f.Arguments), inScope))...)
			}
		}
	}

	if opts.directiveArguments {
		for _, dd := range directives {
			violations = append(violations, checkAlphaOrder(inScopeNames(argumentNames(dd.Arguments), inScope))...)
		}
	}

	return violations
}

// tier3Alphabetize runs alphabetize against defs. It does not run
// unless cfg enables it (config.Lint.Enabled).
func tier3Alphabetize(defs []scopedDefinition, parsed *parsedSchema, inScope set.Set[*ast.Source],
	cfg *config.Lint,
) []Violation {
	if !cfg.Enabled(config.RuleAlphabetize) {
		return nil
	}
	opts := parseAlphabetizeOptions(cfg.Options(config.RuleAlphabetize))
	return alphabetizeViolations(defs, parsed.doc.Directives, inScope, opts)
}
