//go:build mage

package main

import (
	"fmt"
	"os"

	"github.com/getoutreach/gobox/pkg/box"
	"github.com/magefile/mage/sh"
)

// runGoCommand runs the given go command with the given arguments
// while setting required environment variables
func runGoCommand(args ...string) error {
	goFlags := ""
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		// When not running in Kubernetes, build in or_dev mode
		goFlags = "-tags=or_dev"
	}

	conf, err := box.LoadBox()
	if err != nil {
		return err
	}

	return sh.RunWith(map[string]string{
		"GOFLAGS":   goFlags,
		"GOPRIVATE": fmt.Sprintf("github.com/%s/*", conf.Org),
	}, "go", args...)
}

// getLDFlagsStringFromMap returns a string of all the ldflags from the given map
func getLDFlagsStringFromMap(ldflags map[string]string) string {
	ldFlags := ""
	for k, v := range ldflags {
		ldFlags += fmt.Sprintf("-X %s=%s ", k, v)
	}
	return ldFlags
}
