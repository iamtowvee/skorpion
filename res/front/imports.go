package front

import (
	"fmt"
	"os"
	"path/filepath"
	"skrp/res/stdlib"
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

	// Парсим главный файл С ИМЕНЕМ ФАЙЛА!
	parser := NewParserWithFile(string(content), path)
	mainProg := parser.Parse()

	if mainProg == nil {
		return nil, fmt.Errorf("failed to parse main file: %s", path)
	}

	// Загружаем импорты
	visited := make(map[string]bool)
	for _, imp := range mainProg.Imports {
		subProg, err := im.loadModuleInternal(imp.Path, visited)
		if err != nil {
			return nil, err
		}

		// Добавляем ТОЛЬКО экспортируемые функции
		for _, fn := range subProg.Functions {
			if fn.IsExport {
				im.importedFuncs = append(im.importedFuncs, fn)
			}
		}
	}

	return mainProg, nil
}

// loadModuleInternal - внутренняя загрузка без мьютекса
func (im *ImportManager) loadModuleInternal(path string, visited map[string]bool) (*Program, error) {
	if visited[path] {
		return nil, fmt.Errorf("circular import detected: %s", path)
	}

	if loaded, ok := im.loaded[path]; ok {
		return loaded.Program, nil
	}

	// Резолвим путь
	fullPath, found, builtin := im.resolver.Resolve(path)
	if !found {
		return nil, fmt.Errorf("module not found: %s", path)
	}

	var content string

	if builtin {
		var ok bool
		content, ok = stdlib.GetModule(path)
		if !ok {
			return nil, fmt.Errorf("builtin module not found: %s", path)
		}
	} else {
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read module %s: %v", path, err)
		}
		content = string(data)
	}

	// Парсим с именем файла
	var parser *Parser
	if builtin {
		parser = NewParserWithFile(content, "<builtin:"+path+">")
	} else {
		parser = NewParserWithFile(content, fullPath)
	}
	prog := parser.Parse()

	if prog == nil {
		return nil, fmt.Errorf("failed to parse module: %s", path)
	}

	// Загружаем вложенные импорты
	visited[path] = true
	for _, imp := range prog.Imports {
		subProg, err := im.loadModuleInternal(imp.Path, visited)
		if err != nil {
			return nil, err
		}
		prog.Functions = append(prog.Functions, subProg.Functions...)
	}
	delete(visited, path)

	im.loaded[path] = &LoadedModule{
		Path:    fullPath,
		Program: prog,
		Imports: getImportPaths(prog),
	}

	return prog, nil
}

// LoadModule публичный метод с мьютексом для внешних вызовов
func (im *ImportManager) LoadModule(path string) (*Program, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	visited := make(map[string]bool)
	return im.loadModuleInternal(path, visited)
}

func (im *ImportManager) GetModule(path string) (*LoadedModule, bool) {
	im.mu.Lock()
	defer im.mu.Unlock()
	mod, ok := im.loaded[path]
	return mod, ok
}

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

func GetModuleName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func IsExportable(fn *Function) bool {
	return fn.IsExport
}

func (im *ImportManager) GetAllFunctionsInternal() []*Function {
	im.mu.Lock()
	defer im.mu.Unlock()

	var allFunctions []*Function
	for _, mod := range im.loaded {
		allFunctions = append(allFunctions, mod.Program.Functions...)
	}
	return allFunctions
}
