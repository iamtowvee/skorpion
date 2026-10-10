package front

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"strings"
)

// Версия формата .sklib. Меняется при несовместимых изменениях структуры.
const SKLibFormatVersion = 1

// Library — содержимое .sklib.
type Library struct {
	FormatVersion int
	Name          string
	Version       string
	Author        string
	Dependencies  []string
	Program       *Program
}

// WriteLibrary сериализует Library в файл.
func WriteLibrary(path string, lib *Library) error {
	lib.FormatVersion = SKLibFormatVersion

	// AllFunctions восстанавливается при импорте — не пишем.
	if lib.Program != nil {
		lib.Program.AllFunctions = nil
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(lib); err != nil {
		return fmt.Errorf("gob encode: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// ReadLibrary читает Library из файла.
func ReadLibrary(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var lib Library
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&lib); err != nil {
		return nil, fmt.Errorf("gob decode: %w", err)
	}

	if lib.FormatVersion != SKLibFormatVersion {
		return nil, fmt.Errorf(
			"unsupported .sklib format version: got %d, expected %d (recompile the library)",
			lib.FormatVersion, SKLibFormatVersion)
	}

	return &lib, nil
}

// LibSpec — разобранный путь "lib:author/name@1.0.0".
type LibSpec struct {
	Author  string
	Name    string
	Version string // "" если не указана
}

// Raw возвращает исходную строку без "lib:".
func (s LibSpec) Raw() string {
	if s.Version == "" {
		return s.Author + "/" + s.Name
	}
	return s.Author + "/" + s.Name + "@" + s.Version
}

// ParseLibSpec разбирает "lib:author/name@1.0.0" (или без версии).
func ParseLibSpec(path string) (LibSpec, error) {
	spec := LibSpec{}

	p := strings.TrimPrefix(path, "lib:")
	if p == path {
		return spec, fmt.Errorf("library path must start with 'lib:'")
	}

	if idx := strings.LastIndex(p, "@"); idx >= 0 {
		spec.Version = p[idx+1:]
		p = p[:idx]
		if spec.Version == "" {
			return spec, fmt.Errorf("empty version in '%s'", path)
		}
	}

	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 {
		return spec, fmt.Errorf(
			"invalid library path '%s': expected 'lib:author/name[@version]'", path)
	}

	spec.Author = parts[0]
	spec.Name = parts[1]

	if spec.Author == "" || spec.Name == "" {
		return spec, fmt.Errorf("empty author or name in '%s'", path)
	}

	return spec, nil
}

// FindLibraryFile ищет .sklib в libsDir для данного spec.
// Если version указана — ищет LIBRARY-{name}-{version}.sklib.
// Если нет — требует ровно один .sklib в папке.
func FindLibraryFile(libsDir string, spec LibSpec) (string, error) {
	dir := fmt.Sprintf("%s/%s/%s",
		strings.TrimRight(libsDir, "/"), spec.Author, spec.Name)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf(
			"library not found: %s/%s (dir: %s)", spec.Author, spec.Name, dir)
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "LIBRARY-") || !strings.HasSuffix(name, ".sklib") {
			continue
		}
		if spec.Version != "" {
			expected := fmt.Sprintf("LIBRARY-%s-%s.sklib", spec.Name, spec.Version)
			if name == expected {
				matches = append(matches, dir+"/"+name)
			}
		} else {
			matches = append(matches, dir+"/"+name)
		}
	}

	if len(matches) == 0 {
		if spec.Version != "" {
			return "", fmt.Errorf("library %s/%s@%s not found in %s",
				spec.Author, spec.Name, spec.Version, dir)
		}
		return "", fmt.Errorf("library %s/%s not found in %s",
			spec.Author, spec.Name, dir)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf(
			"multiple versions of library %s/%s found in %s; specify @version",
			spec.Author, spec.Name, dir)
	}

	return matches[0], nil
}

// init регистрирует все типы AST в gob.
func init() {
	gob.Register(&Program{})
	gob.Register(&Import{})
	gob.Register(&Function{})
	gob.Register(&Param{})
	gob.Register(&Block{})
	gob.Register(&VarDecl{})
	gob.Register(&Assign{})
	gob.Register(&BinaryExpr{})
	gob.Register(&UnaryExpr{})
	gob.Register(&CallExpr{})
	gob.Register(&CallRangeExpr{})
	gob.Register(&RangeExpr{})
	gob.Register(&ReturnStmt{})
	gob.Register(&Ident{})
	gob.Register(&Number{})
	gob.Register(&String{})
	gob.Register(&CharLiteral{})
	gob.Register(&UnicodeLiteral{})
	gob.Register(&NullLiteral{})
	gob.Register(&IfStmt{})
	gob.Register(&Elsif{})
	gob.Register(&CaseStmt{})
	gob.Register(&CaseBranch{})
	gob.Register(&WhileStmt{})
	gob.Register(&ForInStmt{})
	gob.Register(&ArrayLiteral{})
	gob.Register(&ArrayIndex{})
	gob.Register(&ArrayLength{})
	gob.Register(&ArrayAdd{})
	gob.Register(&DictLiteral{})
	gob.Register(&DictElement{})
	gob.Register(&DictIndex{})
	gob.Register(&TypeOf{})
	gob.Register(&TernaryExpr{})
	gob.Register(&FormatExpr{})
	gob.Register(&IncludeC{})
	gob.Register(&IknowIdoBlock{})
	gob.Register(&ErrorDecl{})
	gob.Register(&ErrorField{})
	gob.Register(&ErrorInstance{})
	gob.Register(&ThrowStmt{})
	gob.Register(&FieldAccess{})
	gob.Register(&TryStmt{})
	gob.Register(&CatchClause{})
}
