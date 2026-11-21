package skpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// CheckRelativePath — путь внутри проекта: относительный, нормализованный,
// без обхода каталогов и без «сюрпризов» Windows/POSIX.
func CheckRelativePath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("NUL byte in path")
	}
	if strings.ContainsRune(p, '\\') {
		return fmt.Errorf("backslashes are not allowed, use '/'")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("absolute paths are not allowed")
	}
	if strings.ContainsRune(p, ':') {
		return fmt.Errorf("drive letters and NTFS alternate streams are not allowed")
	}
	if filepath.VolumeName(p) != "" {
		return fmt.Errorf("volume-qualified paths are not allowed")
	}
	if path.Clean(p) != p {
		return fmt.Errorf("path is not normalized (clean form is '%s')", path.Clean(p))
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid path segment '%s'", seg)
		}
		for _, r := range seg {
			if r < 0x20 || r == 0x7f {
				return fmt.Errorf("control characters in segment '%s'", seg)
			}
		}
		if strings.ContainsAny(seg, `<>|"*?`) {
			return fmt.Errorf("forbidden characters in segment '%s'", seg)
		}
		if strings.HasSuffix(seg, " ") || strings.HasSuffix(seg, ".") {
			return fmt.Errorf("segment '%s' must not end with a space or a dot", seg)
		}
		if isWindowsReservedName(seg) {
			return fmt.Errorf("segment '%s' is a reserved device name", seg)
		}
	}
	return nil
}

// CheckIdentifier проверяет author/name/version, которые становятся
// именами каталогов и файлов при установке.
func CheckIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("empty identifier")
	}
	if s == "." || s == ".." {
		return fmt.Errorf("identifier '%s' is not a valid name", s)
	}
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-' || r == '+'
		if !ok {
			return fmt.Errorf("invalid character %q in identifier '%s'", r, s)
		}
	}
	if isWindowsReservedName(s) {
		return fmt.Errorf("identifier '%s' is a reserved device name", s)
	}
	return nil
}

func isWindowsReservedName(seg string) bool {
	base := strings.ToUpper(seg)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		return base[3] >= '1' && base[3] <= '9'
	}
	return false
}

// IsHash — sha256 в нижнем регистре, ровно 64 символа.
func IsHash(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// HashFile считает sha256 файла потоково.
func HashFile(filePath string) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// SafeJoin соединяет корень и относительный путь и проверяет, что результат
// не выходит за корень.
func SafeJoin(root, rel string) (string, error) {
	if err := CheckRelativePath(rel); err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(rel))

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	back, err := filepath.Rel(absRoot, absFull)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path '%s' escapes '%s'", rel, root)
	}
	return full, nil
}

// EnsureNoSymlinks запрещает symlink в любом компоненте пути: иначе пакет
// мог бы писать сквозь заранее подложенную ссылку.
func EnsureNoSymlinks(root, rel string) error {
	parts := strings.Split(rel, "/")
	cur := root
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path '%s' goes through the symlink '%s'",
				rel, strings.Join(parts[:i+1], "/"))
		}
		if i < len(parts)-1 && !fi.IsDir() {
			return fmt.Errorf("'%s' is not a directory", strings.Join(parts[:i+1], "/"))
		}
	}
	return nil
}

func checkRegularFile(fi os.FileInfo, name string) error {
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("'%s' is a symlink", name)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("'%s' is not a regular file", name)
	}
	return nil
}

// WriteFileAtomic пишет через временный файл в том же каталоге + rename.
func WriteFileAtomic(target string, r io.Reader, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".skpkg-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// ── транзакция на файловой системе ──────────────────────────────────────────

type txEntry struct {
	target string
	backup string // "" если файла не было
}

// fileTxn — «всё или ничего» для набора файлов. Существующие файлы
// откладываются в бэкап рядом с целью (тот же том → rename атомарен).
type fileTxn struct {
	entries []txEntry
	dirs    []string
}

func (t *fileTxn) MkdirAll(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	var missing []string
	cur := abs
	for {
		fi, statErr := os.Stat(cur)
		if statErr == nil && fi.IsDir() {
			break
		}
		missing = append(missing, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		t.dirs = append(t.dirs, missing[i])
	}
	return nil
}

func (t *fileTxn) CopyFile(target, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.WriteStream(target, f)
}

func (t *fileTxn) WriteStream(target string, r io.Reader) error {
	backup, err := t.backupExisting(target)
	if err != nil {
		return err
	}
	if err := WriteFileAtomic(target, r, 0o644); err != nil {
		if backup != "" {
			os.Rename(backup, target)
		}
		return err
	}
	t.entries = append(t.entries, txEntry{target: target, backup: backup})
	return nil
}

func (t *fileTxn) backupExisting(target string) (string, error) {
	fi, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("'%s' is a directory", target)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("'%s' is a symlink", target)
	}

	for i := 0; ; i++ {
		candidate := fmt.Sprintf("%s.skpkg-bak-%d", target, i)
		if _, statErr := os.Lstat(candidate); os.IsNotExist(statErr) {
			if err := os.Rename(target, candidate); err != nil {
				return "", err
			}
			return candidate, nil
		} else if statErr != nil {
			return "", statErr
		}
	}
}

func (t *fileTxn) Commit() {
	for _, e := range t.entries {
		if e.backup != "" {
			os.Remove(e.backup)
		}
	}
	t.entries = nil
	t.dirs = nil
}

func (t *fileTxn) Rollback() error {
	var firstErr error
	for i := len(t.entries) - 1; i >= 0; i-- {
		e := t.entries[i]
		var err error
		if e.backup != "" {
			err = os.Rename(e.backup, e.target)
		} else {
			err = os.Remove(e.target)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for i := len(t.dirs) - 1; i >= 0; i-- {
		os.Remove(t.dirs[i]) // удалится, только если пуст
	}
	t.entries = nil
	t.dirs = nil
	return firstErr
}
