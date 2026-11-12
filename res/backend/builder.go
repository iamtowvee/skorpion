package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"skrp/res/errors"
)

type Builder struct {
	Config  *BuildConfig
	CSource string
}

type BuildConfig struct {
	ProfileManager interface {
		GetCompilerForOS(osName string) string
	}
	Targets    []string
	OutputName string
	OutputDir  string
	IsTest     bool
}

func NewBuilder(config *BuildConfig) *Builder {
	return &Builder{Config: config}
}

func (b *Builder) Build(ir *IRProgram) bool {
	gen := NewCodeGenerator(ir)
	b.CSource = gen.Generate()

	cFile := filepath.Join(b.Config.OutputDir, "temp_skorpion.c")
	if err := os.WriteFile(cFile, []byte(b.CSource), 0644); err != nil {
		errors.NewError("0020", fmt.Sprintf("Cannot write C file: %v", err), 0, 0, "")
		return false
	}
	defer os.Remove(cFile)

	targets := b.Config.Targets
	if len(targets) == 0 {
		targets = []string{runtime.GOOS}
	}

	anySuccess := false
	for _, targetOS := range targets {
		if b.buildForTarget(targetOS, cFile) {
			anySuccess = true
		}
	}

	return anySuccess
}

func (b *Builder) buildForTarget(targetOS string, cFile string) bool {
	normOS := normalizeOS(targetOS)

	var compilerPath string
	if b.Config.ProfileManager != nil {
		compilerPath = b.Config.ProfileManager.GetCompilerForOS(normOS)
	}

	if compilerPath == "" || compilerPath == "auto" {
		compilerPath = b.findCompilerForOS(normOS)
	}

	if compilerPath == "" {
		errors.NewError("3001",
			fmt.Sprintf("Compiler not found for target '%s'", targetOS),
			0, 0, "")
		return false
	}

	cmdName, cmdArgs := splitCompilerCommand(compilerPath)

	var flags []string
	switch normOS {
	case "windows":
		flags = append(flags, "-DSK_OS_WINDOWS=1")
	case "linux":
		flags = append(flags, "-DSK_OS_LINUX=1", "-lm", "-lpthread")
	case "darwin":
		flags = append(flags, "-DSK_OS_DARWIN=1", "-lm", "-lpthread")
	}

	outFile := filepath.Join(b.Config.OutputDir, b.Config.OutputName)
	if normOS == "windows" {
		outFile += ".exe"
	}

	isWSL := cmdName == "wsl"

	actualCFile := cFile
	actualOutFile := outFile
	if isWSL {
		actualCFile = windowsToWSLPath(cFile)
		actualOutFile = windowsToWSLPath(outFile)
	}

	finalArgs := append(cmdArgs, actualCFile, "-o", actualOutFile)
	finalArgs = append(finalArgs, flags...)

	cmd := exec.Command(cmdName, finalArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		errors.NewError("3000", fmt.Sprintf("Compilation failed for %s: %v", targetOS, err), 0, 0, "")
		return false
	}

	fmt.Printf("Built for %s: %s\n", targetOS, outFile)
	return true
}

func normalizeOS(osName string) string {
	switch osName {
	case "win", "windows":
		return "windows"
	case "linux":
		return "linux"
	case "darwin":
		return "darwin"
	}
	return osName
}

// splitCompilerCommand разбирает строку-путь на имя команды и аргументы.
//
//	"gcc"                     → ("gcc", nil)
//	"wsl gcc"                 → ("wsl", ["gcc"])
//	"C:\Program Files\...\cc" → ("C:\Program Files\...\cc", nil)
//
// Общая часть, ранее дублировавшаяся в buildForTarget и compilerExists.
func splitCompilerCommand(path string) (string, []string) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		if _, err := os.Stat(path + ".exe"); err == nil {
			return path + ".exe", nil
		}
	}
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return path, nil
	}
	cmd := parts[0]
	if len(parts) > 1 {
		return cmd, parts[1:]
	}
	return cmd, nil
}

// findCompilerForOS — фолбэк, когда профиль auto.
func (b *Builder) findCompilerForOS(targetOS string) string {
	if targetOS == runtime.GOOS {
		return b.findLocalCompiler()
	}

	switch targetOS {
	case "windows":
		for _, c := range []string{
			"x86_64-w64-mingw32-gcc",
			"i686-w64-mingw32-gcc",
			"mingw32-gcc",
		} {
			if path, err := exec.LookPath(c); err == nil {
				return path
			}
		}
	case "linux":
		for _, c := range []string{
			"x86_64-linux-gnu-gcc",
			"gcc-linux",
		} {
			if path, err := exec.LookPath(c); err == nil {
				return path
			}
		}
		if _, err := exec.LookPath("wsl"); err == nil {
			return "wsl gcc"
		}
	}

	return ""
}

// findLocalCompiler — для текущей ОС.
func (b *Builder) findLocalCompiler() string {
	compilers := []string{"gcc", "clang", "tcc"}
	for _, c := range compilers {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
		if runtime.GOOS == "windows" {
			if path, err := exec.LookPath(c + ".exe"); err == nil {
				return path
			}
		}
	}

	var tccPath string
	if runtime.GOOS == "windows" {
		tccPath = filepath.Join("libs", "win", "tcc.exe")
	} else {
		tccPath = filepath.Join("libs", "linux", "tcc")
	}
	if _, err := os.Stat(tccPath); err == nil {
		return tccPath
	}

	return ""
}

// windowsToWSLPath конвертирует Windows-путь в WSL-путь:
// "F:\iats\skorpion\bin\temp.c" → "/mnt/f/iats/skorpion/bin/temp.c"
func windowsToWSLPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = strings.ReplaceAll(abs, "\\", "/")
	if len(abs) >= 2 && abs[1] == ':' {
		drive := strings.ToLower(string(abs[0]))
		abs = "/mnt/" + drive + abs[2:]
	}
	return abs
}
