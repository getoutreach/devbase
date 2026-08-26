// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared descriptionSite/groupDescriptionSites machinery
// for the Tier 3 require-description and description-style rules.

// descriptions.go holds what require_description.go and
// description_style.go (the first 2, of 10, Tier 3 custom rules) both
// need: descriptionSite, one node in a file that can carry a
// description, and groupDescriptionSites, which walks parsed.doc once
// and buckets every such node by the file it was written in.
// tier3Descriptions ties the two rules together, running whichever of
// them scripts/devbase.yaml enables against every file. deprecation.go
// and hashtag.go reuse the same descriptionSite/groupDescriptionSites
// walk for require-deprecation-reason, require-deprecation-date, and
// no-hashtag-description.

package lint

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
)

// ErrDescriptionTokenMismatch is wrapped by the error tier3Descriptions
// returns if a file's count of non-empty descriptions in the parsed
// schema doesn't match descriptionTokens' count of description-like
// string tokens in that file's raw source -- description-style cannot
// safely pair them up in that case. This should never happen for valid
// SDL; see description_style.go's package doc comment for why
// descriptionTokens is expected to find exactly one token per
// description.
var ErrDescriptionTokenMismatch = errors.New("description-style: mismatched description token count")

// Option keys read from scripts/devbase.yaml's require-description
// options, matching @graphql-eslint's own option names, and doubling
// as descriptionSite.optionKind tags -- see requireDescriptionOptions.applies.
const (
	// optionTypes is require-description's "types" option key, and the
	// descriptionSite.optionKind for a type/interface/union/enum/scalar/
	// input object definition.
	optionTypes = "types"

	// optionRootField is require-description's "rootField" option key.
	// It never appears as a descriptionSite.optionKind; instead it
	// gates optionFieldDefinition sites whose isRootField is true.
	optionRootField = "rootField"

	// optionDirectiveDefinition is require-description's
	// "DirectiveDefinition" option key, and the descriptionSite.optionKind
	// for a directive definition.
	optionDirectiveDefinition = "DirectiveDefinition"

	// optionFieldDefinition is require-description's "FieldDefinition"
	// option key, and the descriptionSite.optionKind for an object or
	// interface type's own field.
	optionFieldDefinition = "FieldDefinition"

	// optionInputValueDefinition is require-description's
	// "InputValueDefinition" option key, and the descriptionSite.optionKind
	// for an argument or an input object type's own field.
	optionInputValueDefinition = "InputValueDefinition"

	// optionEnumValueDefinition is require-description's
	// "EnumValueDefinition" option key, and the descriptionSite.optionKind
	// for an enum value.
	optionEnumValueDefinition = "EnumValueDefinition"

	// optionSchemaDefinition is the descriptionSite.optionKind for a
	// schema definition. It is not one of require-description's option
	// keys -- see requireDescriptionOptions.applies.
	optionSchemaDefinition = "SchemaDefinition"
)

// descriptionSite is one node in a file that can carry a description:
// a type definition, field, argument, enum value, directive
// definition, or schema definition. This file uses it for
// require-description and description-style; deprecation.go and
// hashtag.go reuse the same walk (groupDescriptionSites) for
// require-deprecation-reason, require-deprecation-date, and
// no-hashtag-description, via the directives and
// afterDescriptionComment fields below.
type descriptionSite struct {
	// description is the node's decoded description text, or "" if it
	// has none.
	description string

	// directives is the node's own applied directives, or nil for a
	// kind that cannot carry any (a type, directive, or schema
	// definition). deprecation.go's only use for this field is finding
	// an applied "deprecated" usage, and gqlparser rejects @deprecated
	// on those kinds before Tier 3 ever runs (see Files' doc comment),
	// so they are never populated.
	directives ast.DirectiveList

	// afterDescriptionComment is the "#" comment(s) gqlparser recorded
	// immediately before this site, after its description if it has
	// one, or nil if there were none. hashtag.go is this field's only
	// reader.
	afterDescriptionComment *ast.CommentGroup

	// pos anchors the require-description violation location, and the
	// order groupDescriptionSites sorts a file's sites into for pairing
	// with descriptionTokens. It is always the node's own Name
	// position, set regardless of whether a description preceded it.
	pos *ast.Position

	// optionKind is which require-description option this site's kind
	// maps to -- see requireDescriptionOptions.applies.
	optionKind string

	// isRootField is true for a FieldDefinition site whose parent
	// object type is one of the schema's root operation types.
	isRootField bool

	// name is the node's own name, or "" for a schema definition.
	name string

	// typeDefKindKey is set only when optionKind is optionTypes, to one
	// of typeDefinitionKindKeys -- the individual per-kind option key
	// (for example "ObjectTypeDefinition") that can override the
	// blanket "types" option for this site's specific kind.
	typeDefKindKey string

	// kindLabel is the node's kind, formatted the way
	// @graphql-eslint's displayNodeName renders it (for example
	// "type", "field", "input value").
	kindLabel string

	// parentLabel is the enclosing node's own formatted name (for
	// example `type "Foo"`), or "" for a site with no parent (a type
	// definition, directive definition, or schema definition).
	parentLabel string
}

// nodeName formats s the way @graphql-eslint's getNodeName does, for
// use in a rule's violation message: `<kind> "<name>"`, followed by
// ` in <parentLabel>` if s has a parent.
func (s *descriptionSite) nodeName() string {
	name := s.kindLabel
	if s.name != "" {
		name = fmt.Sprintf("%s %q", s.kindLabel, s.name)
	}
	if s.parentLabel != "" {
		name += " in " + s.parentLabel
	}
	return name
}

// typeKindLabel formats a type definition's DefinitionKind the way
// @graphql-eslint's DisplayNodeNameMap does.
func typeKindLabel(kind ast.DefinitionKind) string {
	switch kind {
	case ast.Scalar:
		return "scalar"
	case ast.Object:
		return "type"
	case ast.Interface:
		return "interface"
	case ast.Union:
		return "union"
	case ast.Enum:
		return "enum"
	case ast.InputObject:
		return "input"
	default:
		return string(kind)
	}
}

// typeDefinitionKindKeys are require-description's per-kind option
// keys for the 6 kinds its blanket "types" option covers -- named to
// match @graphql-eslint's own GraphQL AST Kind constants, since that's
// what scripts/devbase.yaml authors coming from @graphql-eslint's
// config already know them as.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var typeDefinitionKindKeys = map[ast.DefinitionKind]string{
	ast.Scalar:      "ScalarTypeDefinition",
	ast.Object:      "ObjectTypeDefinition",
	ast.Interface:   "InterfaceTypeDefinition",
	ast.Union:       "UnionTypeDefinition",
	ast.Enum:        "EnumTypeDefinition",
	ast.InputObject: "InputObjectTypeDefinition",
}

// inputValueSite builds the descriptionSite for an argument or an
// input object field -- the 3 constructs graphql-js represents as an
// InputValueDefinition node, all labeled and bucketed identically.
func inputValueSite(description string, pos *ast.Position, name, parentLabel string,
	dirs ast.DirectiveList, comment *ast.CommentGroup,
) descriptionSite {
	return descriptionSite{
		description: description, pos: pos,
		optionKind: optionInputValueDefinition, name: name,
		kindLabel: "input value", parentLabel: parentLabel,
		directives: dirs, afterDescriptionComment: comment,
	}
}

// forEachDefinition calls onDefinition for every base definition in
// doc.Definitions, and onExtension for every extension in
// doc.Extensions with no base definition. A based extension is skipped:
// validator.ValidateSchemaDocument already merged its Fields and
// EnumValues into the base Definition in place, so onDefinition already
// covers it. groupDescriptionSites passes two different functions here
// (an extension can never carry its own description); typenamePrefixViolations
// and namingSites pass the same one for both.
func forEachDefinition(doc *ast.SchemaDocument, onDefinition, onExtension func(*ast.Definition)) {
	seen := make(set.Set[string], len(doc.Definitions))
	for _, def := range doc.Definitions {
		seen.Insert(def.Name)
		onDefinition(def)
	}
	for _, def := range doc.Extensions {
		if !seen.Contains(def.Name) {
			onExtension(def)
		}
	}
}

// groupDescriptionSites collects every descriptionSite in doc, grouped
// by the *ast.Source it was written in (each site's own Position.Src),
// with each group sorted by pos.Start -- the same file order
// descriptionTokens produces, which tier3Descriptions relies on to
// pair the two up.
func groupDescriptionSites(doc *ast.SchemaDocument, rootTypeNames map[string]bool) map[*ast.Source][]descriptionSite {
	sites := make(map[*ast.Source][]descriptionSite)
	add := func(src *ast.Source, s descriptionSite) {
		sites[src] = append(sites[src], s)
	}

	addFields := func(def *ast.Definition) {
		// Each field/argument/enum value is bucketed by its own
		// Position.Src, not def.Position.Src -- see the doc.Extensions
		// loop below for why they can differ.
		parentLabel := fmt.Sprintf("%s %q", typeKindLabel(def.Kind), def.Name)
		for _, f := range def.Fields {
			if f.Position == nil {
				// validator.ValidateSchemaDocument injects the __schema and
				// __type introspection meta-fields into the Query root
				// type's Fields, with no Position of their own since they
				// were never written in any file's SDL -- skip them,
				// there's nothing to check.
				continue
			}

			if def.Kind == ast.InputObject {
				add(f.Position.Src, inputValueSite(f.Description, f.Position, f.Name, parentLabel,
					f.Directives, f.AfterDescriptionComment))
			} else {
				add(f.Position.Src, descriptionSite{
					description: f.Description, pos: f.Position,
					optionKind: optionFieldDefinition, name: f.Name,
					kindLabel: "field", parentLabel: parentLabel,
					isRootField: def.Kind == ast.Object && rootTypeNames[def.Name],
					directives:  f.Directives, afterDescriptionComment: f.AfterDescriptionComment,
				})
			}

			fieldLabel := fmt.Sprintf("field %q", f.Name)
			for _, arg := range f.Arguments {
				add(arg.Position.Src, inputValueSite(arg.Description, arg.Position, arg.Name, fieldLabel,
					arg.Directives, arg.AfterDescriptionComment))
			}
		}
		for _, ev := range def.EnumValues {
			add(ev.Position.Src, descriptionSite{
				description: ev.Description, pos: ev.Position,
				optionKind: optionEnumValueDefinition, name: ev.Name,
				kindLabel: "enum value", parentLabel: parentLabel,
				directives: ev.Directives, afterDescriptionComment: ev.AfterDescriptionComment,
			})
		}
	}

	forEachDefinition(doc, func(def *ast.Definition) {
		add(def.Position.Src, descriptionSite{
			description: def.Description, pos: def.Position,
			optionKind: optionTypes, name: def.Name, kindLabel: typeKindLabel(def.Kind),
			typeDefKindKey:          typeDefinitionKindKeys[def.Kind],
			afterDescriptionComment: def.AfterDescriptionComment,
		})
		addFields(def)
	}, addFields)

	for _, dd := range doc.Directives {
		add(dd.Position.Src, descriptionSite{
			description: dd.Description, pos: dd.Position,
			optionKind: optionDirectiveDefinition, name: dd.Name, kindLabel: "directive",
			afterDescriptionComment: dd.AfterDescriptionComment,
		})

		directiveLabel := fmt.Sprintf("directive %q", dd.Name)
		for _, arg := range dd.Arguments {
			add(dd.Position.Src, inputValueSite(arg.Description, arg.Position, arg.Name, directiveLabel,
				arg.Directives, arg.AfterDescriptionComment))
		}
	}

	for _, sd := range doc.Schema {
		add(sd.Position.Src, descriptionSite{
			description: sd.Description, pos: sd.Position,
			optionKind: optionSchemaDefinition, kindLabel: "schema",
			afterDescriptionComment: sd.AfterDescriptionComment,
		})
	}

	for _, group := range sites {
		sort.SliceStable(group, func(i, j int) bool { return group[i].pos.Start < group[j].pos.Start })
	}
	return sites
}

// rootTypeNameSet returns the names of schema's root operation types
// (Query, Mutation, and/or Subscription -- whatever schema actually
// declares, whether by convention or an explicit `schema { ... }`
// block), for requireDescriptionOptions' rootField option.
func rootTypeNameSet(schema *ast.Schema) map[string]bool {
	names := make(map[string]bool, 3)
	for _, def := range []*ast.Definition{schema.Query, schema.Mutation, schema.Subscription} {
		if def != nil {
			names[def.Name] = true
		}
	}
	return names
}

// tier3Descriptions runs require-description and description-style --
// neither runs unless cfg enables it (config.Lint.Enabled) --
// against every file in fileSources, using sitesByFile (built once by
// tier3's shared groupDescriptionSites call) to resolve each file's own
// description-bearing sites.
func tier3Descriptions(fileSources []*ast.Source, sitesByFile map[*ast.Source][]descriptionSite,
	cfg *config.Lint,
) ([]Violation, error) {
	requireEnabled := cfg.Enabled(config.RuleRequireDescription)
	styleEnabled := cfg.Enabled(config.RuleDescriptionStyle)
	if !requireEnabled && !styleEnabled {
		return nil, nil
	}

	requireOpts := parseRequireDescriptionOptions(cfg.Options(config.RuleRequireDescription))
	styleOpts := parseDescriptionStyleOptions(cfg.Options(config.RuleDescriptionStyle))

	var violations []Violation
	for _, src := range fileSources {
		sites := sitesByFile[src]

		if requireEnabled {
			violations = append(violations, requireDescriptionViolations(sites, requireOpts)...)
		}

		if styleEnabled {
			tokens, err := descriptionTokens(src)
			if err != nil {
				return nil, err
			}

			nonEmpty := 0
			for _, s := range sites {
				if strings.TrimSpace(s.description) != "" {
					nonEmpty++
				}
			}
			if nonEmpty != len(tokens) {
				return nil, fmt.Errorf("%s: %d description(s) in the parsed schema, %d description-like string token(s) "+
					"in the raw source: %w", src.Name, nonEmpty, len(tokens), ErrDescriptionTokenMismatch)
			}

			violations = append(violations, descriptionStyleViolations(sites, tokens, styleOpts)...)
		}
	}
	return violations, nil
}
