// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for devbase yq's flag parser and validation pass.

package yq

import (
	"testing"

	"gotest.tools/v3/assert"
)

// TestParseYqFlags_Basics confirms plain long and short flags parse
// into the right fields and that the filter/files rest is preserved in
// order.
func TestParseYqFlags_Basics(t *testing.T) {
	flags, rest, err := parseYqFlags([]string{"-r", "--slurp", ".foo", "file.yaml"})
	assert.NilError(t, err)
	assert.Assert(t, flags.RawOutput)
	assert.Assert(t, flags.Slurp)
	assert.DeepEqual(t, rest, []string{".foo", "file.yaml"})
}

// TestParseYqFlags_Aliases confirms -y/--yaml-output/--yml-output are
// true aliases of one flag, and likewise for -Y's four spellings and
// --indentless-lists/--indentless.
func TestParseYqFlags_Aliases(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want func(*yqFlags) bool
	}{
		{"short -y", []string{"-y", "."}, func(f *yqFlags) bool { return f.YAMLOutput }},
		{"--yaml-output", []string{"--yaml-output", "."}, func(f *yqFlags) bool { return f.YAMLOutput }},
		{"--yml-output", []string{"--yml-output", "."}, func(f *yqFlags) bool { return f.YAMLOutput }},
		{"short -Y", []string{"-Y", "."}, func(f *yqFlags) bool { return f.YAMLRoundtrip }},
		{"--yaml-roundtrip", []string{"--yaml-roundtrip", "."}, func(f *yqFlags) bool { return f.YAMLRoundtrip }},
		{"--yml-roundtrip", []string{"--yml-roundtrip", "."}, func(f *yqFlags) bool { return f.YAMLRoundtrip }},
		{
			"--yaml-output-grammar-version",
			[]string{"--yaml-output-grammar-version", "."},
			func(f *yqFlags) bool { return f.YAMLRoundtrip },
		},
		{"--yml-out-ver", []string{"--yml-out-ver", "."}, func(f *yqFlags) bool { return f.YAMLRoundtrip }},
		{"--indentless-lists", []string{"--indentless-lists", "."}, func(f *yqFlags) bool { return f.IndentlessLists }},
		{"--indentless", []string{"--indentless", "."}, func(f *yqFlags) bool { return f.IndentlessLists }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			flags, _, err := parseYqFlags(c.args)
			assert.NilError(t, err)
			assert.Assert(t, c.want(flags))
		})
	}
}

// TestParseYqFlags_Clustering confirms bundled short flags parse
// identically to their unbundled equivalents, matching real call
// sites audited from orc#2042 (-cr, -ry, -yi -- -ni is skipped, since
// -n/--null-input has no equivalent among devbase yq's own flags).
func TestParseYqFlags_Clustering(t *testing.T) {
	cases := []struct {
		name      string
		bundled   string
		unbundled []string
	}{
		{"-cr", "-cr", []string{"-c", "-r"}},
		{"-ry", "-ry", []string{"-r", "-y"}},
		{"-yi", "-yi", []string{"-y", "-i"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundled, _, err := parseYqFlags([]string{c.bundled, "."})
			assert.NilError(t, err)

			unbundledArgs := append(append([]string{}, c.unbundled...), ".")
			unbundled, _, err := parseYqFlags(unbundledArgs)
			assert.NilError(t, err)

			assert.DeepEqual(t, bundled, unbundled)
		})
	}
}

// TestParseYqFlags_Clustering_ThreeWay confirms three bundled bool
// short flags parse the same as their unbundled equivalents.
func TestParseYqFlags_Clustering_ThreeWay(t *testing.T) {
	bundled, _, err := parseYqFlags([]string{"-cry", "."})
	assert.NilError(t, err)
	assert.Assert(t, bundled.CompactOutput)
	assert.Assert(t, bundled.RawOutput)
	assert.Assert(t, bundled.YAMLOutput)
}

// TestParseYqFlags_TwoTokenFlags confirms --arg/--argjson/--slurpfile
// each consume exactly two argv tokens and bind by name, matching
// box.sh:52-53's --slurpfile usage.
func TestParseYqFlags_TwoTokenFlags(t *testing.T) {
	flags, rest, err := parseYqFlags([]string{
		"--arg", "name", "value",
		"--argjson", "count", "3",
		"--slurpfile", "boxconf", "box.yaml",
		".",
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, flags.Arg, map[string]string{"name": "value"})
	assert.DeepEqual(t, flags.ArgJSON, map[string]string{"count": "3"})
	assert.DeepEqual(t, flags.SlurpFile, map[string]string{"boxconf": "box.yaml"})
	assert.DeepEqual(t, rest, []string{"."})
}

// TestParseYqFlags_UnknownFlag confirms a token matching no yqFlags
// field is a distinct, generic error -- not the deferred/separate-
// feature wording Validate produces for a recognized flag.
func TestParseYqFlags_UnknownFlag(t *testing.T) {
	_, _, err := parseYqFlags([]string{"--not-a-real-flag", "."})
	assert.ErrorContains(t, err, "unknown flag")
}

// TestValidate_Deferred confirms a recognized-but-deferred flag
// produces the "recognized flag... does not implement yet" message,
// distinct from an unknown flag.
func TestValidate_Deferred(t *testing.T) {
	flags, _, err := parseYqFlags([]string{"-f", "."})
	assert.NilError(t, err)
	err = flags.Validate()
	assert.ErrorContains(t, err, "-f/--from-file is a recognized flag that devbase yq does not implement yet")
}

// TestValidate_SeparateFeature confirms a flag from a separate, larger
// feature (XML/TOML support) names that feature rather than claiming
// the flag is impossible.
func TestValidate_SeparateFeature(t *testing.T) {
	flags, _, err := parseYqFlags([]string{"-x", "."})
	assert.NilError(t, err)
	err = flags.Validate()
	assert.ErrorContains(t, err, "-x/--xml-output is part of XML output support")
}

// TestValidate_ImplementedFlagsPass confirms Validate accepts every
// implemented flag without error.
func TestValidate_ImplementedFlagsPass(t *testing.T) {
	flags, _, err := parseYqFlags([]string{
		"-r", "-y", "-c", "-s", "-i", "-S", "-a", "-V", "-Y", "--indentless",
		"--arg", "n", "v",
		".",
	})
	assert.NilError(t, err)
	assert.NilError(t, flags.Validate())
}
