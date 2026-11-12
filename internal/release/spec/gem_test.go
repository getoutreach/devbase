// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the gem plugin rules and the client version
// files.

package spec_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

func TestValidateGem(t *testing.T) {
	ok := []spec.GemPlugin{
		{},
		{Dir: "api/clients/ruby", Name: "widgets_client", VersionFile: "lib/widgets_client/version.rb", Owner: "getoutreach"},
	}
	for _, g := range ok {
		_, err := (&spec.Config{Branches: []spec.Branch{{Name: "main"}}, Plugins: []spec.Plugin{{Gem: &g}}}).Validate()
		assert.NilError(t, err, g)
	}
	bad := []spec.GemPlugin{
		{Dir: "../ruby"},
		{Dir: "/abs"},
		{Dir: "api/clients/ruby/"},
		{Dir: `api\ruby`},
		{VersionFile: "lib/version.txt"},
		{VersionFile: "../version.rb"},
		{Name: "bad name"},
		{Name: "-x"},
		{Owner: "acme/widgets"},
		{Owner: "-acme"},
	}
	for _, g := range bad {
		_, err := (&spec.Config{Branches: []spec.Branch{{Name: "main"}}, Plugins: []spec.Plugin{{Gem: &g}}}).Validate()
		assert.ErrorIs(t, err, spec.ErrUnsupported, g)
	}
}

func TestGemVersionPath(t *testing.T) {
	assert.Equal(t, (&spec.GemPlugin{}).VersionPath("a_client"), "api/clients/ruby/lib/a_client/version.rb")
	assert.Equal(t, (&spec.GemPlugin{Dir: "ruby", VersionFile: "v.rb"}).VersionPath("a_client"), "ruby/v.rb")
}
