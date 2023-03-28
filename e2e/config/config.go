// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: Logic related to Devenv.yaml config

// Package config contains logic related to Devenv.yaml config
package config

import (
	"context"
	"os"

	"github.com/getoutreach/gobox/pkg/box"
	"github.com/google/go-github/v47/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

// Devenv is a struct that contains the devenv configuration
// which is usually called "devenv.yaml". This also works for the
// legacy service.yaml format.
type Devenv struct {
	// Service denotes if this repository is a service.
	Service bool `yaml:"service"`

	Dependencies struct {
		// Optional is a list of OPTIONAL services e.g. the service can run / gracefully function without it running
		Optional []string `yaml:"optional"`

		// Required is a list of services that this service cannot function without
		Required []string `yaml:"required"`
	} `yaml:"dependencies"`
}

// FromFile parses the devenv.yaml file and returns a DevenvConfig
func FromFile(confPath string) (*Devenv, error) {
	f, err := os.Open(confPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read devenv.yaml or service.yaml")
	}
	defer f.Close()

	var dc Devenv
	if err := yaml.NewDecoder(f).Decode(&dc); err != nil {
		return nil, errors.Wrapf(err, "failed to parse devenv.yaml or service.yaml")
	}

	return &dc, nil
}

// FromGitHub reads and parses DevenvConfig from GitHub
func FromGitHub(ctx context.Context, conf *box.Config, serviceName string,
	gh *github.Client, configFileName string) (*Devenv, error) {
	r, _, err := gh.Repositories.DownloadContents(ctx, conf.Org, serviceName, configFileName, nil)
	l := log.With().Str("service", serviceName).Str("file", configFileName).Logger()
	if err != nil {
		l.Debug().Msg("Unable to find file in GH")
		return nil, err
	}
	defer r.Close()
	var dc Devenv
	if err := yaml.NewDecoder(r).Decode(&dc); err != nil {
		l.Warn().Msg("Unable to parse config file")
		return nil, err
	}
	return &dc, nil
}

// getDependencies returns dependencies to install based on Devenv config and DEPLOY_OPTIONAL_DEPENDENCIES environment variable
func (c *Devenv) GetDependencies() []string {
	deps := make([]string, 0)
	deps = append(deps, c.Dependencies.Required...)
	if os.Getenv("DEPLOY_OPTIONAL_DEPENDENCIES") == "true" {
		deps = append(deps, c.Dependencies.Optional...)
	}
	return deps
}
