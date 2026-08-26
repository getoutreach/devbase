// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 no-case-insensitive-enum-values-duplicates rule.

// case_insensitive_enum_values_duplicates.go implements 1 (of 10) Tier 3
// custom rules: no-case-insensitive-enum-values-duplicates, which forbids
// two values on the same enum whose names differ only by casing,
// matching @graphql-eslint's rule of the same name.
//
// A type extension's enum values are checked under its base enum, via
// forEachDefinition (descriptions.go): validator.ValidateSchemaDocument
// already merges an extension's EnumValues into the base Definition's,
// so a duplicate split across a base enum and its extension -- or across
// two extensions in different files -- is caught the same as one written
// in a single enum block. @graphql-eslint itself cannot see this case: it
// lints one file's own AST at a time, so a duplicate spread across files
// is invisible to it.

package lint

import (
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// caseInsensitiveEnumDuplicateViolations reports a
// RuleNoCaseInsensitiveEnumValuesDuplicates violation for every enum
// value in doc whose name matches an earlier value on the same enum,
// case-insensitively. inScope restricts the walk to enum values actually
// written in one of the repository's own files.
func caseInsensitiveEnumDuplicateViolations(doc *ast.SchemaDocument, inScope set.Set[*ast.Source]) []Violation {
	var violations []Violation

	checkValues := func(def *ast.Definition) {
		if def.Kind != ast.Enum {
			return
		}
		seenLower := set.Of[string]()
		for _, v := range def.EnumValues {
			if v.Position == nil || !inScope.Contains(v.Position.Src) {
				continue
			}
			lower := strings.ToLower(v.Name)
			if !seenLower.Insert(lower) {
				violations = append(violations, Violation{
					err: gqlerror.ErrorPosf(v.Position,
						"Case-insensitive enum values duplicates are not allowed! Found: `%s`.", v.Name),
					Rule: config.RuleNoCaseInsensitiveEnumValuesDuplicates,
				})
			}
		}
	}

	forEachDefinition(doc, checkValues, checkValues)

	return violations
}

// tier3NoCaseInsensitiveEnumValuesDuplicates runs
// no-case-insensitive-enum-values-duplicates against parsed. It does not
// run unless cfg enables it (config.Lint.Enabled).
func tier3NoCaseInsensitiveEnumValuesDuplicates(parsed *parsedSchema, inScope set.Set[*ast.Source],
	cfg *config.Lint,
) []Violation {
	if !cfg.Enabled(config.RuleNoCaseInsensitiveEnumValuesDuplicates) {
		return nil
	}
	return caseInsensitiveEnumDuplicateViolations(parsed.doc, inScope)
}
