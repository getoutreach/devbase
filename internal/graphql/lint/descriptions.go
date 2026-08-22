// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 require-description and description-style rules.

// descriptions.go implements the first 2 (of 10) Tier 3 custom rules:
// require-description (a description is required on the kinds of
// definition its options enable it for) and description-style (every
// description in a schema must use the same quoting style: inline
// "..." or block """...""").
//
// description-style needs to know whether each description was
// written inline or as a block string, but ast.Definition and friends
// only expose the decoded text, not which quoting style produced it.
// This file recovers that by re-lexing each file's own raw source
// (descriptionTokens) and pairing the resulting tokens back up with
// the parsed schema's description-bearing nodes (groupDescriptionSites),
// in the order both appear in that file.
//
// descriptionTokens' scan has to tell a description string apart from
// every other string literal SDL allows: a field/argument default
// value (`= "x"`) and a directive usage argument (`@dir(arg: "x")`),
// including either nested inside a list or input object literal. Both
// are only reachable from a "=" token or a ":" inside a directive
// usage's own argument list, and a description is never preceded by
// either -- so skipValue, which walks past one Value production
// (recursing through matching brackets or braces), can exclude them
// and leave exactly the description candidates.

package lint

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/lexer"
)

// ErrDescriptionTokenMismatch is wrapped by the error tier3Descriptions
// returns if a file's count of non-empty descriptions in the parsed
// schema doesn't match descriptionTokens' count of description-like
// string tokens in that file's raw source -- description-style cannot
// safely pair them up in that case. This should never happen for valid
// SDL; see this file's package doc comment for why descriptionTokens is
// expected to find exactly one token per description.
var ErrDescriptionTokenMismatch = errors.New("description-style: mismatched description token count")

// Option keys read from scripts/devbase.yaml's require-description
// options, matching @graphql-eslint's own option names, and doubling
// as descriptionSite.optionKind tags -- see requireDescriptionOptions.applies.
const (
	optionTypes                = "types"
	optionRootField            = "rootField"
	optionDirectiveDefinition  = "DirectiveDefinition"
	optionFieldDefinition      = "FieldDefinition"
	optionInputValueDefinition = "InputValueDefinition"
	optionEnumValueDefinition  = "EnumValueDefinition"
	optionSchemaDefinition     = "SchemaDefinition"
)

// descriptionSite is one node in a file that can carry a description:
// a type definition, field, argument, enum value, directive
// definition, or schema definition.
type descriptionSite struct {
	// description is the node's decoded description text, or "" if it
	// has none.
	description string

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
func inputValueSite(description string, pos *ast.Position, name, parentLabel string) descriptionSite {
	return descriptionSite{
		description: description, pos: pos,
		optionKind: optionInputValueDefinition, name: name,
		kindLabel: "input value", parentLabel: parentLabel,
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
				add(f.Position.Src, inputValueSite(f.Description, f.Position, f.Name, parentLabel))
			} else {
				add(f.Position.Src, descriptionSite{
					description: f.Description, pos: f.Position,
					optionKind: optionFieldDefinition, name: f.Name,
					kindLabel: "field", parentLabel: parentLabel,
					isRootField: def.Kind == ast.Object && rootTypeNames[def.Name],
				})
			}

			fieldLabel := fmt.Sprintf("field %q", f.Name)
			for _, arg := range f.Arguments {
				add(arg.Position.Src, inputValueSite(arg.Description, arg.Position, arg.Name, fieldLabel))
			}
		}
		for _, ev := range def.EnumValues {
			add(ev.Position.Src, descriptionSite{
				description: ev.Description, pos: ev.Position,
				optionKind: optionEnumValueDefinition, name: ev.Name,
				kindLabel: "enum value", parentLabel: parentLabel,
			})
		}
	}

	hasBaseDefinition := make(map[string]bool, len(doc.Definitions))
	for _, def := range doc.Definitions {
		hasBaseDefinition[def.Name] = true
		add(def.Position.Src, descriptionSite{
			description: def.Description, pos: def.Position,
			optionKind: optionTypes, name: def.Name, kindLabel: typeKindLabel(def.Kind),
			typeDefKindKey: typeDefinitionKindKeys[def.Kind],
		})
		addFields(def)
	}
	for _, def := range doc.Extensions {
		// validator.ValidateSchemaDocument merges an extension's Fields
		// and EnumValues into its base type's own Definition.Fields in
		// place, each keeping its own original Position -- so the
		// doc.Definitions loop above already walked every field an
		// extension of an existing type adds; addFields here would
		// double-count them. Only an extension of a type with no base
		// definition at all has fields that exist nowhere else.
		//
		// An extension can never carry its own description -- gqlparser
		// rejects one the same way it rejects any description before
		// "extend" -- but its fields and enum values can, so those still
		// need walking for the undefined-base case.
		if !hasBaseDefinition[def.Name] {
			addFields(def)
		}
	}

	for _, dd := range doc.Directives {
		add(dd.Position.Src, descriptionSite{
			description: dd.Description, pos: dd.Position,
			optionKind: optionDirectiveDefinition, name: dd.Name, kindLabel: "directive",
		})

		directiveLabel := fmt.Sprintf("directive %q", dd.Name)
		for _, arg := range dd.Arguments {
			add(dd.Position.Src, inputValueSite(arg.Description, arg.Position, arg.Name, directiveLabel))
		}
	}

	for _, sd := range doc.Schema {
		add(sd.Position.Src, descriptionSite{
			description: sd.Description, pos: sd.Position,
			optionKind: optionSchemaDefinition, kindLabel: "schema",
		})
	}

	for _, group := range sites {
		sort.SliceStable(group, func(i, j int) bool { return group[i].pos.Start < group[j].pos.Start })
	}
	return sites
}

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

// descriptionStyleOptions is description-style's scripts/devbase.yaml
// options, decoded from Rule.Options.
type descriptionStyleOptions struct {
	// block is true if every description must be a block string
	// ("""...""") -- @graphql-eslint's default -- or false if every
	// description must be inline ("...").
	block bool
}

// parseDescriptionStyleOptions decodes opts (Rule.Options) into
// descriptionStyleOptions, defaulting to block style if opts is nil or
// its "style" key isn't the string "inline".
func parseDescriptionStyleOptions(opts map[string]any) descriptionStyleOptions {
	style, _ := opts["style"].(string)
	return descriptionStyleOptions{block: style != "inline"}
}

// descriptionStyleViolations reports a RuleDescriptionStyle violation
// for every description in sites whose quoting style -- read off the
// matching entry in tokens, in order -- disagrees with opts. tokens
// must have exactly one entry per site in sites with a non-empty
// description, in the same order; tier3Descriptions guarantees this
// before calling in.
func descriptionStyleViolations(sites []descriptionSite, tokens []lexer.Token, opts descriptionStyleOptions) []Violation {
	violations := make([]Violation, 0, len(tokens))
	i := 0
	for j := range sites {
		s := &sites[j]
		if strings.TrimSpace(s.description) == "" {
			continue
		}
		tok := tokens[i]
		i++

		isBlock := tok.Kind == lexer.BlockString
		if isBlock == opts.block {
			continue
		}

		foundStyle := "inline"
		if isBlock {
			foundStyle = "block"
		}
		violations = append(violations, Violation{
			err:  gqlerror.ErrorPosf(&tok.Pos, "Unexpected %s description for %s", foundStyle, s.nodeName()),
			Rule: config.RuleDescriptionStyle,
		})
	}
	return violations
}

// descriptionTokens scans src's raw source for every String or
// BlockString token that is a description, in file order, excluding
// default values and directive usage arguments -- see this file's
// package doc comment for how it tells them apart.
func descriptionTokens(src *ast.Source) ([]lexer.Token, error) {
	lx := lexer.New(src)

	var all []lexer.Token
	for {
		tok, err := lx.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("lex %s: %w", src.Name, err)
		}
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.Kind == lexer.Comment {
			continue
		}
		all = append(all, tok)
	}

	// directiveArgsActive tracks whether i is inside a directive usage's
	// "(...)" argument list -- entered on a "@Name(" sequence, exited on
	// the next ParenR. No depth counter is needed: a directive usage's
	// arguments are Values, and no Value production contains "("
	// (lists use "[...]", input objects use "{...}"), so no further
	// ParenL can occur before the one ParenR that closes this list.
	var descriptions []lexer.Token
	directiveArgsActive := false

	for i := 0; i < len(all); {
		switch all[i].Kind { //nolint:exhaustive // Why: every other token kind falls through to the default case unchanged.
		case lexer.ParenL:
			directiveArgsActive = i >= 2 && all[i-1].Kind == lexer.Name && all[i-2].Kind == lexer.At
			i++
		case lexer.ParenR:
			directiveArgsActive = false
			i++
		case lexer.Colon:
			if directiveArgsActive {
				i = skipValue(all, i+1)
			} else {
				i++
			}
		case lexer.Equals:
			i = skipValue(all, i+1)
		case lexer.String, lexer.BlockString:
			descriptions = append(descriptions, all[i])
			i++
		default:
			i++
		}
	}

	// gqlparser/v2's lexer sets a BlockString token's Pos.Line/Pos.Column
	// only after scanning to its closing """, so for one spanning more
	// than one line -- the common case for a real description -- they
	// name the closing line, not the opening one, and the column is
	// often negative. Pos.Start (a rune offset) is unaffected, so
	// recompute Line/Column from that instead. This works around a
	// gqlparser/v2 bug verified against v2.5.36 (go.mod's pinned
	// version); drop it once gqlparser fixes BlockString's own position.
	starts := lineStarts(src.Input)
	for i := range descriptions {
		descriptions[i].Pos.Line, descriptions[i].Pos.Column = linePosition(starts, descriptions[i].Pos.Start)
	}
	return descriptions, nil
}

// lineStarts returns the rune offset of the first rune of each line in
// input; line i (1-indexed) starts at starts[i-1]. A line break is a
// "\r", optionally followed by "\n", or a lone "\n" -- the same 3 forms
// gqlparser/v2's own lexer recognizes, so this numbering matches its
// Pos.Line elsewhere. Offsets are in runes, matching ast.Position.Start;
// walking input as UTF-8 directly, rather than converting it to a
// []rune first, avoids copying the whole file.
func lineStarts(input string) []int {
	starts := make([]int, 1, len(input)/40+1)
	runeIdx := 0
	for i := 0; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		i += size
		runeIdx++
		switch r {
		case '\r':
			// '\n' is single-byte ASCII and never a continuation byte of a
			// multi-byte UTF-8 sequence, so it's safe to check input[i]
			// directly without decoding another rune.
			if i < len(input) && input[i] == '\n' {
				i++
				runeIdx++
			}
			starts = append(starts, runeIdx)
		case '\n':
			starts = append(starts, runeIdx)
		}
	}
	return starts
}

// linePosition returns the 1-based (line, column) for the rune offset
// start, given starts (from lineStarts) -- the same Line/Column
// convention gqlparser/v2's own lexer uses everywhere else.
func linePosition(starts []int, start int) (line, col int) {
	i := sort.SearchInts(starts, start+1) - 1
	if i < 0 {
		i = 0
	}
	return i + 1, start - starts[i] + 1
}

// skipValue returns the index just past the one Value production
// (spec grammar) starting at tokens[i]: a scalar/enum-value token
// (i+1), or -- for a list or input object literal -- the index past
// its matching closing bracket or brace, however deeply nested.
func skipValue(tokens []lexer.Token, i int) int {
	if i >= len(tokens) {
		return i
	}
	switch tokens[i].Kind { //nolint:exhaustive // Why: every scalar/enum-value token kind falls through to the default case unchanged.
	case lexer.BracketL:
		return skipBalanced(tokens, i, lexer.BracketL, lexer.BracketR)
	case lexer.BraceL:
		return skipBalanced(tokens, i, lexer.BraceL, lexer.BraceR)
	default:
		return i + 1
	}
}

// skipBalanced returns the index just past the token that closes the
// openTok/closeTok pair starting at tokens[i] (itself an openTok
// token), counting nested pairs of the same kind.
func skipBalanced(tokens []lexer.Token, i int, openTok, closeTok lexer.Type) int {
	depth := 0
	for i < len(tokens) {
		switch tokens[i].Kind { //nolint:exhaustive // Why: every other token kind is part of the value being skipped and needs no handling.
		case openTok:
			depth++
		case closeTok:
			depth--
		}
		i++
		if depth == 0 {
			break
		}
	}
	return i
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
// against every file in fileSources, using parsed (already built by
// Files for Tier 1/Tier 2) to resolve each file's own definitions and
// the schema's root operation type names.
func tier3Descriptions(fileSources []*ast.Source, parsed *parsedSchema, cfg *config.Lint) ([]Violation, error) {
	requireEnabled := cfg.Enabled(config.RuleRequireDescription)
	styleEnabled := cfg.Enabled(config.RuleDescriptionStyle)
	if !requireEnabled && !styleEnabled {
		return nil, nil
	}

	requireOpts := parseRequireDescriptionOptions(cfg.Options(config.RuleRequireDescription))
	styleOpts := parseDescriptionStyleOptions(cfg.Options(config.RuleDescriptionStyle))
	roots := rootTypeNameSet(parsed.schema)
	sitesByFile := groupDescriptionSites(parsed.doc, roots)

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
