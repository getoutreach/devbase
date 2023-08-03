// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: This is the entrypoint of the e2e runner for the devenv.

// TODO(george-e-shaw-iv): Remove all calls to log.Fatal with graceful exits in mind.

package main

import (
	"context"
	"encoding/xml"
	"fmt"
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

// junitTestResultPath path to test results after we run (devenv apps e2e)
const junitTestResultPath = "./bin/unit-tests.xml"

// devenvAlreadyExists contains message when devenv exists
const devenvAlreadyExists = "Re-using existing cluster, this may lead to a non-reproducible failure/success. " +
	"To ensure a clean operation, run `devenv destroy` before running tests"

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
func runDevconfig(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "./scripts/shell-wrapper.sh", "devconfig.sh").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
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

// runE2ETestsUsingDevspace uses devspace and binary sync to deploy application. There's no devconfig and docker build.
func runE2ETestsUsingDevspace(ctx context.Context, conf *box.Config) error {
	if isDevenvProvisioned(ctx) {
		log.Info().Msgf(devenvAlreadyExists)
	} else {
		err := provisionDevenv(ctx, conf)
		if err != nil {
			return err
		}
	}

	serviceName, err := config.ReadServiceName()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		log.Info().Msg("Building binaries for devspace pod")
		if err := osStdInOutErr(exec.CommandContext(ctx, "make", "devspace")).Run(); err != nil {
			log.Error().Err(err).Msg("Error when building for devspace")
			panic(err)
		}
	}()

	log.Info().Msgf("Deploying latest stable version of %s application into cluster together with dependencies", serviceName)
	if err := osStdInOutErr(exec.CommandContext(
		ctx, "devenv", "--skip-update", "apps", "deploy", "--with-deps", serviceName)).Run(); err != nil {
		return errors.Wrapf(err, "Failed to deploy %s into devenv", serviceName)
	}

	wg.Wait()

	log.Info().Msg("Starting devspace pod and running e2e tests")
	if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "apps", "e2e", "--sync-binaries", ".")).Run(); err != nil {
		return errors.Wrapf(err, "Failed to deploy %s into devenv", serviceName)
	}
	if runningInCi() {
		// Copy junit report to place where CircleCi expects it
		if err := osStdInOutErr(exec.CommandContext(ctx, "cp", junitTestResultPath, "/tmp/test-results/")).Run(); err != nil {
			return errors.Wrap(err, "Unable to copy tests results to CircleCI artifact path")
		}
	}
	testsSuccess, err := parseResultFromJunitReport()
	if err != nil {
		return err
	}
	if !testsSuccess {
		return errors.New("E2E Tests failed")
	}
	log.Info().Msg("E2E Tests succeeded.")
	return nil
}

// parseResultFromJunitReport parses if tests succeeded from junit xml file
func parseResultFromJunitReport() (bool, error) {
	type Testsuite struct {
		XMLName  xml.Name `xml:"testsuites"`
		Failures int      `xml:"failures,attr"`
	}

	data, err := os.ReadFile(junitTestResultPath)
	if err != nil {
		return false, errors.Wrap(err, "Unable to find e2e tests results")
	}

	var testsuite Testsuite
	err = xml.Unmarshal(data, &testsuite)
	if err != nil {
		return false, errors.Wrap(err, "Unable to parse junit e2e tests results")
	}

	return testsuite.Failures == 0, nil
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
		if runningInCi() {
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

	// USE_DEVSPACE env var is used to onboard in cluster run of e2e tests using devspace
	useDevspace := os.Getenv("USE_DEVSPACE") == "true" //nolint:goconst // Why: true == true
	if useDevspace {
		err := runE2ETestsUsingDevspace(ctx, conf)
		if err != nil {
			log.Fatal().Err(err).Msgf("Error in running e2e tests using devspace")
		}
		return
	}

	log.Info().Msg("Building dependency tree")

	// Provision a devenv if it doesn't already exist. If it does exist,
	// warn the user their test is no longer potentially reproducible.
	// Allow skipping provision, this is generally only useful for the devenv
	// which uses this framework -- but provisions itself.
	if os.Getenv("SKIP_DEVENV_PROVISION") != "true" {
		if !isDevenvProvisioned(ctx) {
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

			err := provisionDevenv(ctx, conf)
			if err != nil {
				//nolint:gocritic // Why: need to get exit code >0
				log.Fatal().Err(err).Msg("Failed to provision devenv")
				return
			}

			if !dockerBuilt {
				log.Info().Msg("Waiting for docker build to finish")
			}

			wg.Wait() // To ensure that docker build is finished
		} else {
			log.Info().
				//nolint:lll // Why: Message to user
				Msg(devenvAlreadyExists)
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
			if err := runDevconfig(ctx); err != nil {
				// Call cancel to hopefully communicate a signal back to other currently running commands to stop
				// doing what they're doing. If we just exit (implicitly via log.Fatal) they likely will continue
				// running.
				cancel()

				log.Fatal().Err(err).Msg("failed to run devconfig")
			}
			log.Info().Msg("Running devconfig in background finished")
		}(&wg)
	}

	if dc.Service {
		log.Info().Msg("Deploying current application into cluster")
		if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "apps", "deploy", "--with-deps", ".")).Run(); err != nil {
			log.Fatal().Err(err).Msg("Failed to deploy current application into devenv")
		}
	} else {
		// we want to build CLI application so that E2E tests can invoke it
		log.Info().Msg("Building application")
		if err := exec.CommandContext(ctx, "make", "build").Run(); err != nil {
			log.Fatal().Err(err).Msg("Error building application")
		} else {
			log.Info().Msg("Build done")
		}
	}

	if requireDevconfigAfterDeploy {
		log.Info().Msg("Running devconfig")
		if err := runDevconfig(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to run devconfig")
		}
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

// provisionDevenv provisions devenv in correct target based on application dependencies
func provisionDevenv(ctx context.Context, conf *box.Config) error {
	deps, err := BuildDependenciesList(ctx, conf)
	if err != nil {
		return errors.Wrap(err, "Failed to build dependency tree")
	}

	// TODO(jaredallard): outreach specific code
	target := "base"
	if os.Getenv("PROVISION_TARGET") != "" {
		target = os.Getenv("PROVISION_TARGET")
	} else {
		for _, d := range deps {
			if d == "outreach" {
				target = flagship
				break
			}
		}
	}

	log.Info().Strs("deps", deps).Str("target", target).Msg("Provisioning devenv")

	if err := provisionNew(ctx, target); err != nil {
		return errors.Wrap(err, "Failed to create cluster")
	}
	return nil
}

func isDevenvProvisioned(ctx context.Context) bool {
	return exec.CommandContext(ctx, "devenv", "--skip-update", "status").Run() == nil
}

func runningInCi() bool {
	return os.Getenv("CI") == "true" //nolint:goconst // Why: true == true
}
