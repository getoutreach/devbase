// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The Python client plugin: a Poetry package that the
// release publishes to an AWS CodeArtifact PyPI repository, as the
// stencil Python client hooks do.

package spec

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// DefaultPythonDir is the package folder of a Python plugin without a
// Dir: the Python client folder that stencil renders.
const DefaultPythonDir = "api/clients/python"

// These are the name rules of the CodeArtifact API reference
// (DescribePackageVersion), and an AWS region name.
var (
	// codeArtifactDomainRegex matches a CodeArtifact domain name.
	codeArtifactDomainRegex = regexp.MustCompile(`^[a-z][a-z0-9\-]{0,48}[a-z0-9]$`)

	// codeArtifactRepositoryRegex matches a CodeArtifact repository name.
	codeArtifactRepositoryRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{1,99}$`)

	// awsAccountRegex matches an AWS account ID.
	awsAccountRegex = regexp.MustCompile(`^\d{12}$`)

	// awsRegionRegex matches an AWS region name, such as eu-west-1.
	awsRegionRegex = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d+$`)
)

// PythonPlugin publishes a Poetry package to an AWS CodeArtifact PyPI
// repository. Domain, Region and Repository are required.
type PythonPlugin struct {
	// Dir is the package folder in the repository, with pyproject.toml.
	// Empty means DefaultPythonDir.
	Dir string

	// Domain is the CodeArtifact domain.
	Domain string

	// Account is the 12-digit AWS account that owns the domain. Empty
	// means aws.defaultAccountID of the box config, as in the task.
	Account string

	// Region is the AWS region of the domain.
	Region string

	// Repository is the CodeArtifact PyPI repository.
	Repository string
}

// WithDefaults returns p with DefaultPythonDir when it has no Dir. The
// publisher reads an empty Account from the box config.
func (p *PythonPlugin) WithDefaults() PythonPlugin {
	d := *p
	if d.Dir == "" {
		d.Dir = DefaultPythonDir
	}
	return d
}

// validate checks the plugin with its defaults: the package folder is
// in the repository, the CodeArtifact repository is set, and the names
// follow the API rules.
func (p *PythonPlugin) validate() error {
	d := p.WithDefaults()
	switch {
	case d.Domain == "" || d.Repository == "" || d.Region == "":
		return fmt.Errorf("%w: the Python plugin needs the CodeArtifact domain, repository and region", ErrUnsupported)
	case !isLocalPath(d.Dir):
		return fmt.Errorf("%w: Python package folder %q is not a clean path in the repository", ErrUnsupported, d.Dir)
	case !codeArtifactDomainRegex.MatchString(d.Domain):
		return fmt.Errorf("%w: CodeArtifact domain %q", ErrUnsupported, d.Domain)
	case !codeArtifactRepositoryRegex.MatchString(d.Repository):
		return fmt.Errorf("%w: CodeArtifact repository %q", ErrUnsupported, d.Repository)
	case !awsRegionRegex.MatchString(d.Region):
		return fmt.Errorf("%w: AWS region %q", ErrUnsupported, d.Region)
	case d.Account != "" && !awsAccountRegex.MatchString(d.Account):
		return fmt.Errorf("%w: AWS account %q is not 12 digits", ErrUnsupported, d.Account)
	}
	return nil
}

// validatePythonBranches checks that every prerelease version of the
// branches has a PEP 440 form, when the config publishes a Python
// package.
func (c *Config) validatePythonBranches() error {
	if !slices.ContainsFunc(c.Plugins, func(p Plugin) bool { return p.Python != nil }) {
		return nil
	}
	for i := range c.Branches {
		b := &c.Branches[i]
		if !b.IsPrerelease() {
			continue
		}
		if _, err := PEP440Version("1.0.0-" + b.Prerelease + ".1"); err != nil {
			return fmt.Errorf("branch %q and the Python package: %w", b.Name, err)
		}
	}
	return nil
}

// pep440Prereleases maps a prerelease identifier, in lowercase, to its
// normalized PEP 440 segment. PEP 440 accepts the other spellings
// ("alpha", "beta", "c", "pre", "preview"). A post-release is left out,
// because it sorts after its release, while a semver prerelease sorts
// before it.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var pep440Prereleases = map[string]string{
	"a": "a", "alpha": "a",
	"b": "b", "beta": "b",
	"c": "rc", "rc": "rc", "pre": "rc", "preview": "rc",
	"dev": ".dev",
}

// PEP440Version returns the normalized PEP 440 form of a release
// version, which is what "poetry version <version>" writes to
// pyproject.toml and what the package files and the registry use:
// "1.2.0" stays "1.2.0", "1.2.0-rc.1" becomes "1.2.0rc1", "1.2.0-beta.2"
// becomes "1.2.0b2", and "1.2.0-dev.3" becomes "1.2.0.dev3". A
// prerelease that is not "<identifier>.<number>" with a known
// identifier, and build metadata, are errors.
func PEP440Version(version string) (string, error) {
	v, err := semver.StrictNewVersion(version)
	if err != nil || v.Metadata() != "" {
		return "", fmt.Errorf("%w: version %q has no PEP 440 form", ErrUnsupported, version)
	}
	out := fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch())
	if v.Prerelease() == "" {
		return out, nil
	}
	id, num, _ := strings.Cut(v.Prerelease(), ".")
	segment, ok := pep440Prereleases[strings.ToLower(id)]
	n, err := strconv.ParseUint(num, 10, 64)
	if !ok || err != nil {
		return "", fmt.Errorf("%w: version %q has no PEP 440 form", ErrUnsupported, version)
	}
	return out + segment + strconv.FormatUint(n, 10), nil
}
