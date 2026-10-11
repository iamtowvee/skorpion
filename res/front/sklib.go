package front

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"
	"strings"
)

// Версия формата .sklib. Меняется при несовместимых изменениях структуры.
const SKLibFormatVersion = 1

// Магические байты в начале .sklib. Позволяют мгновенно отличить
// файл Skorpion-библиотеки от чего угодно другого.
var sklibMagic = []byte{'S', 'K', 'L', 'B'}

// Размер заголовка: 4 байта магии + 1 байт версии формата.
const sklibHeaderSize = 5

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

	// gob-тело, сжатое gzip
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(lib); err != nil {
		return fmt.Errorf("gob encode: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}

	// Собираем файл: magic + version + gzipped gob
	var buf bytes.Buffer
	buf.Write(sklibMagic)
	buf.WriteByte(byte(SKLibFormatVersion))
	buf.Write(body.Bytes())

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

	// 1. Проверка заголовка
	if len(data) < sklibHeaderSize {
		return nil, fmt.Errorf("not a .sklib file: too short (%d bytes)", len(data))
	}
	if !bytes.Equal(data[:4], sklibMagic) {
		return nil, fmt.Errorf(
			"not a .sklib file: bad magic bytes (expected %q, got %q)",
			sklibMagic, data[:4])
	}

	headerVersion := int(data[4])
	if headerVersion != SKLibFormatVersion {
		return nil, fmt.Errorf(
			"unsupported .sklib format version: got %d, expected %d (recompile the library)",
			headerVersion, SKLibFormatVersion)
	}

	// 2. gzip + gob
	body := data[sklibHeaderSize:]
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gzip open: %w", err)
	}
	defer gz.Close()

	var lib Library
	dec := gob.NewDecoder(gz)
	if err := dec.Decode(&lib); err != nil {
		return nil, fmt.Errorf("gob decode: %w", err)
	}

	// 3. Проверка версии внутри Library (защита от рассинхрона)
	// Если кто-то вручную подменил байт в заголовке — поймаем здесь.
	if lib.FormatVersion != SKLibFormatVersion {
		return nil, fmt.Errorf(
			"internal version mismatch: header=%d, body=%d (file is corrupted)",
			headerVersion, lib.FormatVersion)
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

// FindLibrary ищет .sklib в libsDir для данного spec
// и возвращает прочитанную Library вместе с путём к файлу.
//
// Правила:
//   - Папка: {libsDir}/{author}/{name}/
//   - Расширение: .sklib (обязательно)
//   - Если version указан — ищем файл, внутри которого Version == spec.Version
//   - Если version НЕ указан — в папке должен быть ровно 1 .sklib
func FindLibrary(libsDir string, spec LibSpec) (*Library, string, error) {
	dir := fmt.Sprintf("%s/%s/%s",
		strings.TrimRight(libsDir, "/"), spec.Author, spec.Name)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("library not found: %s/%s (dir: %s)",
			spec.Author, spec.Name, dir)
	}

	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".sklib") {
			continue
		}
		candidates = append(candidates, dir+"/"+e.Name())
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("library %s/%s not found in %s (no .sklib files)",
			spec.Author, spec.Name, dir)
	}

	// Без версии — ровно 1 файл
	if spec.Version == "" {
		if len(candidates) > 1 {
			return nil, "", fmt.Errorf(
				"multiple versions of library %s/%s found in %s; specify @version",
				spec.Author, spec.Name, dir)
		}
		lib, err := ReadLibrary(candidates[0])
		if err != nil {
			return nil, "", fmt.Errorf("cannot read %s: %w", candidates[0], err)
		}
		return lib, candidates[0], nil
	}

	// С версией — читаем все и ищем совпадение
	var matches []*Library
	var matchFiles []string
	for _, file := range candidates {
		lib, err := ReadLibrary(file)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read %s: %w", file, err)
		}
		if lib.Version == spec.Version {
			matches = append(matches, lib)
			matchFiles = append(matchFiles, file)
		}
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("library %s/%s@%s not found in %s",
			spec.Author, spec.Name, spec.Version, dir)
	}
	if len(matches) > 1 {
		return nil, "", fmt.Errorf(
			"multiple files with version %s for library %s/%s in %s",
			spec.Version, spec.Author, spec.Name, dir)
	}

	return matches[0], matchFiles[0], nil
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
