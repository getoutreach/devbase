//go:build mage

package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
	"github.com/getoutreach/gobox/pkg/box"
	"github.com/magefile/mage/sh"
	"github.com/pkg/errors"

	"github.com/rs/zerolog"
)

func pathFromGitURL(origin string) (string, error) {
	if strings.HasPrefix(origin, "https://") {
		u, err := url.Parse(origin)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse HTTPS URL")
		}
		return strings.TrimPrefix(u.Path, "/"), nil
	}

	u, err := giturls.Parse(origin)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse SSH URL")
	}
	return u.Path, nil
}

// getOrg returns the Github organization name of the current repository
func getOrg() (string, error) {
	conf, err := box.LoadBox()
	if err == nil {
		return conf.Org, nil
	}

	// Fallback to reading the git origin
	origin, err := sh.Output("git", "remote", "get-url", "origin")
	if err != nil {
		return "", errors.Wrap(err, "failed to get git origin")
	}

	urlPath, err := pathFromGitURL(origin)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse git origin %q", origin)
	}

	// ["getoutreach", "gobox"]
	spl := strings.Split(urlPath, "/")
	if len(spl) != 2 {
		return "", fmt.Errorf("failed to parse org from git origin %q", origin)
	}

	return spl[0], nil
}

// runGoCommand runs the given go command with the given arguments
// while setting required environment variables
func runGoCommand(log zerolog.Logger, args ...string) error {
	goFlags := ""
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		// When not running in Kubernetes, build in or_dev mode
		goFlags = "-tags=or_dev"
	}

	org, err := getOrg()
	if err != nil {
		return errors.Wrap(err, "failed to get determine org")
	}

	vars := map[string]string{
		"GOFLAGS": goFlags,
		// TODO(jaredallard): We may not always want to set GOPRIVATE...
		"GOPRIVATE": fmt.Sprintf("github.com/%s/*", org)}

	if goos := os.Getenv("BUILD_FOR_GOOS"); goos != "" { // BUILD_FOR_GOOS is used when we build on macos for linux
		log.Info().Msgf("Building for GOOS %s", goos)
		vars["GOOS"] = goos
	}

	return sh.RunWith(vars, "go", args...)
}

// getLDFlagsStringFromMap returns a string of all the ldflags from the given map
func getLDFlagsStringFromMap(ldflags map[string]string) string {
	ldFlags := ""
	for k, v := range ldflags {
		ldFlags += fmt.Sprintf("-X %s=%s ", k, v)
	}
	return ldFlags
}
