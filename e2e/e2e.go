package main

import (
	"context"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/getoutreach/gobox/pkg/sshhelper"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var virtualDeps = map[string][]string{
	// TODO: Put a service.yaml in flagship with this
	"flagship": {
		"outreach-templating-service",
		"olis",
		"mint",
		"giraffe",
		"outreach-accounts",
		"clientron",
	},
}

// Service is a mock of the service.yaml in bootstrap, which isn't currently
// open-sourced, yet!
type Service struct {
	Dependencies struct {
		// Optional is a list of OPTIONAL services e.g. the service can run / gracefully function without it running
		Optional []string `yaml:"optional"`

		// Reqiored is a list of services that this service cannot function without
		Required []string `yaml:"required"`
	} `yaml:"dependencies"`
}

// BuildDependenciesList builds a list of dependencies by cloning them
// and appending them to the list. Deduplication is done and returned
// is a flat list of all of the dependencies of the initial root
// application who's dependency list was provided.
func BuildDependenciesList(ctx context.Context) ([]string, error) {
	deps := make(map[string]bool)

	a := sshhelper.GetSSHAgent()
	_, err := sshhelper.LoadDefaultKey("github.com", a, logrus.StandardLogger())
	if err != nil {
		return nil, err
	}

	auth := sshhelper.NewExistingSSHAgentCallback(a)

	f, err := os.Open("service.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read service.yaml")
	}

	var s *Service
	err = yaml.NewDecoder(f).Decode(&s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse service.yaml")
	}

	// Populate the initial dependencies to prevent us
	// downloading them again later
	for _, d := range s.Dependencies.Optional {
		deps[d] = true
	}

	for _, d := range s.Dependencies.Required {
		deps[d] = true
	}

	for _, d := range append(s.Dependencies.Required, s.Dependencies.Optional...) {
		err := grabDependencies(ctx, deps, d, auth)
		if err != nil {
			return nil, err
		}
	}

	depsList := make([]string, 0)
	for d := range deps {
		depsList = append(depsList, d)
	}

	return depsList, nil
}

// grabDependencies traverses the dependency tree by calculating
// it on the fly via git cloning of the dependencies. Passed in
// is a hash map used to prevent infinite recurison and de-duplicate
// dependencies. New dependencies are inserted into the provided hash-map
func grabDependencies(ctx context.Context, deps map[string]bool, name string, auth *sshhelper.ExistingSSHAgentCallback) error {
	fs := memfs.New()

	// Skip if we've already been downloaded
	if _, ok := deps[name]; ok {
		return nil
	}

	log.Info().Str("dep", name).Msg("Resolving dependency")

	var foundDeps []string

	// If we don't have a virtualDeps entry here, then download the git
	// repo, read service.yaml, and
	if _, ok := virtualDeps[name]; !ok {
		opts := &git.CloneOptions{
			URL:  "git@github.com:" + path.Join("getoutreach", name),
			Auth: auth,
		}
		_, err := git.CloneContext(ctx, memory.NewStorage(), fs, opts)
		if err != nil {
			return errors.Wrapf(err, "failed to clone dependency %s", name)
		}

		f, err := fs.Open("service.yaml")
		if err != nil {
			deps[name] = true
			log.Warn().Err(err).Msg("Failed to find service.yaml, will not try to calculate dependencies of this service")
			return nil
		}

		var s *Service
		err = yaml.NewDecoder(f).Decode(&s)
		if err != nil {
			return errors.Wrapf(err, "failed to parse service.yaml in dependency %s", name)
		}

		foundDeps = append(s.Dependencies.Required, s.Dependencies.Optional...)
	} else {
		log.Info().Msgf("Using baked-in dependency list")
		foundDeps = virtualDeps[name]
	}

	for _, d := range foundDeps {
		err := grabDependencies(ctx, deps, d, auth)
		if err != nil {
			return err
		}
	}

	deps[name] = true

	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	log.Info().Msg("Building dependency tree")

	deps, err := BuildDependenciesList(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to build dependency tree")
	}

	log.Info().Strs("deps", deps).Msg("Provisioning devenv")

	// TODO: outreach specific code
	target := "base"
	for _, d := range deps {
		if d == "flagship" {
			target = "flagship"
			break
		}
	}

	//nolint:errcheck // Why: Best effort remove existing cluster
	exec.CommandContext(ctx, "devenv", "destroy").Run()

	cmd := exec.CommandContext(ctx, "devenv", "--skip-update", "provision", "--snapshot-target", target)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to provision devenv")
	}

	for _, d := range deps {
		// Skip dep with same name as our target, e.g. flagship
		if d == target {
			continue
		}

		log.Info().Msgf("Deploying dependency '%s'", d)
		cmd := exec.CommandContext(ctx, "devenv", "--skip-update", "deploy-app", d)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		err = cmd.Run()
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to deploy dependency '%s'", d)
		}
	}

	log.Info().Msg("Deploying current application into cluster")
	cmd = exec.CommandContext(ctx, "devenv", "--skip-update", "deploy-app", ".")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to deploy current application into devenv")
	}

	log.Info().Msg("Waiting for application to be ready")
	time.Sleep(30 * time.Second) // TODO: Eventually actually wait

	log.Info().Msg("Running e2e tests")
	cmd = exec.CommandContext(ctx, "./.bootstrap/shell/test.sh")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	if err != nil {
		log.Fatal().Err(err).Msg("E2E tests failed, or failed to run")
	}
}
