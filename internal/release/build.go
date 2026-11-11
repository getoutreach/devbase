// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Builds Go binaries, tar.gz archives and checksums.txt
// for the targets of a release plan.

package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
)

// ChecksumsFile is the name of the checksum file.
const ChecksumsFile = "checksums.txt"

// ArchiveName returns the GoReleaser default archive name. GoReleaser
// leaves out the default amd64 level (v1) and arm64 has no suffix.
func ArchiveName(project, version string, t goreleaser.Target) string {
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", project, version, t.OS, t.Arch)
}

// ErrUnknownTarget is returned for a target that the plan does not have.
var ErrUnknownTarget = errors.New("target is not in the plan")

// BuildOptions are the inputs of Build.
type BuildOptions struct {
	// Dir is the repository directory.
	Dir string

	// Dist is the output directory, relative to Dir.
	Dist string

	// Targets are the "os/arch" targets to build. Empty means all
	// targets in the plan.
	Targets []string

	// Env is the environment for templates and builds.
	Env []string

	// SkipHooks skips the before hooks.
	SkipHooks bool
}

// Build runs the before hooks, then builds and archives the targets of
// a plan in parallel. Each target writes to <dist>/<os>_<arch>/, so
// parallel CI jobs never write the same file.
func Build(ctx context.Context, cfg *goreleaser.Config, plan *Plan, opts *BuildOptions) ([]string, error) {
	targets, err := selectTargets(cfg, plan, opts.Targets)
	if err != nil {
		return nil, err
	}
	if !opts.SkipHooks {
		for _, hook := range cfg.Before.Hooks {
			args := strings.Fields(hook)
			if _, err := run(ctx, opts.Dir, opts.Env, args[0], args[1:]...); err != nil {
				return nil, fmt.Errorf("before hook %q: %w", hook, err)
			}
		}
	}

	ldflags, err := renderLDFlags(cfg, plan.NextRelease.Version, opts.Env)
	if err != nil {
		return nil, err
	}
	extras, err := archiveExtras(cfg, opts.Dir)
	if err != nil {
		return nil, err
	}
	modTime := time.Unix(plan.CommitTime, 0)

	archives := make([]string, len(targets))
	errs := make([]error, len(targets))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			outDir := filepath.Join(opts.Dir, opts.Dist, t.OS+"_"+t.Arch)
			entries := slices.Clone(extras)
			for j := range cfg.Builds {
				b := &cfg.Builds[j]
				bin := filepath.Join(outDir, b.ID, b.Binary)
				env := slices.Concat(opts.Env, b.Env, []string{"GOOS=" + t.OS, "GOARCH=" + t.Arch})
				if _, err := run(ctx, opts.Dir, env, "go", "build", "-ldflags="+ldflags[j], "-o", bin, b.Main); err != nil {
					errs[i] = fmt.Errorf("build %q for %s: %w", b.ID, t, err)
					return
				}
				entries = append(entries, archiveEntry{src: bin, dst: b.Binary, mode: 0o755})
			}
			archives[i] = filepath.Join(outDir, ArchiveName(plan.ProjectName, plan.NextRelease.Version, t))
			errs[i] = writeArchive(archives[i], entries, modTime)
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return archives, nil
}

// WriteChecksums writes checksums.txt for the archives, sorted by file
// name, with GoReleaser's "<sha256>  <name>" line format.
func WriteChecksums(file string, archives []string) error {
	sorted := slices.Clone(archives)
	slices.SortFunc(sorted, func(a, b string) int { return strings.Compare(filepath.Base(a), filepath.Base(b)) })
	var buf bytes.Buffer
	for _, a := range sorted {
		sum, err := sha256File(a)
		if err != nil {
			return err
		}
		fmt.Fprintf(&buf, "%s  %s\n", sum, filepath.Base(a))
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

// selectTargets returns the requested targets, or all plan targets.
func selectTargets(cfg *goreleaser.Config, plan *Plan, requested []string) ([]goreleaser.Target, error) {
	for _, r := range requested {
		if !slices.Contains(plan.Targets, r) {
			return nil, fmt.Errorf("%w: %s", ErrUnknownTarget, r)
		}
	}
	var out []goreleaser.Target
	for _, t := range cfg.Targets() {
		if slices.Contains(plan.Targets, t.String()) && (len(requested) == 0 || slices.Contains(requested, t.String())) {
			out = append(out, t)
		}
	}
	return out, nil
}

// renderLDFlags renders the ldflags of each build once. They do not
// depend on the target.
func renderLDFlags(cfg *goreleaser.Config, version string, env []string) ([]string, error) {
	envMap := map[string]string{}
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}
	data := map[string]any{"Version": version, "Tag": "v" + version, "Env": envMap}
	out := make([]string, len(cfg.Builds))
	for i := range cfg.Builds {
		flags := make([]string, len(cfg.Builds[i].LDFlags))
		for j, f := range cfg.Builds[i].LDFlags {
			t, err := template.New("").Option("missingkey=error").Parse(f)
			if err != nil {
				return nil, fmt.Errorf("build %q ldflags %q: %w", cfg.Builds[i].ID, f, err)
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, data); err != nil {
				return nil, fmt.Errorf("build %q ldflags %q: %w", cfg.Builds[i].ID, f, err)
			}
			flags[j] = buf.String()
		}
		out[i] = strings.Join(flags, " ")
	}
	return out, nil
}

// archiveEntry is one file in an archive.
type archiveEntry struct {
	src, dst string
	mode     os.FileMode
}

// archiveExtras resolves the extra archive files once, sorted by
// destination. An exact src with a dst uses dst as the name; a glob
// with a dst puts the matches in that folder.
func archiveExtras(cfg *goreleaser.Config, dir string) ([]archiveEntry, error) {
	var out []archiveEntry
	for _, f := range cfg.ArchiveFiles() {
		matches, err := filepath.Glob(filepath.Join(dir, f.Src))
		if err != nil {
			return nil, fmt.Errorf("archive file %q: %w", f.Src, err)
		}
		for _, m := range matches {
			dst, err := filepath.Rel(dir, m)
			if err != nil {
				return nil, fmt.Errorf("archive file %q: %w", m, err)
			}
			if f.Dst != "" {
				dst = f.Dst
				if strings.ContainsAny(f.Src, "*?[") {
					dst = filepath.Join(f.Dst, filepath.Base(m))
				}
			}
			out = append(out, archiveEntry{src: m, dst: dst, mode: f.Info.Mode})
		}
	}
	slices.SortFunc(out, func(a, b archiveEntry) int { return strings.Compare(a.dst, b.dst) })
	return out, nil
}

// writeArchive writes a tar.gz with the entries in order.
func writeArchive(file string, entries []archiveEntry, modTime time.Time) error {
	out, err := os.Create(file)
	if err != nil {
		return fmt.Errorf("create %s: %w", file, err)
	}
	defer out.Close()
	gz, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("gzip %s: %w", file, err)
	}
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if err := addTarFile(tw, e, modTime); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar %s: %w", file, err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip %s: %w", file, err)
	}
	return out.Close()
}

// addTarFile adds one file to a tar archive.
func addTarFile(tw *tar.Writer, e archiveEntry, modTime time.Time) error {
	f, err := os.Open(e.src)
	if err != nil {
		return fmt.Errorf("open %s: %w", e.src, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", e.src, err)
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("tar header %s: %w", e.src, err)
	}
	hdr.Name = filepath.ToSlash(e.dst)
	hdr.ModTime = modTime
	hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
	if e.mode != 0 {
		hdr.Mode = int64(e.mode)
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar header %s: %w", e.dst, err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("tar %s: %w", e.dst, err)
	}
	return nil
}

// sha256File returns the hex sha256 of a file, read as a stream.
func sha256File(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
