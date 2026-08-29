package front

import (
	"os"
	"path/filepath"
	"strings"
)

type ModuleResolver struct {
	BaseDir string
	LibsDir string
	Cache   map[string]string
}

func NewModuleResolver(baseDir string) *ModuleResolver {
	return &ModuleResolver{
		BaseDir: baseDir,
		LibsDir: filepath.Join("libs", "std"),
		Cache:   make(map[string]string),
	}
}

func (r *ModuleResolver) Resolve(path string) (string, bool) {
	// Проверяем кеш
	if cached, ok := r.Cache[path]; ok {
		return cached, true
	}

	// 1. Поиск относительно текущей директории
	localPath := filepath.Join(r.BaseDir, path+".sk")
	if _, err := os.Stat(localPath); err == nil {
		r.Cache[path] = localPath
		return localPath, true
	}

	// 2. Поиск с учётом вложенных папок (path/to/module)
	// Пробуем разные варианты
	parts := strings.Split(path, "/")

	// 2a. Ищем как полный путь от BaseDir
	for i := 1; i <= len(parts); i++ {
		subPath := filepath.Join(parts[:i]...)
		fullPath := filepath.Join(r.BaseDir, subPath, path+".sk")
		if _, err := os.Stat(fullPath); err == nil {
			r.Cache[path] = fullPath
			return fullPath, true
		}
	}

	// 3. Поиск в libs/std
	stdPath := filepath.Join(r.LibsDir, path+".sk")
	if _, err := os.Stat(stdPath); err == nil {
		r.Cache[path] = stdPath
		return stdPath, true
	}

	// 4. Поиск в libs/std с вложенными папками
	for i := 1; i <= len(parts); i++ {
		subPath := filepath.Join(parts[:i]...)
		fullPath := filepath.Join(r.LibsDir, subPath, path+".sk")
		if _, err := os.Stat(fullPath); err == nil {
			r.Cache[path] = fullPath
			return fullPath, true
		}
	}

	return "", false
}

func (r *ModuleResolver) SetBaseDir(dir string) {
	r.BaseDir = dir
}

func (r *ModuleResolver) ClearCache() {
	r.Cache = make(map[string]string)
}
