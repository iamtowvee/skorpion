package front

import (
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

	// Env — переменные окружения вида "SOME=15"
	Env []string

	// Execute — дополнительные аргументы CLI, добавляемые в конец команды
	Execute []string
}

func ParseConfig(path string) *Config {
	cfg := &Config{
		BuildOutPath: "bin/",
		TestOutPath:  "test/",
		Env:          []string{},
		Execute:      []string{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		errors.NewError("0010", "Cannot read config file", 0, 0, path)
		return cfg
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// key[value]
		if strings.Contains(line, "[") && strings.Contains(line, "]") {
			key := strings.TrimSpace(line[:strings.Index(line, "[")])
			value := strings.TrimSpace(line[strings.Index(line, "[")+1 : strings.LastIndex(line, "]")])

			switch key {
			case "name":
				cfg.Name = strings.Trim(value, `"`)
			case "version":
				cfg.Version = strings.Trim(value, `"`)
			case "main":
				cfg.Main = strings.Trim(value, `"`)
			case "buildOutName":
				cfg.BuildOutName = strings.Trim(value, `"`)
			case "buildOutPath":
				cfg.BuildOutPath = strings.Trim(value, `"`)
			case "testOutName":
				cfg.TestOutName = strings.Trim(value, `"`)
			case "testOutPath":
				cfg.TestOutPath = strings.Trim(value, `"`)
			case "target":
				value = strings.Trim(value, "<>")
				parts := strings.Split(value, ",")
				for _, p := range parts {
					trimmed := strings.TrimSpace(p)
					trimmed = strings.Trim(trimmed, `"`)
					trimmed = strings.TrimSpace(trimmed)
					if trimmed != "" {
						cfg.Target = append(cfg.Target, trimmed)
					}
				}
			case "authors":
				value = strings.Trim(value, "<>")
				parts := strings.Split(value, ",")
				for _, p := range parts {
					trimmed := strings.TrimSpace(p)
					trimmed = strings.Trim(trimmed, `"`)
					trimmed = strings.TrimSpace(trimmed)
					if trimmed != "" {
						cfg.Authors = append(cfg.Authors, trimmed)
					}
				}
			case "env":
				// env["SOME=15"]
				// env["SOME=15,NODE_ENV=production"]
				value = strings.Trim(value, `"`)
				parts := strings.Split(value, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						cfg.Env = append(cfg.Env, p)
					}
				}
			case "execute":
				// execute["--uncolored --no-optimize"]
				value = strings.Trim(value, `"`)
				// Разбиваем по пробелам
				parts := strings.Fields(value)
				cfg.Execute = append(cfg.Execute, parts...)
			}
		}
	}

	return cfg
}
