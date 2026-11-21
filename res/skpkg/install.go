package skpkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"skrp/res/front"
)

var ErrDeclined = errors.New("declined by user")

type ConfirmKind int

const (
	ConfirmOverwrite ConfirmKind = iota
	ConfirmForeignFiles
)

type ConfirmFunc func(kind ConfirmKind, question string, details []string, defaultYes bool) bool

type InstallOptions struct {
	ProjectDir   string
	PackagePath  string
	AllowForeign bool
	Force        bool
	DryRun       bool
	Confirm      ConfirmFunc
	Log          func(string)
}

type InstallResult struct {
	Name         string
	Version      string
	Author       string
	Dependencies []string
	LibraryPath  string
	CCFlagsPath  string
	Installed    []string
	Skipped      []string
	Foreign      []string // WARN
	Dangerous    []string // DANGER
}

func ConsumerVars(cfg *front.Config) map[string]string {
	pick := func(v, def string) string {
		if strings.TrimSpace(v) == "" {
			return def
		}
		return v
	}
	return map[string]string{
		"name":         cfg.Name,
		"version":      cfg.Version,
		"buildOutPath": pick(cfg.BuildOutPath, "bin/"),
		"testOutPath":  pick(cfg.TestOutPath, "test/"),
		"libs":         pick(cfg.LibsDir, "libs/"),
	}
}

type installPlanEntry struct {
	rel   string
	full  string
	stage string
	hash  string
}

func Install(opts InstallOptions) (*InstallResult, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string) {}
	}
	confirm := opts.Confirm
	if confirm == nil {
		confirm = func(ConfirmKind, string, []string, bool) bool { return false }
	}

	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(projectDir, manifestFileName)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, fmt.Errorf(
			"manifest.spc not found in '%s': run 'skorpion put' inside the consumer project", projectDir)
	}
	cfg := front.ParseConfig(manifestPath)

	if strings.TrimSpace(opts.PackagePath) == "" {
		return nil, fmt.Errorf(`no package specified: skorpion put -lib="file.skpkg"`)
	}
	pkgPath, err := resolvePackagePath(projectDir, cfg.LibsDir, opts.PackagePath)
	if err != nil {
		return nil, err
	}
	logf("Package: " + pkgPath)

	pa, err := openPackageArchive(pkgPath)
	if err != nil {
		return nil, err
	}
	defer pa.Close()

	raw, err := pa.readIncludes()
	if err != nil {
		return nil, err
	}
	manifest, err := DecodeIncludes(raw)
	if err != nil {
		return nil, err
	}

	if pa.libraryName != manifest.Library.File {
		return nil, fmt.Errorf("package mismatch: %s points to '%s', but the archive contains '%s'",
			includesFileName, manifest.Library.File, pa.libraryName)
	}

	needed := make(map[string]int64, len(manifest.Includes))
	for _, e := range manifest.Includes {
		needed[e.Hash] = e.Size
	}
	if len(needed) != len(pa.blobs) {
		return nil, fmt.Errorf("package mismatch: %s lists %d unique file(s), the archive contains %d",
			includesFileName, len(needed), len(pa.blobs))
	}
	for hash := range needed {
		if _, ok := pa.blobs[hash]; !ok {
			return nil, fmt.Errorf("package mismatch: include/%s is missing from the archive", hash)
		}
	}

	stage, err := os.MkdirTemp("", "skorpion-pkg-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)

	libStage := filepath.Join(stage, "library"+libraryExt)
	libHash, _, err := pa.extractAndHash(pa.library, libStage, maxLibrarySize)
	if err != nil {
		return nil, err
	}
	if libHash != manifest.Library.Hash {
		return nil, fmt.Errorf("integrity check failed: '%s' hash mismatch (expected %s, got %s)",
			manifest.Library.File, manifest.Library.Hash, libHash)
	}

	lib, err := front.ReadLibrary(libStage)
	if err != nil {
		return nil, fmt.Errorf("cannot read '%s': %w", manifest.Library.File, err)
	}
	if lib.Name != manifest.Library.Name ||
		lib.Version != manifest.Library.Version ||
		lib.Author != manifest.Library.Author {
		return nil, fmt.Errorf(
			"metadata mismatch: %s declares %s/%s@%s, but the library declares %s/%s@%s",
			includesFileName,
			manifest.Library.Author, manifest.Library.Name, manifest.Library.Version,
			lib.Author, lib.Name, lib.Version)
	}
	for _, id := range []struct{ label, value string }{
		{"author", lib.Author}, {"name", lib.Name}, {"version", lib.Version},
	} {
		if err := CheckIdentifier(id.value); err != nil {
			return nil, fmt.Errorf("library %s: %w", id.label, err)
		}
	}

	blobDir := filepath.Join(stage, includeDirName)
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return nil, err
	}
	for _, hash := range sortedKeys(pa.blobs) {
		dst := filepath.Join(blobDir, hash)
		got, size, err := pa.extractAndHash(pa.blobs[hash], dst, maxIncludeFileSize)
		if err != nil {
			return nil, err
		}
		if got != hash {
			return nil, fmt.Errorf("integrity check failed: include/%s actually hashes to %s", hash, got)
		}
		if want := needed[hash]; want != size {
			return nil, fmt.Errorf("integrity check failed: include/%s is %d bytes, %s declares %d",
				hash, size, includesFileName, want)
		}
	}

	vars := ConsumerVars(cfg)
	var plan []installPlanEntry
	var foreign []string
	var dangerous []string
	usedTargets := make(map[string]string, len(manifest.Includes))

	for _, e := range manifest.Includes {
		rel, err := ResolveOutPattern(e.Out, vars)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(rel)
		if prev, ok := usedTargets[key]; ok {
			return nil, fmt.Errorf(
				"package conflict: '%s' and '%s' both resolve to '%s'", prev, e.Out, rel)
		}
		usedTargets[key] = e.Out

		full, err := SafeJoin(projectDir, rel)
		if err != nil {
			return nil, err
		}
		if err := EnsureNoSymlinks(projectDir, rel); err != nil {
			return nil, err
		}

		switch ClassifyExtension(rel) {
		case ExtWarn:
			foreign = append(foreign, rel)
		case ExtDanger:
			dangerous = append(dangerous, rel)
		}

		plan = append(plan, installPlanEntry{
			rel: rel, full: full, stage: filepath.Join(blobDir, e.Hash), hash: e.Hash,
		})
	}
	sort.Strings(foreign)
	sort.Strings(dangerous)

	libsRoot := cfg.LibsDir
	if strings.TrimSpace(libsRoot) == "" {
		libsRoot = "libs/"
	}
	if filepath.IsAbs(libsRoot) {
		libsRoot = filepath.Clean(libsRoot)
	} else {
		libsRoot = filepath.Join(projectDir, filepath.FromSlash(libsRoot))
	}
	libRel := lib.Author + "/" + lib.Name + "/" + manifest.Library.File
	if err := EnsureNoSymlinks(libsRoot, libRel); err != nil {
		return nil, err
	}
	libFull := filepath.Join(libsRoot, filepath.FromSlash(libRel))

	// Предупреждения о нестандартных файлах.
	allWarnings := append(foreign, dangerous...)
	if len(allWarnings) > 0 && !opts.AllowForeign {
		if !confirm(ConfirmForeignFiles,
			"This package ships non-standard files. Skorpion does not vouch for them. Install anyway?",
			allWarnings, false) {
			return nil, ErrDeclined
		}
	}

	var overwrites []string
	var writePlan []installPlanEntry
	var skipped []string

	for _, p := range plan {
		same, exists, err := sameFile(p.full, p.hash)
		if err != nil {
			return nil, err
		}
		if same {
			skipped = append(skipped, p.rel)
			continue
		}
		if exists {
			overwrites = append(overwrites, p.rel)
		}
		writePlan = append(writePlan, p)
	}

	libSame, libExists, err := sameFile(libFull, libHash)
	if err != nil {
		return nil, err
	}
	libDisplay := displayPath(projectDir, libFull)
	if libExists && !libSame {
		overwrites = append(overwrites, libDisplay)
	}

	if len(overwrites) > 0 && !opts.Force {
		sort.Strings(overwrites)
		if !confirm(ConfirmOverwrite,
			"These files already exist and will be overwritten. Continue?", overwrites, false) {
			return nil, ErrDeclined
		}
	}

	result := &InstallResult{
		Name:         lib.Name,
		Version:      lib.Version,
		Author:       lib.Author,
		Dependencies: lib.Dependencies,
		LibraryPath:  libDisplay,
		Skipped:      skipped,
		Foreign:      foreign,
		Dangerous:    dangerous,
	}
	for _, p := range writePlan {
		result.Installed = append(result.Installed, p.rel)
	}
	if !libSame {
		result.Installed = append(result.Installed, libDisplay)
	}
	sort.Strings(result.Installed)
	sort.Strings(result.Skipped)

	if opts.DryRun {
		return result, nil
	}

	var tx fileTxn
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				logf("rollback reported an error: " + rbErr.Error())
			}
		}
	}()

	for _, p := range writePlan {
		if err := tx.MkdirAll(filepath.Dir(p.full)); err != nil {
			return nil, err
		}
		if err := tx.CopyFile(p.full, p.stage); err != nil {
			return nil, fmt.Errorf("cannot write '%s': %w", p.rel, err)
		}
	}
	if !libSame {
		if err := tx.MkdirAll(filepath.Dir(libFull)); err != nil {
			return nil, err
		}
		if err := tx.CopyFile(libFull, libStage); err != nil {
			return nil, fmt.Errorf("cannot write '%s': %w", libFull, err)
		}
	}

	// Пишем ccflags.json, если есть флаги.
	if HasCCFlags(manifest.CCFlags) {
		ccflagsPath := filepath.Join(libsRoot, lib.Author, lib.Name, CCFlagsFileName)
		if err := tx.MkdirAll(filepath.Dir(ccflagsPath)); err != nil {
			return nil, err
		}
		if err := WriteCCFlags(ccflagsPath, manifest.CCFlags); err != nil {
			return nil, fmt.Errorf("cannot write '%s': %w", ccflagsPath, err)
		}
		result.CCFlagsPath = displayPath(projectDir, ccflagsPath)
		result.Installed = append(result.Installed, result.CCFlagsPath)
		sort.Strings(result.Installed)
	}

	tx.Commit()
	committed = true
	return result, nil
}

func sameFile(target, hash string) (same bool, exists bool, err error) {
	fi, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if err := checkRegularFile(fi, target); err != nil {
		return false, true, err
	}
	got, _, err := HashFile(target)
	if err != nil {
		return false, true, err
	}
	return got == hash, true, nil
}

func resolvePackagePath(projectDir, libsDir, given string) (string, error) {
	var candidates []string
	if filepath.IsAbs(given) {
		candidates = append(candidates, filepath.Clean(given))
	} else {
		candidates = append(candidates, filepath.Join(projectDir, filepath.FromSlash(given)))

		root := libsDir
		if strings.TrimSpace(root) == "" {
			root = "libs/"
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(projectDir, filepath.FromSlash(root))
		}
		candidates = append(candidates, filepath.Join(root, filepath.FromSlash(given)))
	}

	for _, c := range candidates {
		fi, err := os.Stat(c)
		if err == nil && fi.Mode().IsRegular() {
			return c, nil
		}
	}
	return "", fmt.Errorf("package '%s' not found (looked in '%s' and in the libs directory)",
		given, projectDir)
}

func displayPath(projectDir, full string) string {
	rel, err := filepath.Rel(projectDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return full
	}
	return filepath.ToSlash(rel)
}
