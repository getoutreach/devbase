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

// Error is the error of a git command that failed.
type Error struct {
	// Args are the arguments of the command, starting with "git".
	Args []string

	// Dir is the directory where the command ran.
	Dir string

	// Stderr is the standard error of git, without leading and trailing white space.
	Stderr string

	// Err is the error of the command, such as an *exec.ExitError.
	Err error
}

// Error returns the command, its directory, its error, and the standard error of git.
func (e *Error) Error() string {
	return fmt.Sprintf("%s in %s: %v: %s", strings.Join(e.Args, " "), e.Dir, e.Err, e.Stderr)
}

// Unwrap returns the error of the command.
func (e *Error) Unwrap() error {
	return e.Err
}

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

// Output runs cmd, a command from Command, and returns its standard output. When the command
// fails, the error is an *Error.
func Output(cmd *exec.Cmd) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &Error{Args: cmd.Args, Dir: cmd.Dir, Stderr: strings.TrimSpace(stderr.String()), Err: err}
	}

	return stdout.Bytes(), nil
}

// Run runs git with args in dir and returns its standard output. With trust, git treats dir as a
// safe directory, even when another user owns it.
func Run(ctx context.Context, dir string, trust bool, args ...string) ([]byte, error) {
	return Output(Command(ctx, dir, trust, args...))
}
