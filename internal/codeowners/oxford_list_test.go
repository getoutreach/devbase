// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the English list of the CODEOWNERS error messages.

package codeowners

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestOxfordList(t *testing.T) {
	tests := map[string]struct {
		items []string
		want  string
	}{
		"no items":    {want: ""},
		"one item":    {items: []string{"a"}, want: "a"},
		"two items":   {items: []string{"a", "b"}, want: "a and b"},
		"three items": {items: []string{"a", "b", "c"}, want: "a, b, and c"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, oxfordList(tc.items), tc.want)
		})
	}
}
