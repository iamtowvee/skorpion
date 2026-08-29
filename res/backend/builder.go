package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"skrp/res/errors"
	"strings"
)

type Builder struct {
	Config     *BuildConfig
	CSource    string
	OutputPath string
}

type BuildConfig struct {
	Profile      string // auto, tcc, gcc, clang
	CompilerPath string
	OutputName   string
	OutputDir    string
	IsTest       bool
}

func NewBuilder(config *BuildConfig) *Builder {
	return &Builder{
		Config: config,
	}
}

func (b *Builder) Build(ir *IRProgram) bool {
	// 1. Генерируем C код
	gen := NewCodeGenerator(ir)
	b.CSource = gen.Generate()

	// 2. Сохраняем C файл
	cFile := filepath.Join(b.Config.OutputDir, "temp_skorpion.c")
	if err := os.WriteFile(cFile, []byte(b.CSource), 0644); err != nil {
		errors.NewError("2001", fmt.Sprintf("Cannot write C file: %v", err), 0, 0, "")
		return false
	}

	// 3. Находим компилятор
	compiler := b.findCompiler()
	if compiler == "" {
		errors.NewError("2002", "No C compiler found. Please install tcc, gcc, or clang", 0, 0, "")
		return false
	}

	fmt.Printf("Using compiler: %s\n", compiler)

	// 4. Собираем бинарник
	outFile := filepath.Join(b.Config.OutputDir, b.Config.OutputName)
	if runtime.GOOS == "windows" {
		outFile += ".exe"
	}

	// Формируем команду
	var cmd *exec.Cmd
	if b.Config.Profile == "tcc" || strings.Contains(strings.ToLower(compiler), "tcc") {
		// TCC: tcc file.c -o output.exe
		cmd = exec.Command(compiler, cFile, "-o", outFile)
	} else {
		// GCC/Clang: gcc file.c -o output
		cmd = exec.Command(compiler, cFile, "-o", outFile)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		errors.NewError("2003", fmt.Sprintf("Compilation failed: %v", err), 0, 0, "")
		return false
	}

	// 5. Удаляем временный C файл
	os.Remove(cFile)

	fmt.Printf("Build successful: %s\n", outFile)
	return true
}

func (b *Builder) findCompiler() string {
	// 1. Если указан конкретный профиль
	if b.Config.Profile != "auto" {
		// Проверяем переданный путь
		if b.Config.CompilerPath != "" {
			// Проверяем в PATH
			if path, err := exec.LookPath(b.Config.CompilerPath); err == nil {
				return path
			}
			// Windows с .exe
			if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(b.Config.CompilerPath), ".exe") {
				if path, err := exec.LookPath(b.Config.CompilerPath + ".exe"); err == nil {
					return path
				}
			}
			// Проверяем как полный путь
			if _, err := os.Stat(b.Config.CompilerPath); err == nil {
				return b.Config.CompilerPath
			}
		}
	}

	// 2. Проверяем TCC в libs (встроенный)
	if runtime.GOOS == "windows" {
		tccPath := filepath.Join("libs", "win", "tcc.exe")
		if _, err := os.Stat(tccPath); err == nil {
			return tccPath
		}
	} else {
		tccPath := filepath.Join("libs", "linux", "tcc")
		if _, err := os.Stat(tccPath); err == nil {
			return tccPath
		}
	}

	// 3. Ищем в PATH с приоритетом: tcc, gcc, clang
	compilers := []string{"tcc", "gcc", "clang"}

	for _, compiler := range compilers {
		// Проверяем в PATH
		if path, err := exec.LookPath(compiler); err == nil {
			return path
		}
		// Windows с .exe
		if runtime.GOOS == "windows" {
			if path, err := exec.LookPath(compiler + ".exe"); err == nil {
				return path
			}
		}
	}

	return ""
}
