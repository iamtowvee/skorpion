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
}

func ParseConfig(path string) *Config {
	cfg := &Config{
		BuildOutPath: "bin/",
		TestOutPath:  "test/",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		errors.NewError("1002", "Cannot read config file", 0, 0, path)
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
				// array: <"win", "linux">
				value = strings.Trim(value, "<>")
				parts := strings.Split(value, ",")
				for _, p := range parts {
					cfg.Target = append(cfg.Target, strings.Trim(strings.Trim(p, `"`), " "))
				}
			case "authors":
				value = strings.Trim(value, "<>")
				parts := strings.Split(value, ",")
				for _, p := range parts {
					cfg.Authors = append(cfg.Authors, strings.Trim(strings.Trim(p, `"`), " "))
				}
			}
		}
	}

	return cfg
}
