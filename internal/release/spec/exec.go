// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Exec plugin commands: shell commands with ${...} lookups
// of release fields, as @semantic-release/exec renders them.

package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// execExprRegex matches a ${...} lookup in an exec command.
var execExprRegex = regexp.MustCompile(`\$\{([^}]*)\}`)

// execFields are the lookups that exec commands may use.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var execFields = map[string]bool{
	"nextRelease.version": true,
	"nextRelease.gitTag":  true,
	"nextRelease.channel": true,
	"lastRelease.version": true,
	"lastRelease.gitTag":  true,
	"branch.name":         true,
	"options.dryRun":      true,
}

// CheckExecCommand rejects an exec command with a lookup outside the
// supported release fields, or with a lodash <% %> template. The
// lowercase ${options.dryrun}, which semantic-release always renders as
// an empty string, is accepted with a warning.
func CheckExecCommand(cmd string) (warnings []string, err error) {
	if strings.Contains(cmd, "<%") {
		return nil, fmt.Errorf("%w: lodash template in %q", ErrUnsupported, cmd)
	}
	for _, m := range execExprRegex.FindAllStringSubmatch(cmd, -1) {
		field := strings.TrimSpace(m[1])
		if field == "options.dryrun" {
			warnings = append(warnings, "use ${options.dryRun}: semantic-release renders ${options.dryrun} as an empty string")
			continue
		}
		if !execFields[field] {
			return nil, fmt.Errorf("%w: exec command field %q", ErrUnsupported, field)
		}
	}
	return warnings, nil
}
