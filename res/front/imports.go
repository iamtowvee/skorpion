package front

import (
	"fmt"
	"os"
	"path/filepath"
	"skrp/res/debug"
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

	// Для библиотек
	libsDir      string
	dependencies []string
}

func NewImportManager(baseDir string) *ImportManager {
	return &ImportManager{
		resolver:      NewModuleResolver(baseDir),
		loaded:        make(map[string]*LoadedModule),
		importedFuncs: []*Function{},
		baseDir:       baseDir,
	}
}

// SetLibraryConfig прокидывает настройки библиотек из manifest.spc.
// libsDir — абсолютный или относительный путь; относительно projectDir.
// dependencies — список "author/name@1.0.0" (только для build-lib).
func (im *ImportManager) SetLibraryConfig(projectDir, libsDir string, dependencies []string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if libsDir == "" {
		libsDir = "libs/"
	}
	if !filepath.IsAbs(libsDir) {
		libsDir = filepath.Join(projectDir, libsDir)
	}
	im.libsDir = libsDir
	im.dependencies = dependencies
}

func appendUniqueGlobalVars(dst []*VarDecl, src *VarDecl) []*VarDecl {
	if src == nil {
		return dst
	}
	for _, v := range dst {
		if v.Name == src.Name {
			return dst
		}
	}
	return append(dst, src)
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

	// Парсим главный файл С ИМЕНЕМ ФАЙЛА
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

		// Переносим global includeC из модуля в main (с дедупликацией)
		mainProg.GlobalIncludeC = appendUniqueStrings(mainProg.GlobalIncludeC, subProg.GlobalIncludeC)

		// Переносим ErrorDecls из модуля в main (с дедупликацией)
		mainProg.ErrorDecls = appendUniqueErrors(mainProg.ErrorDecls, subProg.ErrorDecls)

		// NEW: переносим только экспортируемые глобальные константы
		for _, gv := range subProg.GlobalVars {
			if gv.IsExport {
				mainProg.GlobalVars = appendUniqueGlobalVars(mainProg.GlobalVars, gv)
			}
		}

		// Добавляем ТОЛЬКО экспортируемые функции
		for _, fn := range subProg.Functions {
			if fn.IsExport {
				im.importedFuncs = append(im.importedFuncs, fn)
			}
		}
	}

	// Собираем полный список функций (локальные + импортированные)
	mainProg.AllFunctions = make([]*Function, 0, len(mainProg.Functions)+len(im.importedFuncs))
	mainProg.AllFunctions = append(mainProg.AllFunctions, mainProg.Functions...)
	mainProg.AllFunctions = append(mainProg.AllFunctions, im.importedFuncs...)

	debug.Debug("LoadMain: Functions=%d ImportedFuncs=%d GlobalVars=%d AllFunctions=%d",
		len(mainProg.Functions), len(im.importedFuncs), len(mainProg.GlobalVars), len(mainProg.AllFunctions))
	for _, fn := range mainProg.AllFunctions {
		debug.Debug("  AllFunctions: %s -> %s", fn.Name, fn.ReturnType)
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

	// === Библиотека ===
	if strings.HasPrefix(path, "lib:") {
		return im.loadLibraryInternal(path, visited)
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

		// Переносим global includeC из вложенных модулей (с дедупликацией)
		prog.GlobalIncludeC = appendUniqueStrings(prog.GlobalIncludeC, subProg.GlobalIncludeC)

		// Переносим ErrorDecls из вложенных модулей (с дедупликацией)
		prog.ErrorDecls = appendUniqueErrors(prog.ErrorDecls, subProg.ErrorDecls)

		// NEW: переносим только экспортируемые глобальные константы
		for _, gv := range subProg.GlobalVars {
			if gv.IsExport {
				prog.GlobalVars = appendUniqueGlobalVars(prog.GlobalVars, gv)
			}
		}

		// Функции — как было
		prog.Functions = append(prog.Functions, subProg.Functions...)
	}
	delete(visited, path)

	// === Заполняем Module для СВОИХ ErrorDecl ===
	moduleName := GetModuleName(path)
	for _, d := range prog.ErrorDecls {
		if d.Module == "" {
			d.Module = moduleName
		}
	}

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

// GetAllFunctionsInternal — ВСЕ функции из всех загруженных модулей,
// включая приватные (IsExport == false). Нужно для backend/pipeline,
// который компилирует тела функций.
func (im *ImportManager) GetAllFunctionsInternal() []*Function {
	im.mu.Lock()
	defer im.mu.Unlock()

	var allFunctions []*Function
	for _, mod := range im.loaded {
		allFunctions = append(allFunctions, mod.Program.Functions...)
	}
	return allFunctions
}

// GetExportedFunctionsInternal — только экспортируемые функции
// из загруженных модулей. Нужно для semantic: приватные функции
// либы (IsExport == false) не должны быть видны потребителю.
func (im *ImportManager) GetExportedFunctionsInternal() []*Function {
	im.mu.Lock()
	defer im.mu.Unlock()

	var exported []*Function
	for _, mod := range im.loaded {
		for _, fn := range mod.Program.Functions {
			if fn.IsExport {
				exported = append(exported, fn)
			}
		}
	}
	return exported
}

func appendUniqueErrors(dst []*ErrorDecl, src []*ErrorDecl) []*ErrorDecl {
	seen := make(map[string]bool, len(dst))
	for _, d := range dst {
		seen[d.Name] = true
	}
	for _, d := range src {
		if !seen[d.Name] {
			dst = append(dst, d)
			seen[d.Name] = true
		}
	}
	return dst
}

func appendUniqueStrings(dst []string, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range src {
		if !seen[s] {
			dst = append(dst, s)
			seen[s] = true
		}
	}
	return dst
}

func (im *ImportManager) loadLibraryInternal(path string, visited map[string]bool) (*Program, error) {
	spec, err := ParseLibSpec(path)
	if err != nil {
		return nil, fmt.Errorf("invalid library path '%s': %w", path, err)
	}

	if im.libsDir == "" {
		return nil, fmt.Errorf(
			"libs dir is not configured; add libs[\"path/\"] to manifest.spc")
	}

	lib, file, err := FindLibrary(im.libsDir, spec)
	if err != nil {
		return nil, err
	}

	// Проверки (имя/автор/версия уже частично проверены в FindLibrary,
	// но Name и Author — нет, потому что папка навигационная).
	if lib.Name != spec.Name {
		return nil, fmt.Errorf(
			"library name mismatch: path says '%s', file says '%s'",
			spec.Name, lib.Name)
	}
	if lib.Author != "" && lib.Author != spec.Author {
		return nil, fmt.Errorf(
			"library author mismatch: path says '%s', file says '%s'",
			spec.Author, lib.Author)
	}
	// Version уже совпала в FindLibrary, если spec.Version != ""
	// Но если spec.Version == "" — версия любая, ок.

	if err := im.checkDependencies(lib, path); err != nil {
		return nil, err
	}

	im.loaded[path] = &LoadedModule{
		Path:    file,
		Program: lib.Program,
	}

	visited[path] = true
	for _, imp := range lib.Program.Imports {
		if imp.IsLib {
			_, err := im.loadModuleInternal(imp.Path, visited)
			if err != nil {
				return nil, fmt.Errorf(
					"library %s depends on %s: %w", path, imp.Path, err)
			}
		}
	}
	delete(visited, path)

	return lib.Program, nil
}

func (im *ImportManager) checkDependencies(lib *Library, libPath string) error {
	if len(lib.Dependencies) == 0 {
		return nil
	}

	declared := make(map[string]bool)
	for key := range im.loaded {
		if strings.HasPrefix(key, "lib:") {
			declared[key] = true
		}
	}

	for _, dep := range im.dependencies {
		if !strings.HasPrefix(dep, "lib:") {
			dep = "lib:" + dep
		}
		declared[dep] = true
	}

	declared[libPath] = true

	for _, dep := range lib.Dependencies {
		depPath := dep
		if !strings.HasPrefix(depPath, "lib:") {
			depPath = "lib:" + depPath
		}
		if !declared[depPath] {
			return fmt.Errorf(
				"library %s requires dependency '%s', but it is not imported; "+
					"add 'use lib:%s' before 'use lib:%s'",
				libPath, dep, dep, strings.TrimPrefix(libPath, "lib:"))
		}
	}

	return nil
}
