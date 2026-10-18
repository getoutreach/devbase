// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the e2e runner.

package main

import "testing"

func TestWithTestParallelism(t *testing.T) {
	tests := []struct {
		flags string
		want  string
	}{
		{flags: "", want: "-p 2"},
		{flags: "-v", want: "-v -p 2"},
		{flags: "-p 4", want: "-p 4"},
		{flags: "-v -p=1", want: "-v -p=1"},
		{flags: "-parallel 8", want: "-parallel 8 -p 2"},
	}
	for _, tt := range tests {
		if got := withTestParallelism(tt.flags); got != tt.want {
			t.Errorf("withTestParallelism(%q) = %q, want %q", tt.flags, got, tt.want)
		}
	}
}
