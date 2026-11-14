// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Signs checksums.txt with a keyless Sigstore signature
// from CircleCI, for releases of public repositories.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
)

// These are the values of releaseOptions.signing in service.yaml.
const (
	// signingNone signs nothing. It is the default.
	signingNone = "none"

	// signingKeyless signs checksums.txt with cosign and a CircleCI OIDC
	// token.
	signingKeyless = "keyless"
)

// bundleSuffix is added to the checksum file name to name its Sigstore
// bundle.
const bundleSuffix = ".sigstore.json"

// sigstoreTokenEnv is the variable that cosign reads an OIDC token from.
// The token goes to cosign as a file, but the masker finds secrets by
// variable name, so the masker gets the token under this name.
const sigstoreTokenEnv = "SIGSTORE_ID_TOKEN" //nolint:gosec // Why: a variable name, not a credential.

// These are the errors of the signing.
var (
	// ErrSigning is returned for a signing setting that the release
	// cannot do.
	ErrSigning = errors.New("unsupported releaseOptions.signing")

	// ErrNotPublic is returned for keyless signing in a repository that
	// is not public, because the Sigstore logs are public.
	ErrNotPublic = errors.New("keyless signing needs a public repository")

	// errNoOIDCToken is returned when the CircleCI OIDC token is empty.
	errNoOIDCToken = errors.New("empty CircleCI OIDC token")
)

// checkSigning reads releaseOptions.signing from service.yaml. For
// keyless signing, it checks that the release has a checksum file and a
// forge release for the bundle, and that the forge reports a public
// repository: keyless certificates and Rekor entries are public and
// permanent, and they name the repository.
func (p *publisher) checkSigning(ctx context.Context) error {
	ro, err := loadServiceOptions(p.r.Git.Dir)
	if err != nil {
		return err
	}
	switch ro.Signing {
	case "", signingNone:
		return nil
	case signingKeyless:
	default:
		return fmt.Errorf("%w: %q, use %q or %q", ErrSigning, ro.Signing, signingNone, signingKeyless)
	}
	if p.r.Config.Artifacts == nil || p.r.Config.Forge() == nil {
		return fmt.Errorf("%w: %q needs checksums and a forge release", ErrSigning, signingKeyless)
	}
	visibility, err := p.opts.Forge.Visibility(ctx)
	if err != nil {
		return fmt.Errorf("check the repository visibility: %w", err)
	}
	if visibility != forge.Public {
		return fmt.Errorf("%w: the repository is %s", ErrNotPublic, visibility)
	}
	p.keyless = true
	return nil
}

// sign signs the checksum file with cosign and a CircleCI OIDC token,
// and writes the Sigstore bundle next to it, for the forge release. It
// runs before the tag push, so a Sigstore failure stops the release
// before git changes. A rerun signs again only when the release of an
// earlier run is missing or is a draft without a complete bundle.
func (p *publisher) sign(ctx context.Context) error {
	if !p.keyless {
		return nil
	}
	name := p.r.Config.Artifacts.Checksums
	sums := filepath.Join(p.r.Git.Dir, p.opts.Dist, name)
	bundle := sums + bundleSuffix
	if p.resume {
		rel, err := p.opts.Forge.FindRelease(ctx, p.plan.NextRelease.GitTag)
		if err != nil {
			return err
		}
		if why := keptSignature(rel, name+bundleSuffix); why != "" {
			fmt.Fprintf(p.log, "Skip signing %s, as %s\n", name, why)
			return nil
		}
	}
	token, err := p.oidcToken(ctx)
	if err != nil {
		return err
	}
	// The masker finds the token in an environment, as for the
	// CodeArtifact token.
	ctx = rlog.WithMasker(ctx, rlog.NewMasker(append(slices.Clip(envOrProcess(p.opts.Env)), sigstoreTokenEnv+"="+token)))
	dir, err := os.MkdirTemp("", "devbase-sign-")
	if err != nil {
		return fmt.Errorf("make the folder of the OIDC token: %w", err)
	}
	defer os.RemoveAll(dir)
	// cosign takes a token, or a file with the token, in --identity-token.
	// A file keeps the token out of the command line.
	tokenFile := filepath.Join(dir, "oidc-token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		return fmt.Errorf("write the OIDC token: %w", err)
	}
	fmt.Fprintf(p.log, "Sign %s with a keyless Sigstore signature\n", name)
	if _, err := execCommand(ctx, p.opts.Exec, p.log, &command{
		dir: p.r.Git.Dir, env: p.opts.Env, name: "cosign",
		args: []string{
			"sign-blob", "--yes", "--new-bundle-format", "--bundle", bundle, "--identity-token", tokenFile, sums,
		},
	}); err != nil {
		return fmt.Errorf("sign %s: %w", name, err)
	}
	p.bundle = bundle
	return nil
}

// keptSignature returns why a rerun keeps the signature of the release
// rel of an earlier run, or "" when it signs again: a published release
// does not change, and a draft keeps a complete bundle.
func keptSignature(rel *forge.Release, bundle string) string {
	switch {
	case rel == nil:
		return ""
	case !rel.Draft:
		return "the release " + rel.Tag + " is published"
	case slices.ContainsFunc(rel.Assets, func(a forge.Asset) bool { return a.Name == bundle && a.Complete }):
		return "the release " + rel.Tag + " has " + bundle
	}
	return ""
}

// oidcToken returns a CircleCI OIDC token for Sigstore: its audience is
// sigstore, and its issuer is https://oidc.circleci.com, which Fulcio
// accepts. The command logs nothing, because its stdout is the token.
func (p *publisher) oidcToken(ctx context.Context) (string, error) {
	fmt.Fprintln(p.log, "Get a CircleCI OIDC token for Sigstore")
	res, err := execCommand(ctx, p.opts.Exec, io.Discard, &command{
		dir: p.r.Git.Dir, env: p.opts.Env, name: "circleci",
		args: []string{"run", "oidc", "get", "--aud", "sigstore", "--root-issuer"},
	})
	if err != nil {
		return "", fmt.Errorf("get the CircleCI OIDC token: %w", err)
	}
	token := strings.TrimSpace(res.stdout)
	if token == "" {
		return "", errNoOIDCToken
	}
	return token, nil
}
