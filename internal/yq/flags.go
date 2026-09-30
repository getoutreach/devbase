// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Command-line flag parsing for `devbase yq`.

package yq

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// yqFlags is devbase yq's own flag set. Every flag `devbase yq`
// recognizes -- implemented, recognized-but-deferred, or a named
// separate-feature gap -- has a field here, tagged the same way
// gojq's own cli.flagopts is (see parseYqFlags): `short`/`long` name
// the flag's spellings, and `args` documents a map-kind flag's two
// argument names. A flag with more than one long spelling (e.g. -y's
// --yaml-output and --yml-output) gets one field per spelling; normalize
// merges them.
//
// Only "genuinely unrecognized" (a token matching no field here) is a
// parse error; every other flag below is at least recognized, even
// when devbase yq does not yet act on it -- see Validate.
type yqFlags struct {
	// Implemented flags.
	RawOutput     bool `short:"r" long:"raw-output"`
	YAMLOutput    bool `short:"y" long:"yaml-output"`
	YAMLOutputYML bool `long:"yml-output"`
	CompactOutput bool `short:"c" long:"compact-output"`
	Slurp         bool `short:"s" long:"slurp"`
	InPlace       bool `short:"i" long:"in-place"`
	SortKeys      bool `short:"S" long:"sort-keys"`
	ASCIIOutput   bool `short:"a" long:"ascii-output"`
	Version       bool `short:"V" long:"version"`

	Arg       map[string]string `long:"arg" args:"name value"`
	ArgJSON   map[string]string `long:"argjson" args:"name value"`
	SlurpFile map[string]string `long:"slurpfile" args:"name path"`

	YAMLRoundtrip              bool `short:"Y" long:"yaml-roundtrip"`
	YAMLRoundtripYML           bool `long:"yml-roundtrip"`
	YAMLRoundtripGrammarVer    bool `long:"yaml-output-grammar-version"`
	YAMLRoundtripGrammarVerYML bool `long:"yml-out-ver"`

	IndentlessLists    bool `long:"indentless-lists"`
	IndentlessListsAlt bool `long:"indentless"`

	// Recognized, deferred: no current caller needs these (see the
	// flag table in the design plan for why each is nonetheless
	// recognized rather than rejected as unknown).
	FromFile           bool              `short:"f" long:"from-file"`
	RawFile            map[string]string `long:"rawfile" args:"name path"`
	YAMLFrontmatter    bool              `short:"F" long:"yaml-frontmatter"`
	Width              *int              `short:"w" long:"width"`
	ExplicitStart      bool              `long:"explicit-start"`
	ExplicitEnd        bool              `long:"explicit-end"`
	MaxExpansionFactor *int              `long:"max-expansion-factor"`
	NoExpandAliases    bool              `long:"no-expand-aliases"`

	// Separate, larger feature: a whole additional output format, not
	// implemented here.
	XMLOutput     bool `short:"x" long:"xml-output"`
	XMLInput      bool `long:"xml-input"`
	TOMLOutput    bool `short:"t" long:"toml-output"`
	TOMLRoundtrip bool `short:"T" long:"toml-roundtrip"`
}

// normalize folds every multi-spelling alias field into its primary
// field (e.g. YAMLOutputYML into YAMLOutput), so the rest of the
// package only ever needs to check one field per flag.
func (f *yqFlags) normalize() {
	f.YAMLOutput = f.YAMLOutput || f.YAMLOutputYML
	f.YAMLRoundtrip = f.YAMLRoundtrip || f.YAMLRoundtripYML ||
		f.YAMLRoundtripGrammarVer || f.YAMLRoundtripGrammarVerYML
	f.IndentlessLists = f.IndentlessLists || f.IndentlessListsAlt
}

// parseYqFlags parses args against a fresh yqFlags and returns it
// alongside the non-flag arguments (the filter, and any file operands
// for -i). It does not itself reject a recognized-but-unimplemented
// flag -- see Validate for that.
func parseYqFlags(args []string) (*yqFlags, []string, error) {
	var flags yqFlags
	rest, err := parseFlags(args, &flags)
	if err != nil {
		return nil, nil, err
	}
	flags.normalize()
	return &flags, rest, nil
}

// deferredFlag names a recognized-but-not-yet-implemented flag and how
// to tell whether it was set.
type deferredFlag struct {
	name string
	set  func(*yqFlags) bool
}

// deferredFlags lists every "recognized, deferred" flag: devbase yq
// parses it without error, but has no current caller and does not act
// on it yet. See the design plan's flag table for why each is
// nonetheless worth recognizing rather than rejecting as unknown.
var deferredFlags = []deferredFlag{ //nolint:gochecknoglobals // Why: read-only lookup table.
	{"-f/--from-file", func(f *yqFlags) bool { return f.FromFile }},
	{"--rawfile", func(f *yqFlags) bool { return f.RawFile != nil }},
	{"-F/--yaml-frontmatter", func(f *yqFlags) bool { return f.YAMLFrontmatter }},
	{"-w/--width", func(f *yqFlags) bool { return f.Width != nil }},
	{"--explicit-start", func(f *yqFlags) bool { return f.ExplicitStart }},
	{"--explicit-end", func(f *yqFlags) bool { return f.ExplicitEnd }},
	{"--max-expansion-factor", func(f *yqFlags) bool { return f.MaxExpansionFactor != nil }},
	{"--no-expand-aliases", func(f *yqFlags) bool { return f.NoExpandAliases }},
}

// separateFeatureFlag names a flag that belongs to a real but
// substantially separate feature devbase yq does not implement --
// naming the feature, not claiming the flag is impossible.
type separateFeatureFlag struct {
	name    string
	feature string
	set     func(*yqFlags) bool
}

// separateFeatureFlags lists every flag belonging to a whole
// additional output format (XML, TOML) that devbase yq does not
// implement. See the design plan's flag table for why this is framed
// as a deferred, separate feature rather than an impossibility.
var separateFeatureFlags = []separateFeatureFlag{ //nolint:gochecknoglobals // Why: read-only lookup table.
	{"-x/--xml-output", "XML output support", func(f *yqFlags) bool { return f.XMLOutput }},
	{"--xml-input", "XML input support", func(f *yqFlags) bool { return f.XMLInput }},
	{"-t/--toml-output", "TOML output support", func(f *yqFlags) bool { return f.TOMLOutput }},
	{"-T/--toml-roundtrip", "TOML round-trip support", func(f *yqFlags) bool { return f.TOMLRoundtrip }},
}

// Validate returns a fatal, actionable error for the first
// recognized-but-unimplemented flag set in f. A "recognized, deferred"
// flag and a "separate, larger feature" flag get distinct wording, so
// either reads differently from parseYqFlags's "unknown flag" error,
// which is reserved for a token matching no field in yqFlags at all.
func (f *yqFlags) Validate() error {
	for _, d := range deferredFlags {
		if d.set(f) {
			return fmt.Errorf("%s is a recognized flag that devbase yq does not implement yet "+
				"(no current caller needs it -- file an issue if you do)", d.name)
		}
	}
	for _, d := range separateFeatureFlags {
		if d.set(f) {
			return fmt.Errorf("%s is part of %s, a separate feature devbase yq does not implement", d.name, d.feature)
		}
	}
	return nil
}

// parseFlags is adapted from github.com/itchyny/gojq's unexported
// cli.parseFlags (cli/flags.go), reused here as devbase's own flag
// parser over yqFlags rather than gojq's own flagopts -- gojq's cli
// package cannot be imported for this directly, since its only
// exported entry point (cli.Run) offers no way to inject devbase's own
// argv/stdin/stdout. The struct-tag-driven reflection algorithm itself
// is unchanged from upstream, minus the "positional" case (for gojq's
// own --args/--jsonargs, which yqFlags has no equivalent of).
//
// gojq is distributed under the MIT License:
//
//	The MIT License (MIT)
//	Copyright (c) 2019-2026 itchyny
//
//	Permission is hereby granted, free of charge, to any person obtaining a copy
//	of this software and associated documentation files (the "Software"), to deal
//	in the Software without restriction, including without limitation the rights
//	to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
//	copies of the Software, and to permit persons to whom the Software is
//	furnished to do so, subject to the following conditions:
//
//	The above copyright notice and this permission notice shall be included in all
//	copies or substantial portions of the Software.
//
//	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
//	IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
//	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
//	AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
//	LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
//	OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
//	SOFTWARE.
func parseFlags(args []string, opts any) ([]string, error) {
	rest := make([]string, 0, len(args))
	val := reflect.ValueOf(opts).Elem()
	typ := val.Type()
	longToValue := map[string]reflect.Value{}
	shortToValue := map[string]reflect.Value{}
	for i := range val.NumField() {
		if flag, ok := typ.Field(i).Tag.Lookup("long"); ok {
			longToValue[flag] = val.Field(i)
		}
		if flag, ok := typ.Field(i).Tag.Lookup("short"); ok {
			shortToValue[flag] = val.Field(i)
		}
	}
	mapKeys := map[string]struct{}{}
	var optsDone bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var (
			val       reflect.Value
			ok        bool
			shortopts string
		)
		switch {
		case optsDone:
			// Skip option parsing after `--`.
		case arg == "--":
			optsDone = true
			continue
		case strings.HasPrefix(arg, "--"):
			if val, ok = longToValue[arg[2:]]; !ok {
				if j := strings.IndexByte(arg, '='); j >= 0 {
					if val, ok = longToValue[arg[2:j]]; ok {
						if val.Kind() == reflect.Bool {
							return nil, fmt.Errorf("boolean flag `%s' cannot have an argument", arg[:j])
						}
						args[i] = arg[j+1:]
						arg = arg[:j]
						i--
					}
				}
				if !ok {
					return nil, fmt.Errorf("unknown flag `%s'", arg)
				}
			}
		case len(arg) > 1 && arg[0] == '-':
			var skip bool
			for i := 1; i < len(arg); i++ {
				opt := arg[i : i+1]
				if val, ok = shortToValue[opt]; ok {
					if val.Kind() != reflect.Bool {
						break
					}
				} else if !("A" <= opt && opt <= "Z" || "a" <= opt && opt <= "z") {
					skip = true
					break
				}
			}
			if !skip && (len(arg) > 2 || !ok) {
				shortopts = arg[1:]
				goto L
			}
		}
		if !ok {
			rest = append(rest, arg)
			continue
		}
	S:
		switch val.Kind() {
		case reflect.Bool:
			val.SetBool(true)
		case reflect.String:
			if i++; i >= len(args) {
				return nil, fmt.Errorf("expected argument for flag `%s'", arg)
			}
			val.SetString(args[i])
		case reflect.Pointer:
			if val.Type().Elem().Kind() == reflect.Int {
				if i++; i >= len(args) {
					return nil, fmt.Errorf("expected argument for flag `%s'", arg)
				}
				v, err := strconv.Atoi(args[i])
				if err != nil {
					return nil, fmt.Errorf("invalid argument for flag `%s': %w", arg, err)
				}
				val.Set(reflect.New(val.Type().Elem()))
				val.Elem().SetInt(int64(v))
			}
		case reflect.Map:
			if i += 2; i >= len(args) {
				return nil, fmt.Errorf("expected 2 arguments for flag `%s'", arg)
			}
			if val.IsNil() {
				val.Set(reflect.MakeMap(val.Type()))
			}
			name := args[i-1]
			if _, ok := mapKeys[name]; !ok {
				mapKeys[name] = struct{}{}
				val.SetMapIndex(reflect.ValueOf(name), reflect.ValueOf(args[i]))
			}
		}
	L:
		if shortopts != "" {
			opt := shortopts[:1]
			if val, ok = shortToValue[opt]; !ok {
				return nil, fmt.Errorf("unknown flag `%s'", opt)
			}
			if val.Kind() != reflect.Bool && len(shortopts) > 1 {
				if shortopts[1] == '=' {
					args[i] = shortopts[2:]
				} else {
					args[i] = shortopts[1:]
				}
				i--
				shortopts = ""
			} else {
				shortopts = shortopts[1:]
			}
			arg = "-" + opt
			goto S
		}
	}
	return rest, nil
}
