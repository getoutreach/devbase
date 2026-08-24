// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared entry point for the Tier 3 custom rules,
// computing the descriptionSite walk most of them read at most once.

package lint

import (
	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
)

// tier3 runs every Tier 3 custom rule implemented so far against
// parsed: require-description and description-style (descriptions.go),
// no-hashtag-description (hashtag.go), require-deprecation-reason and
// require-deprecation-date (deprecation.go), no-typename-prefix
// (typename_prefix.go), and naming-convention (naming_convention.go).
// None of them run unless cfg enables them (config.Lint.Enabled).
//
// The first 5 share one descriptionSite walk (groupDescriptionSites),
// computed at most once here regardless of how many of them are
// enabled, rather than once per rule. no-typename-prefix and
// naming-convention need a different walk, since they care about a
// definition's own name and a field's parent type, not its
// description; they still share the same inScope source set, also
// computed at most once here.
func tier3(fileSources []*ast.Source, parsed *parsedSchema, cfg *config.Lint) ([]Violation, error) {
	var violations []Violation

	if cfg.Enabled(config.RuleRequireDescription) || cfg.Enabled(config.RuleDescriptionStyle) ||
		cfg.Enabled(config.RuleNoHashtagDescription) || cfg.Enabled(config.RuleRequireDeprecationReason) ||
		cfg.Enabled(config.RuleRequireDeprecationDate) {
		roots := rootTypeNameSet(parsed.schema)
		sitesByFile := groupDescriptionSites(parsed.doc, roots)

		descViolations, err := tier3Descriptions(fileSources, sitesByFile, cfg)
		if err != nil {
			return nil, err
		}
		violations = append(violations, descViolations...)

		violations = append(violations, tier3Deprecation(sitesByFile, cfg)...)

		hashtagViolations, err := tier3NoHashtagDescription(fileSources, sitesByFile, cfg)
		if err != nil {
			return nil, err
		}
		violations = append(violations, hashtagViolations...)
	}

	if cfg.Enabled(config.RuleNoTypenamePrefix) || cfg.Enabled(config.RuleNamingConvention) {
		// inScope excludes gqlparser's built-in prelude and any
		// federation- or scalars-synthesized prelude (see federation.go)
		// by their *ast.Source identity -- neither is ever one of
		// fileSources.
		inScope := set.Of(fileSources...)
		violations = append(violations, tier3NoTypenamePrefix(parsed, inScope, cfg)...)
		violations = append(violations, tier3NamingConvention(parsed, inScope, cfg)...)
	}

	return violations, nil
}
