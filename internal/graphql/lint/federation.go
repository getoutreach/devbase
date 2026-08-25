// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Synthesizes the SDL prelude Files merges in when
// scripts/devbase.yaml sets graphql.lint.federation or
// graphql.lint.scalars.

// federation.go resolves a repository's own `extend schema
// @link(url: ..., import: [...])` directive, the mechanism Apollo
// Federation subgraphs use to bring spec directives like @key and
// @shareable into scope. When scripts/devbase.yaml opts in via
// graphql.lint.federation, it synthesizes SDL definitions for the
// directives that @link actually imports, honoring an `as` rename.
// gqlparser has no built-in notion of any of this: @link and
// everything it imports are injected by federation composition
// tooling, never declared via `directive @...` SDL in a subgraph's
// own files.
//
// The synthesized prelude defines only what a schema's own @link
// imports, so gqlparser's existing known-directives check still fails
// a directive that is used but never imported, with the same
// "Undefined directive" error that Files classifies as
// RuleKnownDirectives for any other undeclared directive. No separate
// import-list validation is needed.
//
// Directive signatures below come from the Apollo Federation subgraph
// specification (github.com/apollographql/federation,
// docs/source/schema-design/federated-schemas/reference/subgraph-spec.mdx,
// fetched 2026-08-21). Only the 9 directives named in
// federationDirectives are supported; an import naming any other
// directive, for example @interfaceObject or @authenticated added in
// later spec versions, is reported by name instead of assuming an
// unverified signature.

package lint

import (
	"errors"
	"fmt"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
)

// ErrUnsupportedFederationVersion is wrapped by the error returned
// when scripts/devbase.yaml's graphql.lint.federation names a version
// this package does not ship a directive prelude for.
var ErrUnsupportedFederationVersion = errors.New("unsupported federation version")

// ErrFederationVersionMismatch is wrapped by the error returned when a
// repository's own `@link` names a Federation version other than the
// one scripts/devbase.yaml configures.
var ErrFederationVersionMismatch = errors.New("federation version mismatch")

// ErrUnsupportedFederationDirective is wrapped by the error returned
// when a repository's `@link` import list names something this
// package cannot classify as one of the directives in
// federationDirectives.
var ErrUnsupportedFederationDirective = errors.New("unsupported federation directive import")

// federationLinkURLPrefix identifies a `@link` as linking against the
// Apollo Federation subgraph spec itself. A schema may separately
// `@link` to some other spec; this package leaves those untouched.
const federationLinkURLPrefix = "https://specs.apollo.dev/federation/"

// supportedFederationVersions are the Federation subgraph spec
// versions this package ships a directive prelude for. A
// graphql.lint.federation value outside this set is rejected with
// ErrUnsupportedFederationVersion rather than guessed at.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated; Go has no map consts.
var supportedFederationVersions = map[string]bool{
	"v2.3": true,
}

// federationDirective describes one Federation subgraph directive
// this package knows how to synthesize. sdl is its SDL definition,
// with "%[1]s" standing in for the directive's name so an `as` rename
// can be honored. needsFieldSet marks whether that SDL references the
// FieldSet scalar, so the prelude declares FieldSet only when an
// imported directive needs it.
type federationDirective struct {
	sdl           string
	needsFieldSet bool
}

// federationDirectives are the Federation subgraph directives this
// package knows how to synthesize, keyed by name.
//
//nolint:gochecknoglobals,lll // Why: a fixed lookup table, never mutated; Go has no map consts, and directive location lists can't be wrapped.
var federationDirectives = map[string]federationDirective{
	"key":          {sdl: "directive @%[1]s(fields: FieldSet!, resolvable: Boolean = true) repeatable on OBJECT | INTERFACE", needsFieldSet: true},
	"shareable":    {sdl: "directive @%[1]s repeatable on OBJECT | FIELD_DEFINITION"},
	"override":     {sdl: "directive @%[1]s(from: String!) on FIELD_DEFINITION"},
	"inaccessible": {sdl: "directive @%[1]s on FIELD_DEFINITION | OBJECT | INTERFACE | UNION | ARGUMENT_DEFINITION | SCALAR | ENUM | ENUM_VALUE | INPUT_OBJECT | INPUT_FIELD_DEFINITION"},
	"external":     {sdl: "directive @%[1]s on FIELD_DEFINITION | OBJECT"},
	"requires":     {sdl: "directive @%[1]s(fields: FieldSet!) on FIELD_DEFINITION", needsFieldSet: true},
	"provides":     {sdl: "directive @%[1]s(fields: FieldSet!) on FIELD_DEFINITION", needsFieldSet: true},
	"tag":          {sdl: "directive @%[1]s(name: String!) repeatable on FIELD_DEFINITION | INTERFACE | OBJECT | UNION | ARGUMENT_DEFINITION | SCALAR | ENUM | ENUM_VALUE | INPUT_OBJECT | INPUT_FIELD_DEFINITION"},
	"extends":      {sdl: "directive @%[1]s on OBJECT | INTERFACE"},
}

// federationImport is a single entry from a `@link` import list,
// resolved to the directive name it refers to and the name it is
// imported as (equal to name unless the entry renamed it with `as`).
type federationImport struct {
	name  string
	alias string
}

// preludeSources returns the extra sources Files should merge into
// sources for cfg's federation and scalars settings, or nil if cfg is
// nil or sets neither.
func preludeSources(sources []*ast.Source, cfg *config.Lint) ([]*ast.Source, error) {
	if cfg == nil {
		return nil, nil
	}

	var extra []*ast.Source
	if cfg.Federation != "" {
		prelude, err := federationPrelude(sources, cfg.Federation)
		if err != nil {
			return nil, err
		}
		if prelude != "" {
			extra = append(extra, &ast.Source{Name: "<federation prelude>", Input: prelude})
		}
	}
	if len(cfg.Scalars) > 0 {
		extra = append(extra, &ast.Source{Name: "<scalars prelude>", Input: scalarsPrelude(cfg.Scalars)})
	}
	return extra, nil
}

// scalarsPrelude renders names as one `scalar X` declaration per line.
func scalarsPrelude(names []string) string {
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "scalar %s\n", name)
	}
	return b.String()
}

// federationPrelude parses sources' raw `extend schema @link(...)`
// directives and returns the SDL Files should merge in for
// wantVersion. parser.ParseSchemas performs syntax parsing only, so
// this works even though @link and anything it imports are not yet
// declared. It returns "", nil if sources contain no `@link` to the
// Federation subgraph spec: scripts/devbase.yaml opted in, but
// nothing in this schema uses it yet.
func federationPrelude(sources []*ast.Source, wantVersion string) (string, error) {
	if !supportedFederationVersions[wantVersion] {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFederationVersion, wantVersion)
	}

	doc, err := parser.ParseSchemas(sources...)
	if err != nil {
		return "", fmt.Errorf("parse schema for federation @link directives: %w", err)
	}

	needsFieldSet := false
	var directiveDefs []string
	seenLink := false

	for _, sd := range doc.SchemaExtension {
		for _, dir := range sd.Directives.ForNames("link") {
			gotVersion, err := federationLinkVersion(dir)
			if err != nil {
				return "", err
			}
			if gotVersion == "" {
				continue // dir links to some other, unrelated spec.
			}
			if gotVersion != wantVersion {
				return "", federationErrorf(ErrFederationVersionMismatch, dir.Position,
					"scripts/devbase.yaml sets federation: %s, but this links federation %s", wantVersion, gotVersion)
			}
			seenLink = true

			imports, err := federationLinkImports(dir)
			if err != nil {
				return "", err
			}
			for _, imp := range imports {
				fd, ok := federationDirectives[imp.name]
				if !ok {
					return "", federationErrorf(ErrUnsupportedFederationDirective, dir.Position,
						"imports @%s, which devbase's federation %s prelude does not define", imp.name, wantVersion)
				}
				directiveDefs = append(directiveDefs, fmt.Sprintf(fd.sdl, imp.alias))
				if fd.needsFieldSet {
					needsFieldSet = true
				}
			}
		}
	}

	if !seenLink {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("directive @link(url: String!, as: String, for: link__Purpose, import: [link__Import]) repeatable on SCHEMA\n")
	b.WriteString("scalar link__Import\n")
	b.WriteString("enum link__Purpose { SECURITY EXECUTION }\n")
	if needsFieldSet {
		b.WriteString("scalar FieldSet\n")
	}
	for _, def := range directiveDefs {
		b.WriteString(def)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// federationLinkVersion extracts the Federation subgraph spec version
// named by dir's `url` argument, or "" if url does not reference the
// Federation spec.
func federationLinkVersion(dir *ast.Directive) (string, error) {
	urlArg := dir.Arguments.ForName("url")
	if urlArg == nil {
		return "", nil
	}
	rawURL, err := urlArg.Value.Value(nil)
	if err != nil {
		return "", gqlerror.ErrorPosf(dir.Position, "evaluate @link url argument: %v", err)
	}
	url, _ := rawURL.(string)
	if !strings.HasPrefix(url, federationLinkURLPrefix) {
		return "", nil
	}
	return strings.TrimPrefix(url, federationLinkURLPrefix), nil
}

// federationLinkImports extracts dir's `import` list, if any, as the
// directive imports it names. It returns an error if an entry cannot
// be classified as a directive import.
func federationLinkImports(dir *ast.Directive) ([]federationImport, error) {
	importArg := dir.Arguments.ForName("import")
	if importArg == nil {
		return nil, nil
	}
	rawImports, err := importArg.Value.Value(nil)
	if err != nil {
		return nil, gqlerror.ErrorPosf(dir.Position, "evaluate @link import argument: %v", err)
	}

	list, _ := rawImports.([]any)
	imports := make([]federationImport, 0, len(list))
	for _, entry := range list {
		imp, err := parseFederationImportEntry(entry, dir.Position)
		if err != nil {
			return nil, err
		}
		imports = append(imports, imp)
	}
	return imports, nil
}

// parseFederationImportEntry classifies one entry of a `@link` import
// list, either a bare "@name" string or a {name: "@name", as:
// "@alias"} object, as a directive import. pos anchors any returned
// error to the @link application the entry came from.
func parseFederationImportEntry(entry any, pos *ast.Position) (federationImport, error) {
	switch v := entry.(type) {
	case string:
		name := strings.TrimPrefix(v, "@")
		if name == v {
			return federationImport{}, federationErrorf(ErrUnsupportedFederationDirective, pos,
				"imports %q, which is not a directive (expected a leading \"@\")", v)
		}
		return federationImport{name: name, alias: name}, nil
	case map[string]any:
		nameRaw, _ := v["name"].(string)
		name := strings.TrimPrefix(nameRaw, "@")
		alias := name
		if asRaw, ok := v["as"].(string); ok {
			alias = strings.TrimPrefix(asRaw, "@")
		}
		return federationImport{name: name, alias: alias}, nil
	default:
		return federationImport{}, federationErrorf(ErrUnsupportedFederationDirective, pos,
			"has an unrecognized import entry: %#v", entry)
	}
}

// federationErrorf builds a position-anchored error for a single
// `@link` application, rendered the same way Files' own Violation
// messages are, and wraps sentinel so callers can match it with
// errors.Is.
func federationErrorf(sentinel error, pos *ast.Position, format string, args ...any) error {
	gqlErr := gqlerror.ErrorPosf(pos, format, args...)
	gqlErr.Err = sentinel
	return gqlErr
}
