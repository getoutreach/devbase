// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Builds Go binaries, tar.gz archives and checksums.txt
// for the targets of a release plan.

package release

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// ArchiveName returns the GoReleaser default archive name.
func ArchiveName(project, version string, t *spec.Target) string {
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", project, version, t.OS, t.ArchiveArch())
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
func Build(ctx context.Context, arts *spec.Artifacts, plan *Plan, opts *BuildOptions) (_ []string, err error) {
	ctx, end := startStep(ctx, "build")
	defer end(&err)
	targets, err := selectTargets(arts, plan, opts.Targets)
	if err != nil {
		return nil, err
	}
	tmpl := &spec.Template{Version: plan.NextRelease.Version, Tag: spec.RootTagFormat.Render(plan.NextRelease.Version)}
	if !opts.SkipHooks {
		for _, hook := range arts.Hooks {
			if err := runHook(ctx, hook, tmpl, opts); err != nil {
				return nil, fmt.Errorf("before hook %q: %w", hook, err)
			}
		}
	}
	extras, err := archiveExtras(arts, tmpl, opts.Env, opts.Dir)
	if err != nil {
		return nil, err
	}
	bd := &builder{
		arts: arts, opts: opts, tmpl: tmpl, extras: extras, project: plan.ProjectName, modTime: time.Unix(plan.CommitTime, 0),
	}

	archives := make([]string, len(targets))
	errs := make([]error, len(targets))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i := range targets {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			archives[i], errs[i] = bd.buildTarget(ctx, &targets[i])
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return archives, nil
}

// runHook runs one before hook. The hook is a template, split into
// arguments as GoReleaser does, and run without a shell.
func runHook(ctx context.Context, hook string, tmpl *spec.Template, opts *BuildOptions) error {
	s, err := tmpl.Apply(hook, opts.Env)
	if err != nil {
		return err
	}
	args, err := splitShellWords(s)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errEmptyHook
	}
	_, err = run(ctx, opts.Dir, opts.Env, args[0], args[1:]...)
	return err
}

// errEmptyHook is returned for a before hook with no command. GoReleaser
// 1.20 panics on it.
var errEmptyHook = errors.New("empty command")

// builder holds what every target of one Build call shares.
type builder struct {
	arts    *spec.Artifacts
	opts    *BuildOptions
	tmpl    *spec.Template
	extras  []archiveEntry
	project string
	modTime time.Time
}

// buildTarget builds every build for one target and writes its archive.
// It checks the archive names of the binaries before it builds them.
func (bd *builder) buildTarget(ctx context.Context, t *spec.Target) (string, error) {
	for j := range bd.arts.Builds {
		name := bd.arts.Builds[j].Binary + t.BinaryExt()
		if slices.ContainsFunc(bd.extras, func(e archiveEntry) bool { return e.dst == name }) {
			return "", fmt.Errorf("archive %s: %s: %w", t, name, fs.ErrExist)
		}
	}
	outDir := filepath.Join(bd.opts.Dir, bd.opts.Dist, t.OS+"_"+t.Arch)
	entries := slices.Clone(bd.extras)
	for j := range bd.arts.Builds {
		b := &bd.arts.Builds[j]
		env, err := buildEnv(b, t, bd.tmpl, bd.opts.Env)
		if err != nil {
			return "", fmt.Errorf("build %q for %s: %w", b.ID, t, err)
		}
		ldflags, err := renderLDFlags(b, bd.tmpl, env)
		if err != nil {
			return "", err
		}
		name := b.Binary + t.BinaryExt()
		bin := filepath.Join(outDir, b.ID, name)
		if _, err := run(ctx, bd.opts.Dir, env, "go", "build", "-ldflags="+ldflags, "-o", bin, b.Main); err != nil {
			return "", fmt.Errorf("build %q for %s: %w", b.ID, t, err)
		}
		entries = append(entries, archiveEntry{src: bin, dst: name, mode: 0o755})
	}
	archive := filepath.Join(outDir, ArchiveName(bd.project, bd.tmpl.Version, t))
	return archive, writeArchive(archive, entries, bd.modTime)
}

// renderLDFlags renders the ldflags of a build with .Env from env, and
// joins them with spaces.
func renderLDFlags(b *spec.Build, tmpl *spec.Template, env []string) (string, error) {
	flags := make([]string, len(b.LDFlags))
	for i, f := range b.LDFlags {
		var err error
		if flags[i], err = tmpl.Apply(f, env); err != nil {
			return "", fmt.Errorf("build %q ldflags %q: %w", b.ID, f, err)
		}
	}
	return strings.Join(flags, " "), nil
}

// buildEnv returns the environment of one build for one target, as
// GoReleaser makes it: the process environment, then each builds[].env
// item with $VAR expanded from the process environment and applied as a
// template (an item that renders empty is left out), then the target
// variables.
func buildEnv(b *spec.Build, t *spec.Target, tmpl *spec.Template, processEnv []string) ([]string, error) {
	vars := spec.EnvMap(processEnv)
	env := slices.Clone(processEnv)
	for _, e := range b.Env {
		e = os.Expand(e, func(k string) string { return vars[k] })
		s, err := tmpl.Apply(e, env)
		if err != nil {
			return nil, fmt.Errorf("env %q: %w", e, err)
		}
		if s != "" {
			env = append(env, s)
		}
	}
	return append(env, t.Env()...), nil
}

// WriteChecksums writes checksums.txt for the archives with GoReleaser's
// "<sha256>  <name>" line format, and writes no file when there are no
// archives. The lines are sorted by file name, as in the latest
// GoReleaser (v2.20.2); 1.20, which the builder follows otherwise, sorts
// them by checksum.
func WriteChecksums(file string, archives []string) error {
	if len(archives) == 0 {
		return nil
	}
	sorted := slices.Clone(archives)
	slices.SortFunc(sorted, func(a, b string) int {
		return strings.Compare(filepath.Base(a), filepath.Base(b))
	})
	lines := make([]string, len(sorted))
	for i, a := range sorted {
		sum, err := sha256File(a)
		if err != nil {
			return err
		}
		lines[i] = sum + "  " + filepath.Base(a)
	}
	//nolint:gosec // Why: GoReleaser writes checksums.txt with mode 0644.
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

// selectTargets returns the requested targets, or all plan targets.
func selectTargets(arts *spec.Artifacts, plan *Plan, requested []string) ([]spec.Target, error) {
	for _, r := range requested {
		if !slices.Contains(plan.Targets, r) {
			return nil, fmt.Errorf("%w: %s", ErrUnknownTarget, r)
		}
	}
	var out []spec.Target
	for _, t := range arts.Targets {
		if slices.Contains(plan.Targets, t.String()) && (len(requested) == 0 || slices.Contains(requested, t.String())) {
			out = append(out, t)
		}
	}
	return out, nil
}

// archiveEntry is one file in an archive.
type archiveEntry struct {
	src, dst string
	mode     os.FileMode
}

// archiveExtras resolves the extra archive files once, as GoReleaser's
// archivefiles.Eval does: each src is a template and a glob; a matched
// folder adds every file in it; with a dst, a file keeps its path below
// the common folder of the matches. The result is sorted by destination,
// and the first file wins when two have one destination.
func archiveExtras(arts *spec.Artifacts, tmpl *spec.Template, env []string, dir string) ([]archiveEntry, error) {
	var out []archiveEntry
	for _, f := range arts.Files {
		glob, err := f.Glob(tmpl, env)
		if err != nil {
			return nil, err
		}
		files, err := globFiles(dir, glob)
		if err != nil {
			return nil, fmt.Errorf("archive file %q: %w", glob, err)
		}
		_, statErr := os.Stat(filepath.Join(dir, glob))
		hasMatchers := strings.ContainsAny(glob, "*?[")
		if len(files) == 0 {
			// fileglob.Glob fails for a file that does not exist.
			if !hasMatchers && errors.Is(statErr, fs.ErrNotExist) {
				return nil, fmt.Errorf("archive file %q: %w", glob, fs.ErrNotExist)
			}
			continue
		}
		prefix := glob
		if statErr != nil || hasMatchers {
			prefix = filepath.Dir(longestCommonPrefix(files))
		}
		for _, file := range files {
			dst := file
			if f.Dst != "" {
				rel, err := filepath.Rel(prefix, file)
				if err != nil {
					return nil, fmt.Errorf("archive file %q: %w", file, err)
				}
				dst = filepath.Join(f.Dst, rel)
			}
			out = append(out, archiveEntry{src: filepath.Join(dir, file), dst: filepath.ToSlash(dst), mode: f.Mode})
		}
	}
	slices.SortStableFunc(out, func(a, b archiveEntry) int { return strings.Compare(a.dst, b.dst) })
	return slices.CompactFunc(out, func(a, b archiveEntry) bool { return a.dst == b.dst }), nil
}

// globFiles returns the files that match a glob, relative to dir. A
// matched folder adds every file below it.
func globFiles(dir, glob string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, glob))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		err := filepath.WalkDir(m, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(dir, p)
			files = append(files, rel)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// longestCommonPrefix returns the longest string prefix of strs.
func longestCommonPrefix(strs []string) string {
	lcp := strs[0]
	for _, s := range strs[1:] {
		n := 0
		for n < len(lcp) && n < len(s) && lcp[n] == s[n] {
			n++
		}
		lcp = lcp[:n]
	}
	return lcp
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
