package skpkg

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	includesFileName = "includes.yml"
	includeDirName   = "include"
	manifestFileName = "manifest.spc"
	libraryExt       = ".sklib"

	// PackageExt — расширение пакета Skorpion.
	PackageExt = ".skpkg"
)

const (
	maxIncludeFileSize  int64 = 64 << 20
	maxLibrarySize      int64 = 64 << 20
	maxIncludesSize     int64 = 4 << 20
	maxPackageTotalSize int64 = 1024 << 20
	maxPackageEntries         = 8192
)

// zipEpoch — фиксированная дата для детерминированных архивов.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ── запись ──────────────────────────────────────────────────────────────────

type archiveSource struct {
	Name string
	Data []byte // если задано — пишем байты
	File string // иначе стримим файл с диска
}

func writeZipArchive(target string, sources []archiveSource) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".skpkg-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	fillErr := fillZip(tmp, sources)
	closeErr := tmp.Close()
	if fillErr != nil {
		os.Remove(tmpName)
		return fillErr
	}
	if closeErr != nil {
		os.Remove(tmpName)
		return closeErr
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func fillZip(w io.Writer, sources []archiveSource) error {
	zw := zip.NewWriter(w)
	for _, s := range sources {
		h := &zip.FileHeader{Name: s.Name, Method: zip.Deflate, Modified: zipEpoch}
		h.SetMode(0o644)

		entry, err := zw.CreateHeader(h)
		if err != nil {
			zw.Close()
			return err
		}

		if s.File != "" {
			f, err := os.Open(s.File)
			if err != nil {
				zw.Close()
				return err
			}
			_, err = io.Copy(entry, f)
			f.Close()
			if err != nil {
				zw.Close()
				return err
			}
			continue
		}

		if _, err := entry.Write(s.Data); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

// ── чтение ──────────────────────────────────────────────────────────────────

type packageArchive struct {
	zr          *zip.ReadCloser
	includes    *zip.File
	library     *zip.File
	libraryName string
	blobs       map[string]*zip.File
}

// openPackageArchive открывает .skpkg и принимает только три вида записей:
// includes.yml, один *.sklib в корне и include/<sha256>. Всё прочее — отказ.
func openPackageArchive(filePath string) (*packageArchive, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open package '%s': %w", filePath, err)
	}

	fail := func(format string, args ...interface{}) (*packageArchive, error) {
		zr.Close()
		return nil, fmt.Errorf(format, args...)
	}

	if len(zr.File) == 0 {
		return fail("package '%s' is empty", filePath)
	}
	if len(zr.File) > maxPackageEntries {
		return fail("package '%s' has too many entries: %d (limit %d)",
			filePath, len(zr.File), maxPackageEntries)
	}

	names := make([]string, len(zr.File))
	for i, f := range zr.File {
		n, err := normalizeArchiveName(f.Name)
		if err != nil {
			return fail("package '%s': %w", filePath, err)
		}
		names[i] = n
	}
	prefix := commonRootPrefix(names)

	pa := &packageArchive{zr: zr, blobs: make(map[string]*zip.File)}
	seen := make(map[string]bool, len(zr.File))
	var total int64

	for i, f := range zr.File {
		name := strings.TrimPrefix(names[i], prefix)
		if f.FileInfo().IsDir() {
			continue
		}
		if seen[name] {
			return fail("package '%s': duplicate entry '%s'", filePath, name)
		}
		seen[name] = true

		if f.Mode()&os.ModeSymlink != 0 || !f.Mode().IsRegular() {
			return fail("package '%s': entry '%s' is not a regular file", filePath, name)
		}

		size := int64(f.UncompressedSize64)
		total += size
		if total > maxPackageTotalSize {
			return fail("package '%s' exceeds the unpacked size limit of %d bytes",
				filePath, maxPackageTotalSize)
		}

		switch {
		case name == includesFileName:
			if size > maxIncludesSize {
				return fail("package '%s': %s is too large", filePath, includesFileName)
			}
			pa.includes = f

		case strings.HasPrefix(name, includeDirName+"/"):
			hash := strings.TrimPrefix(name, includeDirName+"/")
			if !IsHash(hash) {
				return fail("package '%s': entry '%s' must be named include/<sha256>", filePath, name)
			}
			if size > maxIncludeFileSize {
				return fail("package '%s': entry '%s' is too large", filePath, name)
			}
			pa.blobs[hash] = f

		case !strings.Contains(name, "/") && strings.HasSuffix(name, libraryExt):
			if pa.library != nil {
				return fail("package '%s' contains more than one %s file", filePath, libraryExt)
			}
			if size > maxLibrarySize {
				return fail("package '%s': library '%s' is too large", filePath, name)
			}
			pa.library = f
			pa.libraryName = name

		default:
			return fail("package '%s': unexpected entry '%s'", filePath, name)
		}
	}

	if pa.includes == nil {
		return fail("package '%s': %s is missing", filePath, includesFileName)
	}
	if pa.library == nil {
		return fail("package '%s': no %s file found", filePath, libraryExt)
	}
	return pa, nil
}

func (pa *packageArchive) Close() error { return pa.zr.Close() }

func (pa *packageArchive) readIncludes() ([]byte, error) {
	rc, err := pa.includes.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, maxIncludesSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxIncludesSize {
		return nil, fmt.Errorf("%s exceeds the size limit of %d bytes", includesFileName, maxIncludesSize)
	}
	return data, nil
}

// extractAndHash распаковывает запись и считает её sha256 одним проходом.
func (pa *packageArchive) extractAndHash(f *zip.File, dst string, limit int64) (string, int64, error) {
	rc, err := f.Open()
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, err
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(rc, limit+1))
	closeErr := out.Close()
	if err != nil {
		return "", 0, err
	}
	if closeErr != nil {
		return "", 0, closeErr
	}
	if n > limit {
		return "", 0, fmt.Errorf("entry '%s' exceeds the size limit of %d bytes", f.Name, limit)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func normalizeArchiveName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty entry name")
	}
	if strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("unsafe entry name '%s': backslashes", name)
	}
	if strings.HasPrefix(name, "/") || strings.ContainsRune(name, ':') {
		return "", fmt.Errorf("unsafe entry name '%s'", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == 0 {
			return "", fmt.Errorf("unsafe entry name '%s': control characters", name)
		}
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe entry name '%s': path traversal", name)
	}
	if clean != strings.TrimSuffix(name, "/") {
		return "", fmt.Errorf("non-normalized entry name '%s'", name)
	}
	return clean, nil
}

// commonRootPrefix допускает архивы с одной папкой-обёрткой.
func commonRootPrefix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	i := strings.IndexByte(names[0], '/')
	if i <= 0 {
		return ""
	}
	prefix := names[0][:i+1]
	for _, n := range names {
		if !strings.HasPrefix(n, prefix) {
			return ""
		}
	}
	return prefix
}
