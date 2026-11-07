package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"skrp/res"
	"skrp/res/errors"
	"skrp/res/log"
)

const version = "1.0.0"

type manifestLight struct {
	Name    string
	Version string
}

func readManifestLight(path string) manifestLight {
	var m manifestLight
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if v, ok := parseManifestKV(line, "name"); ok {
			m.Name = v
		} else if v, ok := parseManifestKV(line, "version"); ok {
			m.Version = v
		}
	}
	if err := sc.Err(); err != nil {
	}
	return m
}

func parseManifestKV(line, key string) (string, bool) {
	if !strings.HasPrefix(line, key+"[") {
		return "", false
	}
	rest := line[len(key)+1:]
	end := strings.LastIndex(rest, "]")
	if end < 0 {
		return "", false
	}
	val := strings.TrimSpace(rest[:end])
	val = strings.Trim(val, `"`)
	return val, true
}

// preParseLogDir — выставляет SK_LOG_DIR/SK_LOCAL_LOG из --log-dir/--local-log
// ДО log.Start(), потому что log.Start читает env.
func preParseLogDir() {
	for i, a := range os.Args {
		switch {
		case a == "--log-dir" && i+1 < len(os.Args):
			os.Setenv("SK_LOG_DIR", os.Args[i+1])
		case strings.HasPrefix(a, "--log-dir="):
			os.Setenv("SK_LOG_DIR", strings.TrimPrefix(a, "--log-dir="))
		case a == "--local-log":
			os.Setenv("SK_LOCAL_LOG", "1")
		}
	}
}

func main() {
	preParseLogDir()

	projectPath := "."
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "--path=") {
			projectPath = strings.TrimPrefix(a, "--path=")
			break
		}
	}
	cfg := readManifestLight(filepath.Join(projectPath, "manifest.spc"))

	if isNoLogCommand(os.Args[1:]) {
		log.StartReadOnly(cfg.Name, cfg.Version, projectPath)
	} else {
		log.Start(cfg.Name, cfg.Version, projectPath)
	}
	log.SetVersion(version)

	defer func() {
		if r := recover(); r != nil {
			log.WriteCrash(r, debug.Stack())
			log.SetExitCode(2)
			log.Finalize(errors.WriteAllTo)
			log.PrintCrashBanner(os.Stderr, r, version)
			os.Exit(2)
		}
		code := 0
		if errors.Count() > 0 || errors.HasFatal() || res.CommandFailed() {
			code = 1
		}
		log.SetExitCode(code)
		log.Finalize(errors.WriteAllTo)
		if code != 0 {
			log.PrintLogPath(os.Stderr)
			os.Exit(code)
		}
	}()

	res.InitLang(os.Args[1:], version)
}

func isNoLogCommand(args []string) bool {
	for _, a := range args {
		if a == "clear-cache" || a == "clear-local" {
			return true
		}
	}
	return false
}
