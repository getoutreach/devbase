// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the client hook matchers.

package spec_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

func TestClientHooks(t *testing.T) {
	for _, tc := range []struct {
		cmd          string
		ruby, python bool
	}{
		{"./scripts/shell-wrapper.sh ruby/build.sh ${nextRelease.version} 1>&2", true, false},
		{"DRYRUN=x ./scripts/shell-wrapper.sh ruby/publish.sh 1.0.0", true, false},
		{"cd py && $(mise which poetry) version ${nextRelease.version}", false, true},
		{"poetry build\n  && echo version", false, true},
		{"mise run publish:pyclient --environment=production", false, true},
		{"ruby/test.sh && poetry install", false, false},
		{"./scripts/publish.sh ${nextRelease.version}", false, false},
	} {
		assert.Equal(t, spec.IsRubyClientHook(tc.cmd), tc.ruby, tc.cmd)
		assert.Equal(t, spec.IsPythonClientHook(tc.cmd), tc.python, tc.cmd)
	}
}
