package front

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type LoadedModule struct {
	Path    string
	Program *Program
	Imports []string
}

type ImportManager struct {
	resolver      *ModuleResolver
	loaded        map[string]*LoadedModule
	importedFuncs []*Function
	mu            sync.Mutex
	baseDir       string
}

func NewImportManager(baseDir string) *ImportManager {
	return &ImportManager{
		resolver:      NewModuleResolver(baseDir),
		loaded:        make(map[string]*LoadedModule),
		importedFuncs: []*Function{},
		baseDir:       baseDir,
	}
}

// LoadMain загружает главный файл (не как модуль, а как entry point)
func (im *ImportManager) LoadMain(path string) (*Program, error) {
	// Сбрасываем кеш для новой сборки
	im.loaded = make(map[string]*LoadedModule)
	im.importedFuncs = []*Function{}
	im.baseDir = filepath.Dir(path)
	im.resolver.SetBaseDir(im.baseDir)
	im.resolver.ClearCache()

	// Читаем главный файл
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read main file %s: %v", path, err)
	}

	// Парсим главный файл
	parser := NewParser(string(content))
	mainProg := parser.Parse()

	if mainProg == nil {
		return nil, fmt.Errorf("failed to parse main file: %s", path)
	}

	// Загружаем импорты из главного файла
	visited := make(map[string]bool)
	for _, imp := range mainProg.Imports {
		subProg, err := im.loadModule(imp.Path, visited)
		if err != nil {
			return nil, err
		}
		// Сохраняем функции из импорта отдельно
		im.importedFuncs = append(im.importedFuncs, subProg.Functions...)
	}

	return mainProg, nil
}

// loadModule загружает модуль (внутренний метод)
func (im *ImportManager) loadModule(path string, visited map[string]bool) (*Program, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Проверяем циклические импорты
	if visited[path] {
		return nil, fmt.Errorf("circular import detected: %s", path)
	}

	// Проверяем, загружен ли уже модуль
	if loaded, ok := im.loaded[path]; ok {
		return loaded.Program, nil
	}

	// Резолвим путь
	fullPath, found := im.resolver.Resolve(path)
	if !found {
		return nil, fmt.Errorf("module not found: %s (searched in %s)", path, im.baseDir)
	}

	// Читаем файл
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read module %s: %v", path, err)
	}

	// Парсим
	parser := NewParser(string(content))
	prog := parser.Parse()

	if prog == nil {
		return nil, fmt.Errorf("failed to parse module: %s", path)
	}

	// Загружаем вложенные импорты
	visited[path] = true
	for _, imp := range prog.Imports {
		subProg, err := im.loadModule(imp.Path, visited)
		if err != nil {
			return nil, err
		}
		// Мержим функции из вложенных импортов
		prog.Functions = append(prog.Functions, subProg.Functions...)
	}
	delete(visited, path)

	// Сохраняем в кеш
	im.loaded[path] = &LoadedModule{
		Path:    fullPath,
		Program: prog,
		Imports: getImportPaths(prog),
	}

	return prog, nil
}

// LoadModule публичный метод для загрузки модуля
func (im *ImportManager) LoadModule(path string) (*Program, error) {
	visited := make(map[string]bool)
	return im.loadModule(path, visited)
}

func (im *ImportManager) GetModule(path string) (*LoadedModule, bool) {
	im.mu.Lock()
	defer im.mu.Unlock()
	mod, ok := im.loaded[path]
	return mod, ok
}

// GetAllFunctions возвращает функции из импортированных модулей
func (im *ImportManager) GetAllFunctions() []*Function {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.importedFuncs
}

func getImportPaths(prog *Program) []string {
	paths := make([]string, len(prog.Imports))
	for i, imp := range prog.Imports {
		paths[i] = imp.Path
	}
	return paths
}

// Получение имени модуля из пути
func GetModuleName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// Проверка, экспортируемая ли функция
func IsExportable(fn *Function) bool {
	return fn.IsExport
}
