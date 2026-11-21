package skpkg

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

// ShiftingEntry — одно правило .skd: "source" -> "destDir/";
type ShiftingEntry struct {
	Source  string
	DestDir string
	Line    int
	Column  int
}

func (e ShiftingEntry) OutPattern() string {
	return e.DestDir + path.Base(e.Source)
}

// CCFlagsDirective — директива #linuxCCFlags, #windowsCCFlags, #darwinCCFlags.
type CCFlagsDirective struct {
	OS     string // "linux", "windows", "darwin"
	Flags  []string
	Line   int
	Column int
}

// ShiftingFile — весь .skd файл: директивы + правила.
type ShiftingFile struct {
	Directives []CCFlagsDirective
	Entries    []ShiftingEntry
}

var consumerVariables = map[string]bool{
	"name":         true,
	"version":      true,
	"buildOutPath": true,
	"testOutPath":  true,
	"libs":         true,
}

func AllowedVariables() []string {
	out := make([]string, 0, len(consumerVariables))
	for k := range consumerVariables {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ExtensionClass — класс расширения файла.
type ExtensionClass int

const (
	ExtStandard ExtensionClass = iota // .c .h .a .sk .txt .md
	ExtWarn                           // .hpp .cpp .cc .o .lib .so .dylib
	ExtDanger                         // остальные
)

// ClassifyExtension классифицирует расширение файла.
func ClassifyExtension(name string) ExtensionClass {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".c", ".h", ".a", ".sk", ".txt", ".md":
		return ExtStandard
	case ".hpp", ".cpp", ".cc", ".o", ".lib", ".so", ".dylib":
		return ExtWarn
	default:
		return ExtDanger
	}
}

// IsStandardExtension — файлы этих типов ставятся без предупреждения.
func IsStandardExtension(name string) bool {
	return ClassifyExtension(name) == ExtStandard
}

// ParseShiftingFile читает и полностью валидирует .skd.
func ParseShiftingFile(filePath string) (*ShiftingFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read shifting script '%s': %w", filePath, err)
	}
	return ParseShifting(string(data), filePath)
}

// ParseShifting разбирает содержимое .skd.
func ParseShifting(src, file string) (*ShiftingFile, error) {
	p := &skdParser{src: src, file: file, line: 1, col: 1}

	var directives []CCFlagsDirective
	var entries []ShiftingEntry
	seenRule := make(map[string]int)
	seenOut := make(map[string]int)
	seenDirective := make(map[string]int)

	for {
		p.skipTrivia()
		if p.eof() {
			break
		}

		line, col := p.line, p.col

		// Директива?
		if p.peek() == '#' && !strings.HasPrefix(p.src[p.off:], "#!") {
			directive, err := p.parseDirective()
			if err != nil {
				return nil, err
			}
			if prev, ok := seenDirective[directive.OS]; ok {
				return nil, fmt.Errorf("%s:%d:%d: duplicate directive #%s (already declared on line %d)",
					file, line, col, directive.OS, prev)
			}
			seenDirective[directive.OS] = line
			directives = append(directives, directive)
			continue
		}

		// Правило.
		source, err := p.quotedString()
		if err != nil {
			return nil, err
		}
		p.skipTrivia()
		if err := p.literal("->"); err != nil {
			return nil, err
		}
		p.skipTrivia()
		dest, err := p.quotedString()
		if err != nil {
			return nil, err
		}
		p.skipTrivia()
		if err := p.literal(";"); err != nil {
			return nil, err
		}

		if err := validateShiftingSource(source); err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", file, line, col, err)
		}
		if !strings.HasSuffix(dest, "/") {
			return nil, fmt.Errorf(
				"%s:%d:%d: destination '%s' must be a directory ending with '/' "+
					"(a .skd script cannot rename files)", file, line, col, dest)
		}
		if err := ValidateOutPattern(dest); err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", file, line, col, err)
		}

		ruleKey := source + "\x00" + dest
		if prev, ok := seenRule[ruleKey]; ok {
			return nil, fmt.Errorf("%s:%d:%d: duplicate rule (already declared on line %d)",
				file, line, col, prev)
		}
		seenRule[ruleKey] = line

		entry := ShiftingEntry{Source: source, DestDir: dest, Line: line, Column: col}
		out := entry.OutPattern()
		outKey := strings.ToLower(out)
		if prev, ok := seenOut[outKey]; ok {
			return nil, fmt.Errorf(
				"%s:%d:%d: destination '%s' is already produced by the rule on line %d",
				file, line, col, out, prev)
		}
		seenOut[outKey] = line

		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].OutPattern() < entries[j].OutPattern()
	})
	return &ShiftingFile{Directives: directives, Entries: entries}, nil
}

func validateShiftingSource(s string) error {
	if s == "" {
		return fmt.Errorf("empty source path")
	}
	if strings.ContainsRune(s, '%') {
		return fmt.Errorf(
			"source path '%s' must not contain %%variables "+
				"(they are resolved on the consumer side and allowed in the destination only)", s)
	}
	if err := CheckRelativePath(s); err != nil {
		return fmt.Errorf("source path '%s': %w", s, err)
	}
	if strings.HasSuffix(s, "/") {
		return fmt.Errorf("source '%s' must be a file, not a directory", s)
	}
	return nil
}

func ValidateOutPattern(out string) error {
	if out == "" {
		return fmt.Errorf("empty destination")
	}
	if strings.ContainsRune(out, '\\') {
		return fmt.Errorf("destination '%s': backslashes are not allowed, use '/'", out)
	}
	if strings.HasPrefix(out, "/") {
		return fmt.Errorf("destination '%s' must be relative to the consumer project", out)
	}
	if strings.ContainsRune(out, ':') {
		return fmt.Errorf("destination '%s': ':' is not allowed", out)
	}
	for _, r := range out {
		if r < 0x20 || r == 0x7f || r == 0 {
			return fmt.Errorf("destination '%s': control characters are not allowed", out)
		}
	}

	rest := out
	for {
		i := strings.IndexByte(rest, '%')
		if i < 0 {
			break
		}
		name, n := readVarName(rest[i+1:])
		if n == 0 {
			return fmt.Errorf("destination '%s': malformed %%variable", out)
		}
		if !consumerVariables[name] {
			return fmt.Errorf("destination '%s': variable '%%%s' is not allowed (allowed: %s)",
				out, name, strings.Join(AllowedVariables(), ", "))
		}
		rest = rest[i+1+n:]
	}

	skeleton := strings.TrimSuffix(varSkeleton(out), "/")
	if skeleton == "" {
		return fmt.Errorf("destination '%s': installing into the project root is not allowed", out)
	}
	if path.Clean(skeleton) != skeleton {
		return fmt.Errorf("destination '%s' is not normalized ('.' and '..' are not allowed)", out)
	}
	for _, seg := range strings.Split(skeleton, "/") {
		if seg == "" {
			return fmt.Errorf("destination '%s': empty path segment", out)
		}
		if strings.ContainsAny(seg, `<>|"*?`) {
			return fmt.Errorf("destination '%s': forbidden characters in path segment '%s'", out, seg)
		}
	}
	return nil
}

func ResolveOutPattern(out string, vars map[string]string) (string, error) {
	if err := ValidateOutPattern(out); err != nil {
		return "", err
	}

	var b strings.Builder
	rest := out
	for {
		i := strings.IndexByte(rest, '%')
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		name, n := readVarName(rest[i+1:])
		if n == 0 || !consumerVariables[name] {
			return "", fmt.Errorf("destination '%s': variable '%%%s' is not allowed", out, name)
		}
		value, ok := vars[name]
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf(
				"destination '%s': the consumer manifest does not define '%s'", out, name)
		}
		b.WriteString(value)
		rest = rest[i+1+n:]
	}

	resolved := b.String()
	clean := path.Clean(resolved)
	if err := CheckRelativePath(clean); err != nil {
		return "", fmt.Errorf("destination '%s' resolves to unsafe path '%s': %w", out, resolved, err)
	}
	return clean, nil
}

func readVarName(s string) (string, int) {
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case i > 0 && c >= '0' && c <= '9':
		default:
			if i == 0 {
				return "", 0
			}
			return s[:i], i
		}
		i++
	}
	if i == 0 {
		return "", 0
	}
	return s[:i], i
}

func varSkeleton(s string) string {
	var b strings.Builder
	rest := s
	for {
		i := strings.IndexByte(rest, '%')
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i])
		_, n := readVarName(rest[i+1:])
		if n == 0 {
			b.WriteByte('%')
			rest = rest[i+1:]
			continue
		}
		b.WriteString("v")
		rest = rest[i+1+n:]
	}
}

// ── лексер .skd ─────────────────────────────────────────────────────────────

type skdParser struct {
	src  string
	file string
	off  int
	line int
	col  int
}

func (p *skdParser) eof() bool { return p.off >= len(p.src) }

func (p *skdParser) peek() rune {
	if p.eof() {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(p.src[p.off:])
	return r
}

func (p *skdParser) advance() rune {
	if p.eof() {
		return 0
	}
	r, w := utf8.DecodeRuneInString(p.src[p.off:])
	p.off += w
	if r == '\n' {
		p.line++
		p.col = 1
	} else {
		p.col++
	}
	return r
}

func (p *skdParser) skipTrivia() {
	for !p.eof() {
		r := p.peek()
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			p.advance()
			continue
		}
		if r == '#' && strings.HasPrefix(p.src[p.off:], "#!") {
			for !p.eof() && p.peek() != '\n' {
				p.advance()
			}
			continue
		}
		return
	}
}

func (p *skdParser) literal(s string) error {
	line, col := p.line, p.col
	for _, want := range s {
		if p.eof() || p.peek() != want {
			return fmt.Errorf("%s:%d:%d: expected '%s'", p.file, line, col, s)
		}
		p.advance()
	}
	return nil
}

func (p *skdParser) quotedString() (string, error) {
	line, col := p.line, p.col
	if p.peek() != '"' {
		return "", fmt.Errorf("%s:%d:%d: expected a quoted string", p.file, line, col)
	}
	p.advance()

	var b strings.Builder
	for {
		if p.eof() {
			return "", fmt.Errorf("%s:%d:%d: unterminated string", p.file, line, col)
		}
		r := p.advance()
		switch r {
		case '"':
			return b.String(), nil
		case '\n':
			return "", fmt.Errorf("%s:%d:%d: newline inside a string", p.file, p.line, p.col)
		case '\\':
			if p.eof() {
				return "", fmt.Errorf("%s:%d:%d: unterminated escape sequence", p.file, line, col)
			}
			e := p.advance()
			if e != '\\' && e != '"' {
				return "", fmt.Errorf(
					"%s:%d:%d: unsupported escape '\\%c' (only \\\\ and \\\" are allowed)",
					p.file, p.line, p.col, e)
			}
			b.WriteRune(e)
		default:
			if r < 0x20 || r == 0x7f {
				return "", fmt.Errorf("%s:%d:%d: control character inside a string", p.file, p.line, p.col)
			}
			b.WriteRune(r)
		}
	}
}

// parseDirective разбирает #name ("...", "...").
func (p *skdParser) parseDirective() (CCFlagsDirective, error) {
	line, col := p.line, p.col
	if p.peek() != '#' {
		return CCFlagsDirective{}, fmt.Errorf("%s:%d:%d: expected '#'", p.file, line, col)
	}
	p.advance()

	name, err := p.identifier()
	if err != nil {
		return CCFlagsDirective{}, err
	}

	var osName string
	switch name {
	case "linuxCCFlags":
		osName = "linux"
	case "windowsCCFlags":
		osName = "windows"
	case "darwinCCFlags":
		osName = "darwin"
	default:
		return CCFlagsDirective{}, fmt.Errorf(
			"%s:%d:%d: unknown directive '#%s' (allowed: #linuxCCFlags, #windowsCCFlags, #darwinCCFlags)",
			p.file, line, col, name)
	}

	p.skipTrivia()
	flags, err := p.stringArray()
	if err != nil {
		return CCFlagsDirective{}, err
	}

	return CCFlagsDirective{OS: osName, Flags: flags, Line: line, Column: col}, nil
}

// identifier читает имя (буквы + цифры + '_').
func (p *skdParser) identifier() (string, error) {
	line, col := p.line, p.col
	var b strings.Builder
	for !p.eof() {
		r := p.peek()
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' {
			b.WriteRune(r)
			p.advance()
			continue
		}
		if b.Len() > 0 && r >= '0' && r <= '9' {
			b.WriteRune(r)
			p.advance()
			continue
		}
		break
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("%s:%d:%d: expected identifier", p.file, line, col)
	}
	return b.String(), nil
}

// stringArray читает ("...", "...").
func (p *skdParser) stringArray() ([]string, error) {
	line, col := p.line, p.col
	if p.peek() != '(' {
		return nil, fmt.Errorf("%s:%d:%d: expected '('", p.file, line, col)
	}
	p.advance()

	var flags []string
	for {
		p.skipTrivia()
		if p.peek() == ')' {
			p.advance()
			return flags, nil
		}
		if len(flags) > 0 {
			if p.peek() != ',' {
				return nil, fmt.Errorf("%s:%d:%d: expected ',' or ')'", p.file, p.line, p.col)
			}
			p.advance()
			p.skipTrivia()
		}
		flag, err := p.quotedString()
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}
}
