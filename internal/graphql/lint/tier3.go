// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared entry point for the Tier 3 custom rules,
// computing the descriptionSite walk they all read at most once.

package lint

import (
	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
)

// tier3 runs every Tier 3 custom rule implemented so far against
// parsed: require-description and description-style (descriptions.go),
// no-hashtag-description (hashtag.go), and require-deprecation-reason
// and require-deprecation-date (deprecation.go). None of them run
// unless cfg enables them (config.Lint.Enabled).
// groupDescriptionSites, the walk all five read, is computed at most
// once here regardless of how many of them are enabled, rather than
// once per rule.
func tier3(fileSources []*ast.Source, parsed *parsedSchema, cfg *config.Lint) ([]Violation, error) {
	if !cfg.Enabled(config.RuleRequireDescription) &&
		!cfg.Enabled(config.RuleDescriptionStyle) &&
		!cfg.Enabled(config.RuleNoHashtagDescription) &&
		!cfg.Enabled(config.RuleRequireDeprecationReason) &&
		!cfg.Enabled(config.RuleRequireDeprecationDate) {
		return nil, nil
	}

	roots := rootTypeNameSet(parsed.schema)
	sitesByFile := groupDescriptionSites(parsed.doc, roots)

	violations, err := tier3Descriptions(fileSources, sitesByFile, cfg)
	if err != nil {
		return nil, err
	}

	violations = append(violations, tier3Deprecation(sitesByFile, cfg)...)

	hashtagViolations, err := tier3NoHashtagDescription(fileSources, sitesByFile, cfg)
	if err != nil {
		return nil, err
	}
	violations = append(violations, hashtagViolations...)

	return violations, nil
}
