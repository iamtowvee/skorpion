package front

import (
	"os"
	"path/filepath"
	"skrp/res/stdlib"
	"strings"
)

type ModuleResolver struct {
	BaseDir string
	Cache   map[string]string
}

func NewModuleResolver(baseDir string) *ModuleResolver {
	return &ModuleResolver{
		BaseDir: baseDir,
		Cache:   make(map[string]string),
	}
}

func (r *ModuleResolver) Resolve(path string) (string, bool, bool) {
	// Проверяем кеш
	if cached, ok := r.Cache[path]; ok {
		return cached, true, false
	}

	// 1. СНАЧАЛА ПРОВЕРЯЕМ ВСТРОЕННУЮ БИБЛИОТЕКУ!
	if stdlib.HasModule(path) {
		r.Cache[path] = path
		return path, true, true // builtin = true
	}

	var fullPath string
	var found bool

	// 2. Ищем локально относительно BaseDir
	localPath := filepath.Join(r.BaseDir, path+".sk")
	if _, err := os.Stat(localPath); err == nil {
		fullPath = localPath
		found = true
	}

	// 3. Ищем в подпапках
	if !found {
		parts := strings.Split(path, "/")
		for i := 1; i <= len(parts); i++ {
			subPath := filepath.Join(parts[:i]...)
			testPath := filepath.Join(r.BaseDir, subPath, path+".sk")
			if _, err := os.Stat(testPath); err == nil {
				fullPath = testPath
				found = true
				break
			}
		}
	}

	if found {
		r.Cache[path] = fullPath
		return fullPath, true, false
	}

	return "", false, false
}

func (r *ModuleResolver) SetBaseDir(dir string) {
	r.BaseDir = dir
}

func (r *ModuleResolver) ClearCache() {
	r.Cache = make(map[string]string)
}
