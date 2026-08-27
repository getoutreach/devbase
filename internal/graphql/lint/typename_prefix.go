// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 no-typename-prefix rule.

// typename_prefix.go implements 1 (of 10) Tier 3 custom rules:
// no-typename-prefix, which forbids a field on an object or interface
// type from starting with that type's own name (case-insensitively),
// matching @graphql-eslint's rule of the same name.
//
// A type extension's fields are checked under its base type's name, via
// forEachDefinition (descriptions.go).

package lint

import (
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// typenamePrefixViolations reports a RuleNoTypenamePrefix violation for
// every field of an object or interface type in defs whose name starts
// with that type's own name, case-insensitively. inScope restricts the
// walk to definitions actually written in one of the repository's own
// files, excluding gqlparser's built-in prelude and any federation- or
// scalars-synthesized prelude (see federation.go).
func typenamePrefixViolations(defs []scopedDefinition, inScope set.Set[*ast.Source]) []Violation {
	var violations []Violation

	for _, sd := range defs {
		def := sd.def
		if def.Kind != ast.Object && def.Kind != ast.Interface {
			continue
		}
		lowerTypeName := strings.ToLower(def.Name)
		for _, f := range def.Fields {
			if f.Position == nil || !inScope.Contains(f.Position.Src) {
				// A nil Position is one of the __schema/__type
				// introspection meta-fields (see descriptions.go); an
				// out-of-scope Position is a prelude- or
				// federation-synthesized field. Neither was written in
				// a repository file, so neither is lintable here.
				continue
			}
			if strings.HasPrefix(strings.ToLower(f.Name), lowerTypeName) {
				violations = append(violations, Violation{
					err: gqlerror.ErrorPosf(f.Position,
						`Field "%s" starts with the name of the parent type "%s"`, f.Name, def.Name),
					Rule: config.RuleNoTypenamePrefix,
				})
			}
		}
	}

	return violations
}

// tier3NoTypenamePrefix runs no-typename-prefix against defs. It does
// not run unless cfg enables it (config.Lint.Enabled).
func tier3NoTypenamePrefix(defs []scopedDefinition, inScope set.Set[*ast.Source], cfg *config.Lint) []Violation {
	if !cfg.Enabled(config.RuleNoTypenamePrefix) {
		return nil
	}
	return typenamePrefixViolations(defs, inScope)
}
