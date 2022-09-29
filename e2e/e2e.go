package main

import (
	"context"
	"encoding/json"
	"go/build"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/getoutreach/gobox/pkg/async"
	"github.com/getoutreach/gobox/pkg/box"
	"github.com/getoutreach/gobox/pkg/sshhelper"
	localizerapi "github.com/getoutreach/localizer/api"
	"github.com/getoutreach/localizer/pkg/localizer"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v2"
)

// flagship is the name of the flagship
const flagship = "flagship"

var virtualDeps = map[string][]string{
	// TODO(jaredallard): [DT-510] Store flagship dependencies in the outreach repository
	// This will be removed once reactor is dead.
	"outreach": {
		"outreach-templating-service",
		"olis",
		"mint",
		"giraffe",
		"outreach-accounts",
		"clientron",
	},
}

// DevenvConfig is a struct that contains the devenv configuration
// which is usually called "devenv.yaml"
type DevenvConfig struct {
	// Service denotes if this repository is a service.
	Service bool `yaml:"service"`

	Dependencies struct {
		// Optional is a list of OPTIONAL services e.g. the service can run / gracefully function without it running
		Optional []string `yaml:"optional"`

		// Required is a list of services that this service cannot function without
		Required []string `yaml:"required"`
	} `yaml:"dependencies"`
}

// osStdinOut is a helper function to use the os stdin/out/err
func osStdInOutErr(c *exec.Cmd) *exec.Cmd {
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// BuildDependenciesList builds a list of dependencies by cloning them
// and appending them to the list. Deduplication is done and returned
// is a flat list of all of the dependencies of the initial root
// application who's dependency list was provided.
func BuildDependenciesList(ctx context.Context) ([]string, error) {
	deps := make(map[string]bool)

	a := sshhelper.GetSSHAgent()
	if _, err := sshhelper.LoadDefaultKey("github.com", a, logrus.StandardLogger()); err != nil {
		return nil, err
	}

	auth := sshhelper.NewExistingSSHAgentCallback(a)

	s, err := parseDevenvConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse service.yaml")
	}

	for _, d := range append(s.Dependencies.Required, s.Dependencies.Optional...) {
		// Error on flagship dependency. This can be removed later as this was a breaking change
		// and is just a nice to have. We only error only on our top-level dependencies. Later on
		// we map flagship -> outreach for sub-level deps.
		if d == flagship {
			log.Fatal().Msg("flagship has been replaced by outreach, please update your dependency list")
		}

		if err := grabDependencies(ctx, deps, d, auth); err != nil {
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
// is a hash map used to prevent infinite recursion and de-duplicate
// dependencies. New dependencies are inserted into the provided hash-map
func grabDependencies(ctx context.Context, deps map[string]bool, name string, auth *sshhelper.ExistingSSHAgentCallback) error {
	// We special case this here to ensure we don't fail on deps that haven't updated
	// their dependency yet.
	if name == flagship {
		name = "outreach"
	}

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

		var dc *DevenvConfig
		if err := yaml.NewDecoder(f).Decode(&dc); err != nil {
			return errors.Wrapf(err, "failed to parse service.yaml in dependency %s", name)
		}

		//nolint:gocritic // Why: done on purpose
		foundDeps = append(dc.Dependencies.Required, dc.Dependencies.Optional...)
	} else {
		log.Info().Msgf("Using baked-in dependency list")
		foundDeps = virtualDeps[name]
	}

	// Mark us as resolved to prevent inf dependency resolution
	// when we encounter cyclical dependency.
	deps[name] = true

	for _, d := range foundDeps {
		err := grabDependencies(ctx, deps, d, auth)
		if err != nil {
			return err
		}
	}

	return nil
}

// parseDevenvConfig parses the devenv.yaml file and returns a struct
func parseDevenvConfig() (*DevenvConfig, error) {
	f, err := os.Open("devenv.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read devenv.yaml or service.yaml")
	}
	defer f.Close()

	var dc DevenvConfig
	if err = yaml.NewDecoder(f).Decode(&dc); err != nil {
		return nil, errors.Wrapf(err, "failed to parse devenv.yaml or service.yaml")
	}

	return &dc, nil
}

// appAlreadyDeployed checks if an application is already deployed, if it is
// it returns true, otherwise false.
func appAlreadyDeployed(ctx context.Context, app string) bool {
	var deployedApps []struct {
		Name string `json:"name"`
	}

	b, err := exec.CommandContext(ctx, "devenv", "apps", "list", "--output", "json").Output()
	if err != nil {
		return false
	}

	if err := json.Unmarshal(b, &deployedApps); err != nil {
		return false
	}

	for _, a := range deployedApps {
		if a.Name == app {
			return true
		}
	}

	return false
}

//nolint:unparam // Why: keeping in the interface for now
func provisionNew(ctx context.Context, deps []string, target string) error {
	//nolint:errcheck // Why: Best effort remove existing cluster
	exec.CommandContext(ctx, "devenv", "--skip-update", "destroy").Run()

	if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update",
		"provision", "--snapshot-target", target)).Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to provision devenv")
	}

	for _, d := range deps {
		// Skip applications that are already deployed, this is usually when
		// they're in a snapshot we just provisioned from.
		if appAlreadyDeployed(ctx, d) {
			log.Info().Msgf("App %s already deployed, skipping", d)
			continue
		}

		log.Info().Msgf("Deploying dependency '%s'", d)
		if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "apps", "deploy", d)).Run(); err != nil {
			log.Fatal().Err(err).Msgf("Failed to deploy dependency '%s'", d)
		}
	}

	return nil
}

// ensureRunningLocalizerWorks check if a localizer is already running, and if it is
// ensure it's working properly (responding to pings). If it's not, remove the socket.
func ensureRunningLocalizerWorks(ctx context.Context) error {
	log.Info().Msg("Ensuring existing localizer is actually running")
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	client, closer, err := localizer.Connect(ctx, grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Made connection, ping it
	if err == nil {
		defer closer()

		// Responding to pings, return nil
		if _, err := client.Ping(ctx, &localizerapi.PingRequest{}); err == nil {
			return nil
		}
	}

	// not responding to pings, or failed to connect, remove the socket
	//nolint:gosec // Why: We're OK with this. It's a constant.
	return osStdInOutErr(exec.Command("sudo", "rm", "-f", localizer.Socket)).Run()
}

func runLocalizer(ctx context.Context) (cleanup func(), err error) {
	if localizer.IsRunning() {
		if err := ensureRunningLocalizerWorks(ctx); err != nil {
			return nil, err
		}
	}

	if !localizer.IsRunning() {
		// Preemptively ask for sudo to prevent input mangling with o.LocalApps
		log.Info().Msg("You may get a sudo prompt so localizer can create tunnels")
		if err := osStdInOutErr(exec.CommandContext(ctx, "sudo", "true")).Run(); err != nil {
			log.Fatal().Err(err).Msg("Failed to get root permissions")
		}

		log.Info().Msg("Starting devenv tunnel")
		if err := osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "tunnel")).Start(); err != nil {
			log.Fatal().Err(err).Msg("Failed to start devenv tunnel")
		}

		// Wait until localizer is running, max 1m
		//nolint:govet // Why: done on purpose
		ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Minute))
		defer cancel()

		for ctx.Err() == nil && !localizer.IsRunning() {
			async.Sleep(ctx, time.Second*1)
		}
	}

	client, closer, err := localizer.Connect(ctx, grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to localizer")
	}
	defer closer()

	log.Info().Msg("Waiting for devenv tunnel to be finished creating tunnels")
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Minute))
	defer cancel()

	for ctx.Err() == nil {
		resp, err := client.Stable(ctx, &localizerapi.Empty{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to check if localizer is running")
		}

		if resp.Stable {
			break
		}

		async.Sleep(ctx, time.Second*2)
	}

	return func() {
		log.Info().Msg("Killing the spawned localizer process (spawned by devenv tunnel)")
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := client.Kill(ctx, &localizerapi.Empty{}); err != nil {
			log.Warn().Err(err).Msg("failed to kill running localizer server")
		}
	}, nil
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

func main() { //nolint:funlen,gocyclo
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	deps, err := BuildDependenciesList(ctx)
	if err != nil {
		//nolint:gocritic // Why: need to get exit code >0
		log.Fatal().Err(err).Msg("Failed to build dependency tree")
		return
	}

	log.Info().Strs("deps", deps).Msg("Provisioning devenv")

	// TODO(jaredallard): outreach specific code
	target := "base"
	for _, d := range deps {
		if d == "outreach" {
			target = flagship
			break
		}
	}

	// Provision a devenv if it doesn't already exist. If it does exist,
	// warn the user their test is no longer potentially reproducible.
	// Allow skipping provision, this is generally only useful for the devenv
	// which uses this framework -- but provisions itself.
	if os.Getenv("SKIP_DEVENV_PROVISION") != "true" {
		if exec.CommandContext(ctx, "devenv", "--skip-update", "status").Run() != nil {
			if err := provisionNew(ctx, deps, target); err != nil {
				//nolint:gocritic // Why: need to get exit code >0
				log.Fatal().Err(err).Msg("Failed to create cluster")
			}
		} else {
			log.Info().
				//nolint:lll // Why: Message to user
				Msg("Re-using existing cluster, this may lead to a non-reproducible failure/success. To ensure a clean operation, run `devenv destroy` before running tests")
		}
	}

	dc, err := parseDevenvConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse service.yaml file")
	}

	// if it's a library we don't need to deploy the application.
	if dc.Service {
		log.Info().Msg("Deploying current application into cluster")
		if osStdInOutErr(exec.CommandContext(ctx, "devenv", "--skip-update", "apps", "deploy", ".")).Run() != nil {
			log.Fatal().Err(err).Msg("Failed to deploy current application into devenv")
		}
	}

	log.Info().Msg("Running devconfig")
	if err := osStdInOutErr(exec.CommandContext(ctx, ".bootstrap/shell/devconfig.sh")).Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to run devconfig")
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
