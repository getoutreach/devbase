// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the git URL path parser.

package giturl_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/giturl"
	"gotest.tools/v3/assert"
)

func TestPath(t *testing.T) {
	tests := map[string]string{
		"https://github.com/o/r.git":            "o/r.git",
		"https://github.com/o/r":                "o/r",
		"https://user:token@github.com/o/r.git": "o/r.git",
		"git@github.com:o/r.git":                "o/r.git",
		"ssh://git@github.com/o/r":              "/o/r",
		"/local/repository":                     "/local/repository",
	}
	for rawURL, want := range tests {
		t.Run(rawURL, func(t *testing.T) {
			path, err := giturl.Path(rawURL)
			assert.NilError(t, err)
			assert.Equal(t, path, want)
		})
	}
}

func TestPathError(t *testing.T) {
	for _, rawURL := range []string{
		"https://x-access-token:ghp_SECRETTOKEN#@github.com/o/r.git",
		"https://user:s3cr3t%zz@github.com/o/r.git",
		"https://user:s3cr3t?x@github.com/o/r.git",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := giturl.Path(rawURL)
			assert.ErrorIs(t, err, giturl.ErrInvalidURL)
			assert.Error(t, err, "the URL cannot be parsed")
		})
	}
}
