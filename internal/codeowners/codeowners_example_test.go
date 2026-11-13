// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Example usage of the CODEOWNERS file loader.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package codeowners_test

import (
	"fmt"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
)

func ExampleLoad() {
	entries, err := codeowners.Load("./testdata/")
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, e := range entries {
		fmt.Printf("[line] %d: [pattern]: %s [owners]: %v\n", e.LineNo, e.Pattern, e.Owners)
	}

	// Output:
	// [line] 8: [pattern]: * [owners]: [@global-owner1 @global-owner2]
	// [line] 14: [pattern]: *.js [owners]: [@js-owner]
	// [line] 19: [pattern]: *.go [owners]: [docs@example.com]
	// [line] 24: [pattern]: /build/logs/ [owners]: [@doctocat]
	// [line] 29: [pattern]: docs/* [owners]: [docs@example.com]
	// [line] 33: [pattern]: apps/ [owners]: [@octocat]
	// [line] 37: [pattern]: /docs/ [owners]: [@doctocat]
}
