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
	TestOutPath  string
	LibsDir      string
	Dependencies []string
	Env          []string
	Execute      []string

	// Vars — все значения: ключ без @.
	// Значение: string ИЛИ []string.
	Vars map[string]interface{}

	// VarOrder — порядок объявления.
	VarOrder []string
}

var presetNames = map[string]bool{
	"name": true, "version": true, "authors": true, "target": true,
	"main": true, "buildOutName": true, "buildOutPath": true,
	"testOutName": true, "testOutPath": true, "libs": true,
	"dependencies": true, "env": true, "execute": true,
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

	// Пустой ключ — ошибка
	if key == "" {
		return fmt.Errorf("empty key on line %d", lineNum)
	}

	// Невалидные символы в ключе — ошибка
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

	// Пустой ключ — ошибка
	if key == "" {
		return fmt.Errorf("empty key on line %d", lineNum)
	}

	// Дубликат — ошибка (для любого ключа)
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
				refName := part[1:] // без @
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
	if v, ok := c.Vars["name"].(string); ok {
		c.Name = v
	}
	if v, ok := c.Vars["version"].(string); ok {
		c.Version = v
	}
	if v, ok := c.Vars["main"].(string); ok {
		c.Main = v
	}
	if v, ok := c.Vars["buildOutName"].(string); ok {
		c.BuildOutName = v
	}
	if v, ok := c.Vars["buildOutPath"].(string); ok {
		c.BuildOutPath = v
	}
	if v, ok := c.Vars["testOutName"].(string); ok {
		c.TestOutName = v
	}
	if v, ok := c.Vars["testOutPath"].(string); ok {
		c.TestOutPath = v
	}
	if v, ok := c.Vars["libs"].(string); ok {
		c.LibsDir = v
	}

	if v, ok := c.Vars["authors"].([]string); ok {
		c.Authors = v
	}
	if v, ok := c.Vars["target"].([]string); ok {
		c.Target = v
	}
	if v, ok := c.Vars["dependencies"].([]string); ok {
		c.Dependencies = v
	}

	if v, ok := c.Vars["env"].(string); ok {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				c.Env = append(c.Env, p)
			}
		}
	}
	if v, ok := c.Vars["execute"].(string); ok {
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
			errors.NewFatalError("0653",
				fmt.Sprintf("Config error: %v", err),
				lineNum, 0, path)
			return cfg
		}
	}

	cfg.syncPresetFields()
	return cfg
}
