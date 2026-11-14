// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The exec hooks that run the stencil Ruby and Python
// client scripts, which the gem and python plugins replace.

package spec

import "regexp"

// These match a hook command that runs the stencil client scripts.
var (
	// rubyClientHookRe matches the Ruby client build or publish script.
	rubyClientHookRe = regexp.MustCompile(`ruby/(build|publish)\.sh`)

	// pythonClientHookRe matches the poetry version command or the
	// publish:pyclient mise task of the Python client.
	pythonClientHookRe = regexp.MustCompile(`(?s)poetry.*\bversion\b|publish:pyclient`)
)

// IsRubyClientHook reports whether the hook command cmd runs the stencil
// Ruby client scripts, which the gem plugin replaces.
func IsRubyClientHook(cmd string) bool {
	return rubyClientHookRe.MatchString(cmd)
}

// IsPythonClientHook reports whether the hook command cmd runs the
// stencil Python client commands, which the python plugin replaces.
func IsPythonClientHook(cmd string) bool {
	return pythonClientHookRe.MatchString(cmd)
}
