// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Exec plugin commands: shell commands with ${...} lookups
// of release fields, as @semantic-release/exec renders them.

package spec

import (
	"fmt"
	"regexp"
	"strconv"
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

// ExecFields are the values of the ${...} lookups of exec commands.
type ExecFields struct {
	NextVersion, NextTag, NextChannel string
	LastVersion, LastTag              string
	Branch                            string
	DryRun                            bool
}

// RenderExecCommand replaces the ${...} lookups of a command that
// CheckExecCommand accepted. A missing value, such as the channel of the
// default channel, renders as an empty string, as lodash renders null.
// ${options.dryRun} and ${options.dryrun} render the dry-run value;
// semantic-release renders an empty string for the lowercase form.
func RenderExecCommand(cmd string, f *ExecFields) string {
	values := map[string]string{
		"nextRelease.version": f.NextVersion,
		"nextRelease.gitTag":  f.NextTag,
		"nextRelease.channel": f.NextChannel,
		"lastRelease.version": f.LastVersion,
		"lastRelease.gitTag":  f.LastTag,
		"branch.name":         f.Branch,
		"options.dryRun":      strconv.FormatBool(f.DryRun),
		"options.dryrun":      strconv.FormatBool(f.DryRun),
	}
	return execExprRegex.ReplaceAllStringFunc(cmd, func(m string) string {
		return values[strings.TrimSpace(execExprRegex.FindStringSubmatch(m)[1])]
	})
}
