// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared entry point for the Tier 3 custom rules,
// computing the descriptionSite walk most of them read at most once.

package lint

import (
	"sync"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
)

// scopedDefinition pairs a definition allDefinitions visits with
// whether it came from doc.Extensions -- a type extension with no base
// definition anywhere -- rather than doc.Definitions.
type scopedDefinition struct {
	def         *ast.Definition
	isExtension bool
}

// allDefinitions flattens doc.Definitions and the doc.Extensions with
// no base definition into one slice, in that order. A based extension
// is omitted: validator.ValidateSchemaDocument already merged its
// Fields and EnumValues into the base Definition, so the base
// definition already covers it.
//
// The name is deliberately not "definitionsInScope": this returns
// every definition in doc, including gqlparser's built-in prelude and
// any federation- or scalars-synthesized prelude (see federation.go).
// Restricting to a repository's own files is a separate step callers
// apply themselves, via the inScope set built in tier3 below.
//
// Every Tier 3 rule needing a definition-level pass over the schema
// shares this one walk.
func allDefinitions(doc *ast.SchemaDocument) []scopedDefinition {
	seen := make(set.Set[string], len(doc.Definitions))
	out := make([]scopedDefinition, 0, len(doc.Definitions)+len(doc.Extensions))
	for _, def := range doc.Definitions {
		seen.Insert(def.Name)
		out = append(out, scopedDefinition{def: def})
	}
	for _, def := range doc.Extensions {
		if !seen.Contains(def.Name) {
			out = append(out, scopedDefinition{def: def, isExtension: true})
		}
	}
	return out
}

// tier3 runs all 10 Tier 3 custom rules against parsed:
// require-description and description-style (descriptions.go),
// no-hashtag-description (hashtag.go), require-deprecation-reason and
// require-deprecation-date (deprecation.go), no-typename-prefix
// (typename_prefix.go), naming-convention (naming_convention.go),
// no-case-insensitive-enum-values-duplicates
// (case_insensitive_enum_values_duplicates.go), alphabetize
// (alphabetize.go), and no-unreachable-types (unreachable_types.go).
// None of them run unless cfg enables them (config.Lint.Enabled).
//
// The first 5 share one descriptionSite walk (groupDescriptionSites);
// no-typename-prefix, naming-convention,
// no-case-insensitive-enum-values-duplicates, and alphabetize share
// allDefinitions instead, since they care about a definition's own
// name, fields, or enum values, not its description. Both walks are
// computed at most once here, shared with whichever rules need them,
// rather than once per rule. no-unreachable-types needs a different,
// schema-wide reachability walk instead, so it keeps its own and never
// triggers either shared walk on its own.
//
// Every rule below only reads parsed and defs, and each writes only its
// own named result variable, so none of them need to wait on another:
// they run as concurrent goroutines (wg.Go), each writing a variable no
// other goroutine touches. wg.Wait's happens-before guarantee makes
// reading those variables afterward, in the fixed order below, safe --
// and that fixed order, not goroutine finishing order, is what decides
// the result's order. defsGroup and no-unreachable-types are launched
// before groupDescriptionSites runs, so that ~17ms walk (needed only by
// the last 3 goroutines below) doesn't delay starting them.
func tier3(fileSources []*ast.Source, parsed *parsedSchema, cfg *config.Lint) ([]Violation, error) {
	descGroupEnabled := cfg.Enabled(config.RuleRequireDescription) || cfg.Enabled(config.RuleDescriptionStyle) ||
		cfg.Enabled(config.RuleNoHashtagDescription) || cfg.Enabled(config.RuleRequireDeprecationReason) ||
		cfg.Enabled(config.RuleRequireDeprecationDate)
	defsGroupEnabled := cfg.Enabled(config.RuleNoTypenamePrefix) || cfg.Enabled(config.RuleNamingConvention) ||
		cfg.Enabled(config.RuleNoCaseInsensitiveEnumValuesDuplicates) || cfg.Enabled(config.RuleAlphabetize)

	var defs []scopedDefinition
	if descGroupEnabled || defsGroupEnabled {
		defs = allDefinitions(parsed.doc)
	}

	var (
		wg                                                                             sync.WaitGroup
		typenameViolations, namingViolations, enumDupViolations, alphabetizeViolations []Violation
		unreachableViolations                                                          []Violation
		descViolations, deprecationViolations, hashtagViolations                       []Violation
		descErr, hashtagErr                                                            error
	)

	if defsGroupEnabled || cfg.Enabled(config.RuleNoUnreachableTypes) {
		// inScope excludes gqlparser's built-in prelude and any
		// federation- or scalars-synthesized prelude (see federation.go)
		// by their *ast.Source identity: neither is ever one of
		// fileSources.
		inScope := set.Of(fileSources...)

		if defsGroupEnabled {
			wg.Go(func() { typenameViolations = tier3NoTypenamePrefix(defs, inScope, cfg) })
			wg.Go(func() { namingViolations = tier3NamingConvention(defs, parsed, inScope, cfg) })
			wg.Go(func() { enumDupViolations = tier3NoCaseInsensitiveEnumValuesDuplicates(defs, inScope, cfg) })
			wg.Go(func() { alphabetizeViolations = tier3Alphabetize(defs, parsed, inScope, cfg) })
		}
		wg.Go(func() { unreachableViolations = tier3NoUnreachableTypes(fileSources, inScope, parsed, cfg) })
	}

	if descGroupEnabled {
		roots := rootTypeNameSet(parsed.schema)
		sitesByFile := groupDescriptionSites(defs, parsed.doc, roots)

		wg.Go(func() { descViolations, descErr = tier3Descriptions(fileSources, sitesByFile, cfg) })
		wg.Go(func() { deprecationViolations = tier3Deprecation(sitesByFile, cfg) })
		wg.Go(func() { hashtagViolations, hashtagErr = tier3NoHashtagDescription(fileSources, sitesByFile, cfg) })
	}

	wg.Wait()

	if descErr != nil {
		return nil, descErr
	}
	if hashtagErr != nil {
		return nil, hashtagErr
	}

	total := len(descViolations) + len(deprecationViolations) + len(hashtagViolations) + len(typenameViolations) +
		len(namingViolations) + len(enumDupViolations) + len(alphabetizeViolations) + len(unreachableViolations)
	violations := make([]Violation, 0, total)
	violations = append(violations, descViolations...)
	violations = append(violations, deprecationViolations...)
	violations = append(violations, hashtagViolations...)
	violations = append(violations, typenameViolations...)
	violations = append(violations, namingViolations...)
	violations = append(violations, enumDupViolations...)
	violations = append(violations, alphabetizeViolations...)
	violations = append(violations, unreachableViolations...)
	return violations, nil
}
