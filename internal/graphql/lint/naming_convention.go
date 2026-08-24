// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 naming-convention rule.

// naming_convention.go implements 1 (of 10) Tier 3 custom rules:
// naming-convention, which requires names to follow the casing,
// prefix/suffix, and underscore conventions its options configure per
// kind of definition, matching @graphql-eslint's rule of the same
// name.
//
// @graphql-eslint's own naming-convention accepts an option key for
// any AST node kind, including operation-only kinds (OperationDefinition,
// FragmentDefinition, VariableDefinition) that never appear in a
// standalone schema file, and lets a key carry an arbitrary selector
// (for example "FieldDefinition[parent.name.value!=Query]" -- see
// https://eslint.org/docs/developer-guide/selectors). devbase graphql
// lint only ever parses schema files (see lint.go's Files), so this
// port narrows the recognized keys to the schema-only kinds: the 6
// type-definition kinds (via the shared "types" option and its
// per-kind overrides), FieldDefinition, InputValueDefinition (covers a
// field's or directive definition's own argument, and an input object
// field -- graphql-js represents all three as the one AST kind),
// EnumValueDefinition, and DirectiveDefinition. It also recognizes the
// 3 root-operation-type field selectors from @graphql-eslint's own
// documented "recommended schema" config
// (FieldDefinition[parent.name.value=Query|Mutation|Subscription]), so
// a repository can forbid a "get" prefix on Query fields specifically
// without forbidding it everywhere. A directive usage's own arguments
// (Kind.ARGUMENT, for example "reason" in `@deprecated(reason:
// "...")`) are out of scope: no repository's config exercises them
// today.

package lint

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Selector keys naming-convention recognizes beyond the 6 type-kind
// keys in typeDefinitionKindKeys (descriptions.go), matching
// @graphql-eslint's own option keys for the AST kinds that appear in a
// schema file.
const (
	// fieldDefinitionSelector is an object or interface field.
	fieldDefinitionSelector = "FieldDefinition"

	// inputValueDefinitionSelector is a field's or directive
	// definition's own argument, or an input object field.
	inputValueDefinitionSelector = "InputValueDefinition"

	// directiveDefinitionSelector is a directive definition.
	directiveDefinitionSelector = "DirectiveDefinition"

	// enumValueDefinitionSelector is an enum value.
	enumValueDefinitionSelector = "EnumValueDefinition"
)

// rootFieldSelector returns the "FieldDefinition[parent.name.value=...]"
// selector key for a field declared directly on role ("Query",
// "Mutation", or "Subscription"), matching @graphql-eslint's own
// documented "recommended schema" config.
func rootFieldSelector(role string) string {
	return fmt.Sprintf("%s[parent.name.value=%s]", fieldDefinitionSelector, role)
}

// namingStyleRegexes classifies a name (with any leading/trailing
// underscores already stripped) into one of @graphql-eslint's 4
// allowed casing styles, using the exact patterns from its own
// StyleToRegex.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var namingStyleRegexes = map[string]*regexp.Regexp{
	"camelCase":  regexp.MustCompile(`^[a-z][\dA-Za-z]*$`),
	"PascalCase": regexp.MustCompile(`^[A-Z][\dA-Za-z]*$`),
	"snake_case": regexp.MustCompile(`^[a-z][\d_a-z]*[\da-z]*$`),
	"UPPER_CASE": regexp.MustCompile(`^[A-Z][\dA-Z_]*[\dA-Z]*$`),
}

// namingTypeKindDisplay names a type-definition kind's own display
// label in a naming-convention message (for example `Type "b" should
// ...`), matching @graphql-eslint's own KindToDisplayName. This is
// deliberately not typeKindLabel (descriptions.go): that one is
// lowercase for a different message shape (`field "foo" in type
// "Bar"`), not naming-convention's own capitalized, parent-less one.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var namingTypeKindDisplay = map[ast.DefinitionKind]string{
	ast.Scalar:      "Scalar",
	ast.Object:      "Type",
	ast.Interface:   "Interface",
	ast.Union:       "Union",
	ast.Enum:        "Enumerator",
	ast.InputObject: "Input type",
}

// namingRule is one selector's casing/prefix/suffix requirement,
// decoded from either the short form (a bare style string) or the
// long form (an options object) of a naming-convention selector
// value, matching @graphql-eslint's own PropertySchema.
type namingRule struct {
	style             string
	prefix            string
	suffix            string
	forbiddenPrefixes []string
	forbiddenSuffixes []string

	// ignoreRegex is ignorePattern, already compiled once here rather
	// than by every namingRuleMessage call the rule applies to, or nil
	// if ignorePattern was unset or failed to compile.
	ignoreRegex *regexp.Regexp
}

// parseNamingRule decodes v -- a selector's value in
// scripts/devbase.yaml -- into a namingRule. v is either a bare style
// string ("PascalCase") or an options object; any other shape is
// reported as not ok, so the caller can leave the selector
// unconfigured rather than panic on a malformed config.
func parseNamingRule(v any) (namingRule, bool) {
	if style, ok := v.(string); ok {
		return namingRule{style: style}, true
	}
	opts, ok := v.(map[string]any)
	if !ok {
		return namingRule{}, false
	}
	rule := namingRule{
		style:             stringOption(opts, "style"),
		prefix:            stringOption(opts, "prefix"),
		suffix:            stringOption(opts, "suffix"),
		forbiddenPrefixes: stringSliceOption(opts, "forbiddenPrefixes"),
		forbiddenSuffixes: stringSliceOption(opts, "forbiddenSuffixes"),
	}
	if pattern := stringOption(opts, "ignorePattern"); pattern != "" {
		if re, err := regexp.Compile(pattern); err == nil {
			rule.ignoreRegex = re
		}
	}
	return rule, true
}

// stringOption reads a string option named key out of opts, defaulting
// to "" if opts is nil, key is absent, or its value isn't a string.
func stringOption(opts map[string]any, key string) string {
	s, _ := opts[key].(string)
	return s
}

// stringSliceOption reads a string-list option named key out of opts,
// skipping any element that isn't itself a string. It returns nil if
// opts is nil, key is absent, or its value isn't a list.
func stringSliceOption(opts map[string]any, key string) []string {
	raw, ok := opts[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// namingConventionOptions is naming-convention's scripts/devbase.yaml
// options, decoded from RuleConfig.Options.
type namingConventionOptions struct {
	// allowLeadingUnderscore and allowTrailingUnderscore, if false
	// (the default, matching @graphql-eslint's own default), make
	// every name's leading/trailing underscores their own violation,
	// independent of and in addition to whatever selectors below
	// apply to that name.
	allowLeadingUnderscore  bool
	allowTrailingUnderscore bool

	// types is the fallback namingRule for a type-definition site
	// whose own kind key (for example "ObjectTypeDefinition") has no
	// selector entry of its own, or nil if the "types" option was not
	// set.
	types *namingRule

	// selectors holds every other selector key present in
	// scripts/devbase.yaml as-is (for example "FieldDefinition" or
	// "FieldDefinition[parent.name.value=Query]"), decoded to a
	// namingRule. A key this package does not recognize (an
	// operation-only kind, or an arbitrary selector no repository's
	// config uses today -- see this file's package doc comment) is
	// carried here too but never matched against any namingSite, so
	// it is simply inert rather than rejected.
	selectors map[string]namingRule
}

// parseNamingConventionOptions decodes opts (RuleConfig.Options) into
// namingConventionOptions.
func parseNamingConventionOptions(opts map[string]any) namingConventionOptions {
	o := namingConventionOptions{
		allowLeadingUnderscore:  boolOption(opts, "allowLeadingUnderscore"),
		allowTrailingUnderscore: boolOption(opts, "allowTrailingUnderscore"),
	}
	for key, v := range opts {
		if key == "allowLeadingUnderscore" || key == "allowTrailingUnderscore" {
			continue
		}
		rule, ok := parseNamingRule(v)
		if !ok {
			continue
		}
		if key == "types" {
			o.types = &rule
			continue
		}
		if o.selectors == nil {
			o.selectors = make(map[string]namingRule)
		}
		o.selectors[key] = rule
	}
	return o
}

// ruleFor resolves the namingRule that applies to s, if any: s's own
// root-field selector (if it has one) or base selector, whichever
// scripts/devbase.yaml actually configured, falling back to the
// shared "types" rule only for a type-definition site whose own kind
// key was not configured individually.
func (o namingConventionOptions) ruleFor(s *namingSite) (namingRule, bool) {
	if s.rootSelector != "" {
		if r, ok := o.selectors[s.rootSelector]; ok {
			return r, true
		}
	}
	if r, ok := o.selectors[s.baseSelector]; ok {
		return r, true
	}
	if s.typeDefKindKey != "" && o.types != nil {
		return *o.types, true
	}
	return namingRule{}, false
}

// namingSite is one name in a schema file that naming-convention
// checks: a type definition, field, input value (an argument or an
// input object field), enum value, or directive definition.
type namingSite struct {
	pos            *ast.Position
	name           string
	kindLabel      string
	baseSelector   string
	rootSelector   string
	typeDefKindKey string
}

// rootTypeRoles maps each of schema's root operation type names (Query,
// Mutation, and/or Subscription -- whatever schema actually declares)
// to its role, for rootFieldSelector.
func rootTypeRoles(schema *ast.Schema) map[string]string {
	roles := make(map[string]string, 3)
	if schema.Query != nil {
		roles[schema.Query.Name] = "Query"
	}
	if schema.Mutation != nil {
		roles[schema.Mutation.Name] = "Mutation"
	}
	if schema.Subscription != nil {
		roles[schema.Subscription.Name] = "Subscription"
	}
	return roles
}

// namingSites collects every namingSite in doc that was written in one
// of inScope's sources -- excluding gqlparser's built-in prelude and
// any federation- or scalars-synthesized prelude (see federation.go) --
// in document order. Like typenamePrefixViolations, an extension's
// fields are only walked on their own when its base type has no
// definition anywhere; otherwise validator.ValidateSchemaDocument has
// already merged them into the base Definition.Fields that the
// doc.Definitions walk already covers (see descriptions.go's
// forEachDefinition).
func namingSites(doc *ast.SchemaDocument, roles map[string]string, inScope set.Set[*ast.Source]) []namingSite {
	var sites []namingSite

	addIfInScope := func(site namingSite) {
		if site.pos == nil || !inScope.Contains(site.pos.Src) {
			return
		}
		sites = append(sites, site)
	}

	addFields := func(def *ast.Definition) {
		rootSelector := ""
		if role := roles[def.Name]; role != "" {
			rootSelector = rootFieldSelector(role)
		}

		for _, f := range def.Fields {
			if f.Position == nil {
				// __schema/__type introspection meta-fields; see
				// descriptions.go.
				continue
			}
			if def.Kind == ast.InputObject {
				addIfInScope(namingSite{pos: f.Position, name: f.Name, kindLabel: "Input property", baseSelector: inputValueDefinitionSelector})
			} else {
				addIfInScope(namingSite{
					pos: f.Position, name: f.Name, kindLabel: "Field",
					baseSelector: fieldDefinitionSelector, rootSelector: rootSelector,
				})
			}
			for _, arg := range f.Arguments {
				addIfInScope(namingSite{pos: arg.Position, name: arg.Name, kindLabel: "Input property", baseSelector: inputValueDefinitionSelector})
			}
		}
		for _, ev := range def.EnumValues {
			addIfInScope(namingSite{pos: ev.Position, name: ev.Name, kindLabel: "Enumeration value", baseSelector: enumValueDefinitionSelector})
		}
	}

	forEachDefinition(doc, func(def *ast.Definition) {
		if kindKey, ok := typeDefinitionKindKeys[def.Kind]; ok {
			addIfInScope(namingSite{
				pos: def.Position, name: def.Name, kindLabel: namingTypeKindDisplay[def.Kind],
				baseSelector: kindKey, typeDefKindKey: kindKey,
			})
		}
		addFields(def)
	}, addFields)

	for _, dd := range doc.Directives {
		addIfInScope(namingSite{pos: dd.Position, name: dd.Name, kindLabel: "Directive", baseSelector: directiveDefinitionSelector})
		for _, arg := range dd.Arguments {
			addIfInScope(namingSite{pos: arg.Position, name: arg.Name, kindLabel: "Input property", baseSelector: inputValueDefinitionSelector})
		}
	}

	return sites
}

// namingRuleMessage reports the violation message naming-convention
// should raise for name under rule, or "" if name satisfies rule. It
// checks, in order, exactly like @graphql-eslint's own getError: a
// required prefix, a required suffix, every forbidden prefix, every
// forbidden suffix, then the casing style -- stopping at the first
// that applies, against name with any leading/trailing underscores
// stripped (regardless of whether the underscore checks themselves are
// enabled). ignorePattern, if it matches, skips all of the above.
func namingRuleMessage(name string, rule *namingRule) string {
	trimmed := strings.Trim(name, "_")

	if rule.ignoreRegex != nil && rule.ignoreRegex.MatchString(trimmed) {
		return ""
	}
	if rule.prefix != "" && !strings.HasPrefix(trimmed, rule.prefix) {
		return fmt.Sprintf("should have %q prefix", rule.prefix)
	}
	if rule.suffix != "" && !strings.HasSuffix(trimmed, rule.suffix) {
		return fmt.Sprintf("should have %q suffix", rule.suffix)
	}
	for _, p := range rule.forbiddenPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return fmt.Sprintf("should not have %q prefix", p)
		}
	}
	for _, s := range rule.forbiddenSuffixes {
		if strings.HasSuffix(trimmed, s) {
			return fmt.Sprintf("should not have %q suffix", s)
		}
	}
	if rule.style == "" {
		return ""
	}
	if re, ok := namingStyleRegexes[rule.style]; ok && !re.MatchString(trimmed) {
		return fmt.Sprintf("should be in %s format", rule.style)
	}
	return ""
}

// namingConventionViolations reports a RuleNamingConvention violation
// for every site in sites that either has a configured namingRule it
// fails (per namingRuleMessage) or an underscore opts forbids, in
// document order. Both checks run for the same site independently --
// neither one skips the other -- matching @graphql-eslint's own
// behavior of registering them as separate listeners.
func namingConventionViolations(sites []namingSite, opts namingConventionOptions) []Violation {
	var violations []Violation
	for _, s := range sites {
		if rule, ok := opts.ruleFor(&s); ok {
			if msg := namingRuleMessage(s.name, &rule); msg != "" {
				violations = append(violations, Violation{
					err:  gqlerror.ErrorPosf(s.pos, "%s %q %s", s.kindLabel, s.name, msg),
					Rule: config.RuleNamingConvention,
				})
			}
		}
		if !opts.allowLeadingUnderscore && strings.HasPrefix(s.name, "_") {
			violations = append(violations, Violation{
				err:  gqlerror.ErrorPosf(s.pos, "Leading underscores are not allowed"),
				Rule: config.RuleNamingConvention,
			})
		}
		if !opts.allowTrailingUnderscore && strings.HasSuffix(s.name, "_") {
			violations = append(violations, Violation{
				err:  gqlerror.ErrorPosf(s.pos, "Trailing underscores are not allowed"),
				Rule: config.RuleNamingConvention,
			})
		}
	}
	return violations
}

// tier3NamingConvention runs naming-convention against parsed. It does
// not run unless cfg enables it (config.Lint.Enabled).
func tier3NamingConvention(parsed *parsedSchema, inScope set.Set[*ast.Source], cfg *config.Lint) []Violation {
	if !cfg.Enabled(config.RuleNamingConvention) {
		return nil
	}
	sites := namingSites(parsed.doc, rootTypeRoles(parsed.schema), inScope)
	opts := parseNamingConventionOptions(cfg.Options(config.RuleNamingConvention))
	return namingConventionViolations(sites, opts)
}
