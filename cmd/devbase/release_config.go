// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: release config CLI subcommands, which convert the old
// release configuration files.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/getoutreach/devbase/v2/internal/release"
	"github.com/urfave/cli/v3"
)

// releaseConfigCommand returns the "release config" command.
func releaseConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Work with the release configuration",
		Commands: []*cli.Command{
			{
				Name: "convert",
				Usage: "Print the release section of scripts/devbase.yaml that replaces " +
					".releaserc.yaml and .goreleaser.yml",
				Description: "Reads .releaserc.yaml and .goreleaser.yml in the working directory, and prints the " +
					"equivalent release section without the default values. The Go submodules that " +
					"release-submodules.sh tags become units. Put the section in scripts/devbase.yaml and " +
					"delete both files. The release command fails while the section and either file exist. " +
					"See docs/releasing.md.",
				Action: releaseConfigConvert,
			},
		},
	}
}

// releaseConfigConvert is the Action for "release config convert". It
// prints the section to stdout, and notes to stderr.
func releaseConfigConvert(_ context.Context, c *cli.Command) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	section, notes, err := release.ConvertConfig(dir)
	if err != nil {
		return fmt.Errorf("convert release configuration: %w", err)
	}
	for _, n := range notes {
		fmt.Fprintln(c.Root().ErrWriter, "Note:", n)
	}
	_, err = c.Root().Writer.Write(section)
	return err
}
