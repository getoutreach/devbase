// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: This file has the package main.
package main

import (
	"context"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/getoutreach/devbase/v2/e2e/config"
	"github.com/getoutreach/gobox/pkg/box"
	githubauth "github.com/getoutreach/gobox/pkg/cli/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// flagship is the name of the flagship
const flagship = "flagship"

// osStdInOutErr is a helper function to use the os stdin/out/err
func osStdInOutErr(c *exec.Cmd) *exec.Cmd {
	c.Stdin = os.Stdin
	return osStdOutErr(c)
}

// osStdOutErr is a helper function to use the os stdout/err
func osStdOutErr(c *exec.Cmd) *exec.Cmd {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// BuildDependenciesList builds a list of dependencies by cloning them
// and appending them to the list. Deduplication is done and returned
// is a flat list of all of the dependencies of the initial root
// application who's dependency list was provided.
func BuildDependenciesList(ctx context.Context, conf *box.Config) ([]string, error) {
	deps := make(map[string]struct{})

	dc, err := config.FromFile("devenv.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse devenv.yaml")
	}

	for _, d := range dc.GetAllDependencies() {
		if err := grabDependencies(ctx, conf, deps, d); err != nil {
			return nil, err
		}
	}

	depsList := make([]string, 0)
	for d := range deps {
		depsList = append(depsList, d)
	}

	return depsList, nil
}

// findDependenciesInRepo finds the dependencies in a repository
// at all of the possible paths
func findDependenciesInRepo(ctx context.Context, conf *box.Config, serviceName string) (map[string]struct{}, error) {
	possibleFiles := []string{"devenv.yaml", "noncompat-service.yaml", "service.yaml"}
	gh, err := githubauth.NewClient()
	if err != nil {
		return nil, err
	}

	var dc *config.Devenv
	for _, f := range possibleFiles {
		dc, err = config.FromGitHub(ctx, conf, serviceName, gh, f)
		if err != nil {
			continue // we continue to the next file, err is logged in getConfig
		}

		// We found a file, stop looking
		break
	}

	if dc == nil {
		log.Warn().Str("service", serviceName).
			Msgf("Failed to find any of the following %v, will not try to calculate dependencies of this service", possibleFiles)
		return nil, nil
	}

	deps := make(map[string]struct{})
	// We deploy just required transitive dependencies
	for _, d := range dc.Dependencies.Required {
		deps[d] = struct{}{}
	}

	return deps, nil
}

// grabDependencies traverses the dependency tree by calculating
// it on the fly via git cloning of the dependencies. Passed in
// is a hash map used to prevent infinite recursion and de-duplicate
// dependencies. New dependencies are inserted into the provided hash-map
func grabDependencies(ctx context.Context, conf *box.Config, deps map[string]struct{}, serviceName string) error {
	// We special case this here to ensure we don't fail on deps that haven't updated
	// their dependency yet.
	if serviceName == flagship {
		serviceName = "outreach"
	}

	// Skip if we've already been downloaded
	if _, ok := deps[serviceName]; ok {
		return nil
	}

	log.Info().Str("dep", serviceName).Msg("Resolving dependency")

	// Find the dependencies of this repo
	foundDeps, err := findDependenciesInRepo(ctx, conf, serviceName)

	if err != nil {
		return errors.Wrap(err, "failed to grab dependencies")
	}

	// Mark us as resolved to prevent inf dependency resolution
	// when we encounter cyclical dependency.
	deps[serviceName] = struct{}{}

	for d := range foundDeps {
		if err := grabDependencies(ctx, conf, deps, d); err != nil {
			return err
		}
	}

	return nil
}

// provisionNew destroys and re-provisions a devenv
func provisionNew(ctx context.Context, target string) error { // nolint:unparam // Why: keeping in the interface for now
	//nolint:errcheck // Why: Best effort remove existing cluster
	exec.CommandContext(ctx, "devenv", "--skip-update", "destroy").Run()

	if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update",
		"provision", "--snapshot-target", target)).Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to provision devenv")
	}

	return nil
}

// runDevconfig executes devconfig command
func runDevconfig(ctx context.Context) {
	if err := osStdOutErr(exec.CommandContext(ctx, ".bootstrap/shell/devconfig.sh")).Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to run devconfig")
	}
}

// shouldRunE2ETests denotes whether or not this needs to actually
// run
func shouldRunE2ETests() (bool, error) {
	var runEndToEndTests bool

	build.Default.BuildTags = []string{"or_test", "or_e2e"}

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if runEndToEndTests {
			// No need to keep walking through files if we've already found one file
			// that requires e2e tests.
			return nil
		}

		if err != nil {
			return err
		}

		if info.IsDir() && path != "." {
			// Skip submodule directories.
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				return filepath.SkipDir
			}
		}

		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			// Skip symlinks.
			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			// Skip all files that aren't go test files.
			return nil
		}

		pkg, err := build.Import(path, filepath.Base(path), build.ImportComment)
		if err != nil {
			// Skip files that are not compatible with current build tags like or_int
			var noGoErr *build.NoGoError
			if errors.As(err, &noGoErr) {
				return nil
			}
			return errors.Wrap(err, "import")
		}

		for _, tag := range pkg.AllTags {
			runEndToEndTests = runEndToEndTests || tag == "or_e2e"
		}

		return nil
	})
	return runEndToEndTests, err
}

func main() { //nolint:funlen,gocyclo // Why: there are no reusable parts to extract
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	conf, err := box.EnsureBoxWithOptions(ctx)
	if err != nil {
		//nolint:gocritic // Why: We're OK with this.
		log.Fatal().Err(err).Msg("Failed to load box config")
	}

	if conf.DeveloperEnvironmentConfig.VaultConfig.Enabled {
		vaultAddr := conf.DeveloperEnvironmentConfig.VaultConfig.Address
		if os.Getenv("CI") == "true" { //nolint:goconst // Why: true == true
			vaultAddr = conf.DeveloperEnvironmentConfig.VaultConfig.AddressCI
		}
		log.Info().Str("vault-addr", vaultAddr).Msg("Set Vault Address")
		os.Setenv("VAULT_ADDR", vaultAddr)
	}

	// No or_e2e build tags were found.
	runE2ETests, err := shouldRunE2ETests()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to determine if e2e tests should be run")
	}
	if !runE2ETests {
		log.Info().Msg("found no occurrences of or_e2e build tags, skipping e2e tests")
		return
	}

	log.Info().Msg("Building dependency tree")

	// Provision a devenv if it doesn't already exist. If it does exist,
	// warn the user their test is no longer potentially reproducible.
	// Allow skipping provision, this is generally only useful for the devenv
	// which uses this framework -- but provisions itself.
	if os.Getenv("SKIP_DEVENV_PROVISION") != "true" {
		if exec.CommandContext(ctx, "devenv", "--skip-update", "status").Run() != nil {
			var wg sync.WaitGroup
			dockerBuilt := false
			wg.Add(1)

			// Build docker sooner and out of critical path to speed things up.
			// Docker build in devenv apps deploy . will be superfast then.
			go func(wg *sync.WaitGroup) {
				defer wg.Done()
				log.Info().Msg("Starting early docker build")
				if err := exec.CommandContext(ctx, "make", "docker-build").Run(); err != nil {
					log.Warn().Err(err).Msg("Error when running early docker build")
				} else {
					log.Info().Msg("Early docker build finished successfully")
				}
				dockerBuilt = true
			}(&wg)

			deps, err := BuildDependenciesList(ctx, conf)
			if err != nil {
				//nolint:gocritic // Why: need to get exit code >0
				log.Fatal().Err(err).Msg("Failed to build dependency tree")
				return
			}

			// TODO(jaredallard): outreach specific code
			target := "base"
			for _, d := range deps {
				if d == "outreach" {
					target = flagship
					break
				}
			}

			log.Info().Strs("deps", deps).Str("target", target).Msg("Provisioning devenv")

			if err := provisionNew(ctx, target); err != nil {
				//nolint:gocritic // Why: need to get exit code >0
				log.Fatal().Err(err).Msg("Failed to create cluster")
			}

			if !dockerBuilt {
				log.Info().Msg("Waiting for docker build to finish")
			}

			wg.Wait() // To ensure that docker build is finished
		} else {
			log.Info().
				//nolint:lll // Why: Message to user
				Msg("Re-using existing cluster, this may lead to a non-reproducible failure/success. To ensure a clean operation, run `devenv destroy` before running tests")
		}
	}

	dc, err := config.FromFile("devenv.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse devenv.yaml, cannot run e2e tests for this repo")
	}

	var wg sync.WaitGroup
	requireDevconfigAfterDeploy := os.Getenv("REQUIRE_DEVCONFIG_AFTER_DEPLOY") == "true"

	if !requireDevconfigAfterDeploy {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			log.Info().Msg("Running devconfig in background")
			runDevconfig(ctx)
			log.Info().Msg("Running devconfig in background finished")
		}(&wg)
	}

	skipAppDeployment := os.Getenv("SKIP_APP_DEPLOYMENT") == "true"

	// if it's a library we don't need to deploy the application.
	if dc.Service {
		if skipAppDeployment {
			log.Info().Msg("Skipping application deployment since SKIP_APP_DEPLOYMENT is true")	
		}
		else{
			log.Info().Msg("Deploying current application into cluster")
			if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "apps", "deploy", "--with-deps", ".")).Run(); err != nil {
				log.Fatal().Err(err).Msg("Failed to deploy current application into devenv")
			}
		}
	}

	if requireDevconfigAfterDeploy {
		log.Info().Msg("Running devconfig")
		runDevconfig(ctx)
	} else {
		wg.Wait() // Ensure that devconfig is done
	}

	// If the post-deploy script for e2e exists, run it.
	if _, err := os.Stat("scripts/devenv/post-e2e-deploy.sh"); err == nil {
		log.Info().Msg("Running scripts/devenv/post-e2e-deploy.sh")

		if err := osStdInOutErr(exec.CommandContext(ctx, "scripts/devenv/post-e2e-deploy.sh")).Run(); err != nil {
			log.Fatal().Err(err).Msg("Failed to run scripts/devenv/post-e2e-deploy.sh")
		}
	}

	// Allow users to opt out of running localizer
	if os.Getenv("SKIP_LOCALIZER") != "true" {
		closer, err := runLocalizer(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to run localizer")
		}
		defer closer()
	}

	log.Info().Msg("Running e2e tests")
	os.Setenv("TEST_TAGS", "or_test,or_e2e")
	if err := osStdInOutErr(exec.CommandContext(ctx, "./.bootstrap/shell/test.sh")).Run(); err != nil {
		log.Fatal().Err(err).Msg("E2E tests failed, or failed to run")
	}
}
