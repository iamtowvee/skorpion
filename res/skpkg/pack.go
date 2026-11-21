package skpkg

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"skrp/res/front"
)

type PackOptions struct {
	ProjectDir string
	OutputDir  string
	DryRun     bool
	Log        func(string)
}

type PackResult struct {
	PackagePath string
	LibraryPath string
	Files       int
	Bytes       int64
	Foreign     []string // WARN
	Dangerous   []string // DANGER
}

type stagedFile struct {
	hash   string
	source string
	out    string
	size   int64
	full   string
}

func Pack(opts PackOptions) (*PackResult, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string) {}
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
		return nil, fmt.Errorf("manifest.spc not found in '%s'", projectDir)
	}
	cfg := front.ParseConfig(manifestPath)

	if cfg.Name == "" {
		return nil, fmt.Errorf("manifest.spc: 'name' is required to build a package")
	}
	if len(cfg.Authors) == 0 {
		return nil, fmt.Errorf("manifest.spc: 'authors' is required to build a package: authors[(\"your-name\")]")
	}
	if cfg.Version == "" {
		cfg.Version = "0.0.0"
	}
	for _, id := range []struct{ label, value string }{
		{"name", cfg.Name}, {"version", cfg.Version}, {"authors[0]", cfg.Authors[0]},
	} {
		if err := CheckIdentifier(id.value); err != nil {
			return nil, fmt.Errorf("manifest.spc: %s: %w", id.label, err)
		}
	}

	builtDir := cfg.BuildOutPath
	if strings.TrimSpace(builtDir) == "" {
		builtDir = "bin/"
	}
	if !filepath.IsAbs(builtDir) {
		builtDir = filepath.Join(projectDir, filepath.FromSlash(builtDir))
	}

	libPath, _, err := findBuiltLibrary(builtDir, cfg.Name, cfg.Version, cfg.Authors[0], logf)
	if err != nil {
		return nil, err
	}
	libHash, _, err := HashFile(libPath)
	if err != nil {
		return nil, err
	}

	var sf *ShiftingFile
	if cfg.Shifting != "" {
		rel := path.Clean(filepath.ToSlash(cfg.Shifting))
		skdPath, err := SafeJoin(projectDir, rel)
		if err != nil {
			return nil, fmt.Errorf("shifting[\"%s\"]: %w", cfg.Shifting, err)
		}
		if err := EnsureNoSymlinks(projectDir, rel); err != nil {
			return nil, fmt.Errorf("shifting[\"%s\"]: %w", cfg.Shifting, err)
		}
		sf, err = ParseShiftingFile(skdPath)
		if err != nil {
			return nil, err
		}
		logf(fmt.Sprintf("Shifting script: %s (%d rule(s), %d directive(s))",
			rel, len(sf.Entries), len(sf.Directives)))
	} else {
		sf = &ShiftingFile{}
		logf("No 'shifting' declared — the package will contain only the library")
	}

	var files []stagedFile
	var foreign []string
	var dangerous []string
	var total int64

	for _, e := range sf.Entries {
		full, err := SafeJoin(projectDir, e.Source)
		if err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", cfg.Shifting, e.Line, e.Column, err)
		}
		if err := EnsureNoSymlinks(projectDir, e.Source); err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", cfg.Shifting, e.Line, e.Column, err)
		}
		fi, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("%s:%d:%d: source '%s' not found",
				cfg.Shifting, e.Line, e.Column, e.Source)
		}
		if err := checkRegularFile(fi, e.Source); err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", cfg.Shifting, e.Line, e.Column, err)
		}
		if fi.Size() > maxIncludeFileSize {
			return nil, fmt.Errorf("%s:%d:%d: source '%s' is too large (%d bytes, limit %d)",
				cfg.Shifting, e.Line, e.Column, e.Source, fi.Size(), maxIncludeFileSize)
		}

		hash, size, err := HashFile(full)
		if err != nil {
			return nil, err
		}

		out := e.OutPattern()
		files = append(files, stagedFile{
			hash: hash, source: e.Source, out: out, size: size, full: full,
		})
		total += size

		switch ClassifyExtension(out) {
		case ExtWarn:
			foreign = append(foreign, out)
		case ExtDanger:
			dangerous = append(dangerous, out)
		}
	}

	if total > maxPackageTotalSize {
		return nil, fmt.Errorf("package payload is too large: %d bytes (limit %d)",
			total, maxPackageTotalSize)
	}

	sort.SliceStable(files, func(i, j int) bool { return files[i].out < files[j].out })
	sort.Strings(foreign)
	sort.Strings(dangerous)

	// Собираем CCFlags из директив.
	var ccflags CCFlagsConfig
	for _, d := range sf.Directives {
		switch d.OS {
		case "linux":
			ccflags.Linux = d.Flags
		case "windows":
			ccflags.Windows = d.Flags
		case "darwin":
			ccflags.Darwin = d.Flags
		}
	}

	libFileName := cfg.Name + "-" + cfg.Version + libraryExt
	manifest := Includes{
		FormatVersion: IncludesFormatVersion,
		Library: LibraryRef{
			File:    libFileName,
			Hash:    libHash,
			Name:    cfg.Name,
			Version: cfg.Version,
			Author:  cfg.Authors[0],
		},
		CCFlags: ccflags,
	}

	blobs := make(map[string]string, len(files))
	for _, f := range files {
		manifest.Includes = append(manifest.Includes, IncludeEntry{
			Hash: f.hash, Out: f.out, Size: f.size, Source: f.source,
		})
		blobs[f.hash] = f.full
	}

	manifestData, err := EncodeIncludes(&manifest)
	if err != nil {
		return nil, err
	}

	sources := []archiveSource{
		{Name: includesFileName, Data: manifestData},
		{Name: libFileName, File: libPath},
	}
	for _, hash := range sortedKeys(blobs) {
		sources = append(sources, archiveSource{Name: includeDirName + "/" + hash, File: blobs[hash]})
	}

	outDir := opts.OutputDir
	if strings.TrimSpace(outDir) == "" {
		outDir = builtDir
	} else if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(projectDir, filepath.FromSlash(outDir))
	}
	target := filepath.Join(outDir, cfg.Name+"-"+cfg.Version+PackageExt)

	result := &PackResult{
		PackagePath: target,
		LibraryPath: libPath,
		Files:       len(files),
		Bytes:       total,
		Foreign:     foreign,
		Dangerous:   dangerous,
	}

	if opts.DryRun {
		return result, nil
	}
	if err := writeZipArchive(target, sources); err != nil {
		return nil, fmt.Errorf("cannot write package '%s': %w", target, err)
	}
	return result, nil
}

func findBuiltLibrary(dir, name, version, author string, logf func(string)) (string, *front.Library, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf(
			"cannot read build directory '%s': %w (run 'skorpion build-lib' first)", dir, err)
	}

	var matches []string
	var libs []*front.Library

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), libraryExt) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		lib, err := front.ReadLibrary(p)
		if err != nil {
			logf("skipped " + e.Name() + ": " + err.Error())
			continue
		}
		if lib.Name == name && lib.Version == version && lib.Author == author {
			matches = append(matches, p)
			libs = append(libs, lib)
		}
	}

	switch len(matches) {
	case 0:
		return "", nil, fmt.Errorf(
			"no built library %s/%s@%s found in '%s' — run 'skorpion build-lib' first",
			author, name, version, dir)
	case 1:
		return matches[0], libs[0], nil
	default:
		return "", nil, fmt.Errorf(
			"multiple built libraries match %s/%s@%s in '%s': %s",
			author, name, version, dir, strings.Join(matches, ", "))
	}
}
