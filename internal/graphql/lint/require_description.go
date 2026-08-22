// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The require-description Tier 3 rule.

// require_description.go implements require-description: a
// description is required on the kinds of definition its options
// enable it for, matching @graphql-eslint's own rule of the same name.

package lint

import (
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// requireDescriptionOptions is require-description's
// scripts/devbase.yaml options, decoded from Rule.Options. Field
// names match @graphql-eslint's own option keys.
type requireDescriptionOptions struct {
	types                bool
	rootField            bool
	directiveDefinition  bool
	fieldDefinition      bool
	inputValueDefinition bool
	enumValueDefinition  bool

	// perKindOverride holds an explicit true/false for one of
	// typeDefinitionKindKeys, overriding the blanket types option for
	// that one kind -- for example {types: true, ObjectTypeDefinition:
	// false} requires a description on every TYPES_KINDS definition
	// except ObjectTypeDefinition. A kind absent from this map falls
	// back to types.
	perKindOverride map[string]bool
}

// parseRequireDescriptionOptions decodes opts (Rule.Options) into
// requireDescriptionOptions. Any key absent from opts, or opts itself
// nil, defaults to false/disabled, matching @graphql-eslint's own
// behavior of a kind being excluded until its option is set to true.
func parseRequireDescriptionOptions(opts map[string]any) requireDescriptionOptions {
	o := requireDescriptionOptions{
		types:                boolOption(opts, optionTypes),
		rootField:            boolOption(opts, optionRootField),
		directiveDefinition:  boolOption(opts, optionDirectiveDefinition),
		fieldDefinition:      boolOption(opts, optionFieldDefinition),
		inputValueDefinition: boolOption(opts, optionInputValueDefinition),
		enumValueDefinition:  boolOption(opts, optionEnumValueDefinition),
	}
	for _, key := range typeDefinitionKindKeys {
		if v, ok := opts[key]; ok {
			if o.perKindOverride == nil {
				o.perKindOverride = make(map[string]bool, len(typeDefinitionKindKeys))
			}
			b, _ := v.(bool)
			o.perKindOverride[key] = b
		}
	}
	return o
}

// applies reports whether o requires a description on s.
func (o requireDescriptionOptions) applies(s *descriptionSite) bool {
	switch s.optionKind {
	case optionTypes:
		if override, ok := o.perKindOverride[s.typeDefKindKey]; ok {
			return override
		}
		return o.types
	case optionDirectiveDefinition:
		return o.directiveDefinition
	case optionFieldDefinition:
		return o.fieldDefinition || (o.rootField && s.isRootField)
	case optionInputValueDefinition:
		return o.inputValueDefinition
	case optionEnumValueDefinition:
		return o.enumValueDefinition
	default:
		// optionSchemaDefinition: require-description never checks a
		// schema definition's own description, matching
		// @graphql-eslint's ALLOWED_KINDS, which omits it.
		return false
	}
}

// boolOption reads a boolean option named key out of opts, defaulting
// to false if opts is nil, key is absent, or its value isn't a bool.
func boolOption(opts map[string]any, key string) bool {
	b, _ := opts[key].(bool)
	return b
}

// requireDescriptionViolations reports a RuleRequireDescription
// violation for every site opts applies to whose description is empty
// once trimmed -- @graphql-eslint trims a description before checking
// it, so a whitespace-only inline description (`"   "`) counts as
// missing.
func requireDescriptionViolations(sites []descriptionSite, opts requireDescriptionOptions) []Violation {
	violations := make([]Violation, 0, len(sites))
	for i := range sites {
		s := &sites[i]
		if !opts.applies(s) || strings.TrimSpace(s.description) != "" {
			continue
		}
		violations = append(violations, Violation{
			err:  gqlerror.ErrorPosf(s.pos, "Description is required for %s", s.nodeName()),
			Rule: config.RuleRequireDescription,
		})
	}
	return violations
}
