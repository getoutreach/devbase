// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Running git commands for the CODEOWNERS checks.

// Package gitcmd runs git commands for the CODEOWNERS checks.
package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Command returns the command that runs git with args in dir. With trust, git treats dir as a
// safe directory, even when another user owns it.
func Command(ctx context.Context, dir string, trust bool, args ...string) *exec.Cmd {
	if trust {
		args = append([]string{"-c", "safe.directory=" + dir}, args...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	return cmd
}

// Output runs cmd, a command from Command, and returns its standard output. The error names the
// command, its directory, and the standard error of git.
func Output(cmd *exec.Cmd) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s in %s: %w: %s", strings.Join(cmd.Args, " "), cmd.Dir, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// Run runs git with args in dir and returns its standard output. With trust, git treats dir as a
// safe directory, even when another user owns it.
func Run(ctx context.Context, dir string, trust bool, args ...string) ([]byte, error) {
	return Output(Command(ctx, dir, trust, args...))
}
