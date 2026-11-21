package front

import (
	"fmt"
	"os"
	"skrp/res/errors"
	"strings"
)

type Config struct {
	Name         string
	Version      string
	Authors      []string
	Target       []string
	Main         string
	BuildOutName string
	BuildOutPath string
	TestOutName  string
	Shifting     string
	TestOutPath  string
	LibsDir      string
	Dependencies []string
	Env          []string
	Execute      []string

	LinuxCCFlags   []string
	WindowsCCFlags []string
	DarwinCCFlags  []string

	Vars map[string]interface{}

	VarOrder []string
}

type presetKind int

const (
	presetString presetKind = iota
	presetArray
	presetStringOrArray
)

var presetKinds = map[string]presetKind{
	"name":         presetString,
	"version":      presetString,
	"main":         presetString,
	"buildOutName": presetString,
	"buildOutPath": presetString,
	"testOutName":  presetString,
	"testOutPath":  presetString,
	"libs":         presetString,
	"shifting":     presetString,

	"authors":        presetArray,
	"target":         presetArray,
	"dependencies":   presetArray,
	"linuxCCFlags":   presetArray,
	"windowsCCFlags": presetArray,
	"darwinCCFlags":  presetArray,

	"env":     presetStringOrArray,
	"execute": presetStringOrArray,
}

var presetNames = func() map[string]bool {
	m := make(map[string]bool, len(presetKinds))
	for k := range presetKinds {
		m[k] = true
	}
	return m
}()

func (c *Config) validatePresetTypes() error {
	for _, key := range c.VarOrder {
		kind, ok := presetKinds[key]
		if !ok {
			continue
		}
		switch v := c.Vars[key].(type) {
		case string:
			if kind == presetArray {
				return fmt.Errorf("config key '%s' must be an array: %s[(\"...\")]", key, key)
			}
			if key == "shifting" && strings.TrimSpace(v) == "" {
				return fmt.Errorf("config key 'shifting' must name a .skd file")
			}
		case []string:
			if kind == presetString {
				return fmt.Errorf("config key '%s' must be a single string: %s[\"...\"]", key, key)
			}
		default:
			return fmt.Errorf("config key '%s' has unsupported value type", key)
		}
	}
	return nil
}

func isValidKeyName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
				return false
			}
			continue
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func (c *Config) parseLine(line string, lineNum int, file string) error {
	openIdx := strings.Index(line, "[")
	if openIdx < 0 {
		return nil
	}

	key := strings.TrimSpace(line[:openIdx])
	if key == "" {
		return fmt.Errorf("empty key on line %d", lineNum)
	}
	if !isValidKeyName(key) {
		return fmt.Errorf(
			"invalid character in key '%s' (line %d)", key, lineNum)
	}

	rest := line[openIdx+1:]

	closeIdx := strings.LastIndex(rest, "]")
	if closeIdx < 0 {
		return fmt.Errorf("missing ']' on line %d", lineNum)
	}

	valueStr := rest[:closeIdx]

	// Дубликат — ошибка (для любого ключа).
	if _, exists := c.Vars[key]; exists {
		return fmt.Errorf("config key '%s' already declared", key)
	}

	value, err := c.parseValue(valueStr, key)
	if err != nil {
		return err
	}

	c.Vars[key] = value
	c.VarOrder = append(c.VarOrder, key)
	return nil
}

func (c *Config) resolveSubstitutions(s string, varName string) (string, error) {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '{' {
			end := strings.IndexByte(s[i+1:], '}')
			if end < 0 {
				return "", fmt.Errorf(
					"unterminated '{' in '%s'", varName)
			}
			name := s[i+1 : i+1+end]
			if name == "" {
				return "", fmt.Errorf(
					"empty variable name in '{}' in '%s'", varName)
			}

			val, ok := c.Vars[name]
			if !ok {
				return "", fmt.Errorf(
					"undefined variable '{%s}' in '%s'", name, varName)
			}

			strVal, ok := val.(string)
			if !ok {
				return "", fmt.Errorf(
					"cannot substitute '{%s}' (it's an array) in string '%s'",
					name, varName)
			}

			out.WriteString(strVal)
			i = i + 1 + end + 1
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String(), nil
}

func (c *Config) parseValue(v string, varName string) (interface{}, error) {
	v = strings.TrimSpace(v)

	// Массив
	if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") {
		inner := v[1 : len(v)-1]
		parts := splitArrayElements(inner)

		result := make([]string, 0, len(parts))
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			// &name
			if strings.HasPrefix(part, "&") {
				refName := part[1:]
				refVal, ok := c.Vars[refName]
				if !ok {
					return nil, fmt.Errorf(
						"variable '%s' is not declared above (in '%s', element %d)",
						refName, varName, i)
				}
				refStr, ok := refVal.(string)
				if !ok {
					return nil, fmt.Errorf(
						"'&%s' refers to a non-string variable (in '%s', element %d)",
						refName, varName, i)
				}
				result = append(result, refStr)
				continue
			}

			// Строка
			if !strings.HasPrefix(part, `"`) || !strings.HasSuffix(part, `"`) {
				return nil, fmt.Errorf(
					"expected string or &name in array (in '%s', element %d), got %q",
					varName, i, part)
			}
			s := part[1 : len(part)-1]
			resolved, err := c.resolveSubstitutions(s, varName)
			if err != nil {
				return nil, err
			}
			result = append(result, resolved)
		}

		return result, nil
	}

	// Строка
	if !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) {
		return nil, fmt.Errorf(
			"expected string or array in '%s', got %q", varName, v)
	}
	s := v[1 : len(v)-1]
	resolved, err := c.resolveSubstitutions(s, varName)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// splitArrayElements разбивает "(a, b, c)" на элементы, учитывая
// строки в кавычках (запятая внутри строки — не разделитель).
func splitArrayElements(s string) []string {
	var parts []string
	var buf strings.Builder
	inString := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' {
			inString = !inString
			buf.WriteByte(ch)
			continue
		}
		if ch == ',' && !inString {
			parts = append(parts, buf.String())
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

func (c *Config) syncPresetFields() {
	str := func(key string) (string, bool) {
		v, ok := c.Vars[key].(string)
		return v, ok
	}
	arr := func(key string) ([]string, bool) {
		switch v := c.Vars[key].(type) {
		case []string:
			return v, true
		case string:
			out := make([]string, 0)
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, p)
				}
			}
			return out, true
		}
		return nil, false
	}

	if v, ok := str("name"); ok {
		c.Name = v
	}
	if v, ok := str("version"); ok {
		c.Version = v
	}
	if v, ok := str("main"); ok {
		c.Main = v
	}
	if v, ok := str("buildOutName"); ok {
		c.BuildOutName = v
	}
	if v, ok := str("buildOutPath"); ok {
		c.BuildOutPath = v
	}
	if v, ok := str("testOutName"); ok {
		c.TestOutName = v
	}
	if v, ok := str("testOutPath"); ok {
		c.TestOutPath = v
	}
	if v, ok := str("libs"); ok {
		c.LibsDir = v
	}
	if v, ok := str("shifting"); ok {
		c.Shifting = strings.TrimSpace(v)
	}
	if v, ok := arr("authors"); ok {
		c.Authors = v
	}
	if v, ok := arr("target"); ok {
		c.Target = v
	}
	if v, ok := arr("dependencies"); ok {
		c.Dependencies = v
	}
	if v, ok := arr("env"); ok {
		c.Env = v
	}
	if v, ok := arr("linuxCCFlags"); ok {
		c.LinuxCCFlags = v
	}
	if v, ok := arr("windowsCCFlags"); ok {
		c.WindowsCCFlags = v
	}
	if v, ok := arr("darwinCCFlags"); ok {
		c.DarwinCCFlags = v
	}

	switch v := c.Vars["execute"].(type) {
	case []string:
		c.Execute = v
	case string:
		c.Execute = strings.Fields(v)
	}
}

func ParseConfig(path string) *Config {
	cfg := &Config{
		BuildOutPath: "bin/",
		TestOutPath:  "test/",
		LibsDir:      "libs/",
		Env:          []string{},
		Execute:      []string{},
		Vars:         make(map[string]interface{}),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		errors.NewError("0010", "Cannot read config file", 0, 0, path)
		return cfg
	}

	lines := strings.Split(string(data), "\n")
	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#!") {
			continue
		}
		if err := cfg.parseLine(line, lineNum, path); err != nil {
			errors.NewFatalError("0653", fmt.Sprintf("Config error: %v", err), lineNum, 0, path)
			return cfg
		}
	}

	if err := cfg.validatePresetTypes(); err != nil {
		errors.NewFatalError("0653", fmt.Sprintf("Config error: %v", err), 0, 0, path)
		return cfg
	}

	cfg.syncPresetFields()
	return cfg
}
