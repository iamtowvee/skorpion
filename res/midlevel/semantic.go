package midlevel

import (
	"fmt"
	"skrp/res/debug"
	"skrp/res/errors"
	"skrp/res/front"
	"strconv"
	"strings"
)

type SemanticAnalyzer struct {
	GlobalScope     *Scope
	CurrentScope    *Scope
	CurrentFunction *front.Function
	CurrentFile     string
	Program         *front.Program
	Errors          []errors.SkorpionError
	ImportedFuncs   map[string]*front.Function
	ImportManager   *front.ImportManager
	ErrorTypes      map[string]*front.ErrorDecl
	IsLibrary       bool
	LoopDepth       int
}

func NewSemanticAnalyzer(prog *front.Program) *SemanticAnalyzer {
	sa := &SemanticAnalyzer{
		Program:       prog,
		GlobalScope:   NewScope(nil, true),
		ImportedFuncs: make(map[string]*front.Function),
		ImportManager: nil,
		ErrorTypes:    make(map[string]*front.ErrorDecl),
	}
	return sa
}

func (sa *SemanticAnalyzer) SetImportManager(im *front.ImportManager) {
	sa.ImportManager = im
	sa.registerImportedFunctions()
}

// SetLibraryMode включает режим библиотеки: checkMain() не требует
// наличия void main(arr args).
func (sa *SemanticAnalyzer) SetLibraryMode(isLib bool) {
	sa.IsLibrary = isLib
}

func (sa *SemanticAnalyzer) registerImportedFunctions() {
	if sa.ImportManager == nil {
		return
	}

	allFunctions := sa.ImportManager.GetAllFunctions()
	for _, fn := range allFunctions {
		if !front.IsExportable(fn) {
			continue
		}

		// 1. Под простым именем (fallback для нормализованных вызовов)
		if _, exists := sa.ImportedFuncs[fn.Name]; !exists {
			sa.ImportedFuncs[fn.Name] = fn
		}

		// 2. Под полным ключом (alias/модуль/All)
		for _, imp := range sa.Program.Imports {
			// Для lib-импортов fn.Name — глобальное, регистрируем под любым ключом
			// (потому что либа не имеет «модульного» пространства имён кроме алиаса)
			var key string
			if imp.Alias != "" {
				key = imp.Alias + "." + fn.Name
			} else if imp.All {
				key = fn.Name
			} else {
				moduleName := front.GetModuleName(imp.Path)
				key = moduleName + "." + fn.Name
			}
			if _, exists := sa.ImportedFuncs[key]; !exists {
				sa.ImportedFuncs[key] = fn
			}
		}
	}
}

func (sa *SemanticAnalyzer) resolveFunction(name string) *front.Function {
	debug.Debug("resolveFunction: %s", name)

	// 1. Локальные
	for _, fn := range sa.Program.Functions {
		if fn.Name == name {
			debug.Debug("Found in current file: %s\n", name)
			return fn
		}
	}

	// 2. Импортированные по точному ключу ("mp.print")
	if fn, ok := sa.ImportedFuncs[name]; ok {
		debug.Debug("Found in imports (exact): %s\n", name)
		return fn
	}

	// 3. Нормализуем "mp.print" → "print"
	simpleName := name
	if idx := strings.LastIndex(simpleName, "."); idx >= 0 {
		simpleName = simpleName[idx+1:]
	}

	// 4. Локальные по простому имени
	for _, fn := range sa.Program.Functions {
		if fn.Name == simpleName {
			debug.Debug("Found in current file (simple): %s\n", simpleName)
			return fn
		}
	}

	// 5. Импортированные по простому имени
	if fn, ok := sa.ImportedFuncs[simpleName]; ok {
		debug.Debug("Found in imports (simple): %s\n", simpleName)
		return fn
	}

	// 6. Скан всех ImportedFuncs по fn.Name (на случай, если register
	//    не зарегистрировал под простым именем — например, старый код)
	for _, fn := range sa.ImportedFuncs {
		if fn.Name == simpleName {
			debug.Debug("Found in imports (scan by fn.Name): %s\n", simpleName)
			return fn
		}
	}

	// 7. Скан Program.AllFunctions
	for _, fn := range sa.Program.AllFunctions {
		if fn.Name == simpleName {
			debug.Debug("Found in AllFunctions: %s\n", simpleName)
			return fn
		}
	}

	debug.Debug("Function %s not found\n", name)
	return nil
}

func (sa *SemanticAnalyzer) Analyze() bool {
	debug.Debug("SemanticAnalyzer.Analyze() started\n")

	sa.initBuiltinTypes()
	debug.Debug("Built-in types initialized\n")

	// Регистрируем встроенный Error
	errorDecl := &front.ErrorDecl{
		Name: "Error",
		Fields: []*front.ErrorField{
			{Name: "msg", Type: "string"},
		},
		Parent: "",
		IsNew:  false,
	}
	sa.ErrorTypes["Error"] = errorDecl
	sa.GlobalScope.Define("Error", SYM_CONST, "Error", true)

	// Регистрируем пользовательские ошибки
	for _, decl := range sa.Program.ErrorDecls {
		sa.registerErrorDecl(decl)
	}

	// NEW: регистрируем глобальные константы
	sa.registerGlobalVars()

	debug.Debug("Registering %d functions from main\n", len(sa.Program.Functions))
	for _, fn := range sa.Program.Functions {
		sa.registerFunction(fn)
		debug.Debug("Registered function: %s\n", fn.Name)
	}

	if !sa.checkMain() {
		debug.Debug("checkMain() failed\n")
		return false
	}
	debug.Debug("checkMain() passed\n")

	debug.Debug("Analyzing %d functions\n", len(sa.Program.Functions))
	for _, fn := range sa.Program.Functions {
		if sa.isImportedFunction(fn.Name) {
			debug.Debug("Skipping imported function: %s\n", fn.Name)
			continue
		}

		debug.Debug("Analyzing function: %s\n", fn.Name)
		sa.CurrentFunction = fn
		sa.CurrentFile = fn.File
		sa.CurrentScope = NewScope(sa.GlobalScope, false)
		sa.analyzeFunction(fn)

		if len(sa.Errors) > 0 {
			debug.Debug("Errors found after analyzing %s: %d\n", fn.Name, len(sa.Errors))
			for _, err := range sa.Errors {
				debug.Debug("Error: %s: %s\n", err.Code, err.Message)
			}
			return false
		}
	}

	debug.Debug("All functions analyzed, checking for errors...\n")
	debug.Debug("Total errors: %d\n", len(sa.Errors))

	if len(sa.Errors) > 0 {
		debug.Debug("Errors found at the end: %d\n", len(sa.Errors))
		for _, err := range sa.Errors {
			debug.Debug("Final Error: %s: %s\n", err.Code, err.Message)
		}
	}

	return len(sa.Errors) == 0
}

func (sa *SemanticAnalyzer) registerGlobalVars() {
	for _, g := range sa.Program.GlobalVars {
		if g == nil {
			continue
		}

		if existing := sa.GlobalScope.ResolveLocal(g.Name); existing != nil {
			sa.addError("1500",
				fmt.Sprintf("Global constant '%s' already declared", g.Name),
				g.GetLine(), g.GetColumn(), "")
			continue
		}

		if g.Type == "void" {
			sa.addError("1558",
				fmt.Sprintf("Cannot declare global constant '%s' of type 'void'", g.Name),
				g.GetLine(), g.GetColumn(), "")
			continue
		}

		if !sa.isValidType(g.Type) && !isUnionType(g.Type) && !isArrayTypeSemantic(g.Type) {
			sa.addError("1506",
				fmt.Sprintf("Unknown type '%s' for global constant '%s'", g.Type, g.Name),
				g.GetLine(), g.GetColumn(), "")
			continue
		}

		if g.Expr == nil {
			sa.addError("1557",
				fmt.Sprintf("Global constant '%s' requires an initializer", g.Name),
				g.GetLine(), g.GetColumn(), "")
			continue
		}

		// Проверка инициализатора — литерал и его тип должен совпадать с declared.
		// Global-константы инициализируются только литералами (см. pipeline.processGlobalVar),
		// поэтому проверяем именно литеральный тип.
		exprType := sa.getGlobalInitType(g.Expr)
		if exprType == "" {
			sa.addError("1557",
				fmt.Sprintf("Global constant '%s' initializer must be a literal (number, string, char, bool)", g.Name),
				g.Expr.GetLine(), g.Expr.GetColumn(), sa.CurrentFile)
			continue
		}

		if exprType == "void" {
			sa.addError("1557",
				fmt.Sprintf("Cannot assign null to global constant '%s'", g.Name),
				g.Expr.GetLine(), g.Expr.GetColumn(), sa.CurrentFile)
			continue
		}

		// Разрешаем char↔int (как для локальных), any принимает всё.
		if g.Type == "any" {
			// ok
		} else if g.Type == "int" && exprType == "char" {
			// ok
		} else if g.Type == "char" && exprType == "int" {
			// ok
		} else if g.Type != exprType {
			sa.addError("1507",
				fmt.Sprintf("Type mismatch: cannot assign '%s' to global constant '%s' of type '%s'",
					exprType, g.Name, g.Type),
				g.Expr.GetLine(), g.Expr.GetColumn(), sa.CurrentFile)
			continue
		}

		sym := sa.GlobalScope.Define(g.Name, SYM_CONST, g.Type, g.IsExport)
		sym.IsConst = true
	}
}

// getGlobalInitType возвращает тип литерала (или "", если это не литерал).
func (sa *SemanticAnalyzer) getGlobalInitType(node front.Node) string {
	switch n := node.(type) {
	case *front.Number:
		if strings.HasSuffix(n.Value, "f") || strings.HasSuffix(n.Value, "F") {
			return "float"
		}
		if strings.Contains(n.Value, ".") {
			return "double"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.CharLiteral:
		return "char"
	case *front.UnicodeLiteral:
		return "char"
	case *front.NullLiteral:
		return "void"
	case *front.Ident:
		if n.Name == "true" || n.Name == "false" {
			return "bool"
		}
		return ""
	}
	return ""
}

func (sa *SemanticAnalyzer) isImportedFunction(name string) bool {
	debug.Debug("isImportedFunction: checking %s\n", name)
	for _, fn := range sa.ImportedFuncs {
		if fn.Name == name {
			debug.Debug("isImportedFunction: %s is imported\n", name)
			return true
		}
	}
	debug.Debug("isImportedFunction: %s is NOT imported\n", name)
	return false
}

func (sa *SemanticAnalyzer) initBuiltinTypes() {
	types := []string{"int", "string", "float", "double", "bool", "char", "arr", "dict", "void", "any"}
	for _, t := range types {
		sa.GlobalScope.Define("type_"+t, SYM_CONST, "type", true)
	}

	builtins := map[string]string{
		"to_int":    "int",
		"to_float":  "float",
		"to_double": "double",
		"to_string": "string",
		"to_bool":   "bool",
		"to_arr":    "arr[any]",
	}
	for name, retType := range builtins {
		sa.GlobalScope.Define(name, SYM_FUNCTION, retType, true)
	}
}

func (sa *SemanticAnalyzer) registerFunction(fn *front.Function) {
	if existing := sa.GlobalScope.Resolve(fn.Name); existing != nil {
		sa.addError("1500", fmt.Sprintf("Function '%s' already declared", fn.Name),
			fn.GetNameLine(), fn.GetNameColumn(), fn.File)
		return
	}
	if _, isError := sa.ErrorTypes[fn.Name]; isError {
		sa.addError("1500",
			fmt.Sprintf("Name '%s' already used as error type", fn.Name),
			fn.GetNameLine(), fn.GetNameColumn(), fn.File)
		return
	}
	sa.GlobalScope.Define(fn.Name, SYM_FUNCTION, fn.ReturnType, fn.IsExport)
}

func (sa *SemanticAnalyzer) checkMain() bool {
	// В режиме библиотеки main() не нужен — это просто набор функций.
	if sa.IsLibrary {
		return true
	}

	mainSym := sa.GlobalScope.Resolve("main")
	if mainSym == nil {
		sa.addError("1501", "No main() function found", 0, 0, "")
		return false
	}

	if mainSym.Type != "void" {
		sa.addError("1502", "main() must return void", 0, 0, "")
		return false
	}

	for _, fn := range sa.Program.Functions {
		if fn.Name == "main" {
			if len(fn.Params) != 1 {
				sa.addError("1503", "main() must take exactly one parameter (arr args)",
					fn.GetNameLine(), fn.GetNameColumn(), fn.File)
				return false
			}
			if !isArrayTypeSemantic(fn.Params[0].Type) {
				sa.addError("1504", "main() parameter must be of type 'arr'",
					fn.GetNameLine(), fn.GetNameColumn(), fn.File)
				return false
			}
			break
		}
	}

	return true
}

func (sa *SemanticAnalyzer) checkUselessAsync(fn *front.Function) {
	if !fn.IsAsync || fn.Body == nil {
		return
	}
	if sa.hasDynamicConstructs(fn.Body) {
		return
	}
	errors.NewWarning("2005",
		fmt.Sprintf("'async' is useless in '%s()' — function contains only static operations; "+
			"Promise will only hold the final return value", fn.Name),
		fn.GetNameLine(), fn.GetNameColumn(), fn.File)
}

func (sa *SemanticAnalyzer) hasDynamicConstructs(node front.Node) bool {
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *front.PromiseSet:
		return true
	case *front.AwaitExpr:
		return true
	case *front.ThrowStmt:
		return true
	case *front.TryStmt:
		return true
	case *front.IncludeC:
		return true
	case *front.CallExpr:
		simpleName := n.Name
		if strings.Contains(simpleName, ".") {
			parts := strings.Split(simpleName, ".")
			simpleName = parts[len(parts)-1]
		}
		if target := sa.resolveFunction(simpleName); target != nil {
			if target.IsAsync {
				return true
			}
			if target.Body != nil && sa.containsIncludeC(target.Body) {
				return true
			}
		}
		for _, arg := range n.Args {
			if sa.hasDynamicConstructs(arg) {
				return true
			}
		}
		return false
	case *front.Block:
		for _, s := range n.Statements {
			if sa.hasDynamicConstructs(s) {
				return true
			}
		}
	case *front.IfStmt:
		if sa.hasDynamicConstructs(n.Condition) {
			return true
		}
		if sa.hasDynamicConstructs(n.Then) {
			return true
		}
		for _, e := range n.Elsifs {
			if sa.hasDynamicConstructs(e.Condition) || sa.hasDynamicConstructs(e.Then) {
				return true
			}
		}
		if sa.hasDynamicConstructs(n.Else) {
			return true
		}
	case *front.WhileStmt:
		if sa.hasDynamicConstructs(n.Condition) || sa.hasDynamicConstructs(n.Body) {
			return true
		}
	case *front.DoWhileStmt:
		if sa.hasDynamicConstructs(n.Body) || sa.hasDynamicConstructs(n.Condition) {
			return true
		}
	case *front.ForInStmt:
		if sa.hasDynamicConstructs(n.Iterable) || sa.hasDynamicConstructs(n.Body) {
			return true
		}
	case *front.CaseStmt:
		if sa.hasDynamicConstructs(n.Value) {
			return true
		}
		for _, b := range n.Branches {
			if sa.hasDynamicConstructs(b.Body) {
				return true
			}
		}
		if sa.hasDynamicConstructs(n.Default) {
			return true
		}
	case *front.BinaryExpr:
		return sa.hasDynamicConstructs(n.Left) || sa.hasDynamicConstructs(n.Right)
	case *front.UnaryExpr:
		return sa.hasDynamicConstructs(n.Expr)
	case *front.TernaryExpr:
		return sa.hasDynamicConstructs(n.Condition) ||
			sa.hasDynamicConstructs(n.Then) ||
			sa.hasDynamicConstructs(n.Else)
	case *front.VarDecl:
		return sa.hasDynamicConstructs(n.Expr)
	case *front.Assign:
		return sa.hasDynamicConstructs(n.Expr)
	case *front.ReturnStmt:
		return sa.hasDynamicConstructs(n.Expr)
	case *front.ArrayLiteral:
		for _, e := range n.Elements {
			if sa.hasDynamicConstructs(e) {
				return true
			}
		}
	case *front.ArrayIndex:
		return sa.hasDynamicConstructs(n.Index)
	case *front.ArrayAdd:
		return sa.hasDynamicConstructs(n.Elem)
	case *front.FormatExpr:
		return sa.hasDynamicConstructs(n.Expr)
	case *front.TypeOf:
		return sa.hasDynamicConstructs(n.Expr)
	case *front.RangeExpr:
		return sa.hasDynamicConstructs(n.Start) || sa.hasDynamicConstructs(n.End)
	}
	return false
}

func (sa *SemanticAnalyzer) analyzeFunction(fn *front.Function) {
	sa.LoopDepth = 0

	debug.Debug("analyzeFunction: %s\n", fn.Name)

	sa.CurrentScope = NewScope(sa.GlobalScope, false)
	defer func() {
		debug.Debug("analyzeFunction: exiting %s\n", fn.Name)
		sa.CurrentScope = sa.CurrentScope.Parent
	}()

	// Проверка возвращаемого типа на union
	if isUnionType(fn.ReturnType) {
		for _, t := range parseUnionTypes(fn.ReturnType) {
			if t == "any" {
				sa.addError("0615",
					fmt.Sprintf("Cannot use 'any' in return union of function '%s'", fn.Name),
					fn.GetLine(), fn.GetColumn(), sa.CurrentFile)
			}
			if isUnionType(t) {
				sa.addError("0611",
					"Cannot nest T<...> inside T<...>",
					fn.GetLine(), fn.GetColumn(), sa.CurrentFile)
			}
		}
	}

	// Проверка дублирующихся параметров и default перед non-default
	seenParams := make(map[string]bool)
	seenDefault := false
	for _, param := range fn.Params {
		if seenParams[param.Name] {
			sa.addError("1536",
				fmt.Sprintf("Duplicate parameter '%s' in function '%s'", param.Name, fn.Name),
				param.GetLine(), param.GetColumn(), sa.CurrentFile)
		}
		seenParams[param.Name] = true

		if param.HasDefault() {
			seenDefault = true
		} else if seenDefault {
			sa.addError("1537",
				fmt.Sprintf("Parameter '%s' has no default value but comes after parameters with defaults in '%s'",
					param.Name, fn.Name),
				param.GetLine(), param.GetColumn(), sa.CurrentFile)
		}
	}

	// Регистрируем параметры
	for _, param := range fn.Params {
		debug.Debug("Adding parameter: %s %s\n", param.Name, param.Type)

		if isUnionType(param.Type) {
			for _, t := range parseUnionTypes(param.Type) {
				if t == "any" {
					sa.addError("0615",
						fmt.Sprintf("Cannot use 'any' in union type of parameter '%s'", param.Name),
						param.GetLine(), param.GetColumn(), sa.CurrentFile)
				}
				if isUnionType(t) {
					sa.addError("0611",
						"Cannot nest T<...> inside T<...>",
						param.GetLine(), param.GetColumn(), sa.CurrentFile)
				}
			}
		}

		sa.CurrentScope.Define(param.Name, SYM_VARIABLE, param.Type, false)
	}

	// Проверка: ... только один и только последний
	variadicCount := 0
	for i, param := range fn.Params {
		if param.IsVariadic {
			variadicCount++
			if i != len(fn.Params)-1 {
				sa.addError("0560",
					"Variadic parameter must be the last one",
					param.GetLine(), param.GetColumn(), sa.CurrentFile)
			}
		}
	}
	if variadicCount > 1 {
		sa.addError("0560",
			"Only one variadic parameter is allowed",
			fn.GetLine(), fn.GetColumn(), sa.CurrentFile)
	}

	// === ASYNC checks ===
	if fn.IsAsync {
		if isUnionType(fn.ReturnType) {
			for _, t := range parseUnionTypes(fn.ReturnType) {
				if t == "any" {
					sa.addError("0615",
						"Cannot use 'any' in async return union",
						fn.GetLine(), fn.GetColumn(), sa.CurrentFile)
				}
			}
		}

		// Promise[...] — только для async T, где T != void.
		// Для async void Promise[...] запрещён: результат некуда девать.
		// Правило «ровно один» применяется только к async T (T != void).
		if fn.Body != nil {
			promiseCount := 0
			for i, stmt := range fn.Body.Statements {
				if ps, isPromise := stmt.(*front.PromiseSet); isPromise {
					promiseCount++
					if fn.ReturnType == "void" {
						sa.addError("0676",
							fmt.Sprintf("'Promise[...]' cannot be used in async void function '%s' — the result is discarded", fn.Name),
							ps.GetLine(), ps.GetColumn(), sa.CurrentFile)
						continue
					}
					if i != 0 {
						sa.addError("0670",
							fmt.Sprintf("'Promise[...]' must be the first statement in async function '%s'", fn.Name),
							ps.GetLine(), ps.GetColumn(), sa.CurrentFile)
					}
					if !isLiteralNode(ps.Value) {
						sa.addError("0671",
							"Promise[...] requires a literal (number, string, char, true, false, null)",
							ps.Value.GetLine(), ps.Value.GetColumn(), sa.CurrentFile)
					}
				}
			}
			if promiseCount > 1 && fn.ReturnType != "void" {
				sa.addError("0675",
					fmt.Sprintf("async function '%s' must have exactly one 'Promise[...]'", fn.Name),
					fn.GetNameLine(), fn.GetNameColumn(), sa.CurrentFile)
			}
		}
	} else {
		if fn.Body != nil {
			for _, stmt := range fn.Body.Statements {
				if _, isPromise := stmt.(*front.PromiseSet); isPromise {
					sa.addError("0662",
						"'Promise[...]' can only be used inside an async function",
						stmt.GetLine(), stmt.GetColumn(), sa.CurrentFile)
				}
			}
		}
	}

	if fn.Body != nil {
		debug.Debug("Analyzing body of %s\n", fn.Name)
		sa.analyzeBlock(fn.Body, true)
		debug.Debug("Body of %s analyzed\n", fn.Name)

		// Проверка: non-void функция должна иметь return.
		// Для async void — не требуется (это Task без значения).
		if fn.ReturnType != "void" && !sa.hasReturn(fn.Body) {
			sa.addError("1535",
				fmt.Sprintf("Function '%s' must return a value of type '%s'", fn.Name, fn.ReturnType),
				fn.GetNameLine(), fn.GetNameColumn(), sa.CurrentFile)
		}

		if len(fn.Body.Statements) == 0 {
			errors.NewWarning("2003",
				fmt.Sprintf("Empty function body in '%s'", fn.Name),
				fn.GetNameLine(), fn.GetNameColumn(), sa.CurrentFile)
		}

		used := make(map[string]bool)
		sa.collectUsedIdents(fn.Body, used)

		hasIncludeC := sa.containsIncludeC(fn.Body)

		unusedParams := []*front.Param{}
		if fn.Name != "main" {
			for _, param := range fn.Params {
				if !used[param.Name] {
					unusedParams = append(unusedParams, param)
				}
			}
		}

		if len(unusedParams) > 0 {
			if hasIncludeC {
				names := []string{}
				for _, p := range unusedParams {
					names = append(names, "'"+p.Name+"'")
				}
				errors.NewWarningSpan("2004",
					fmt.Sprintf("Could not check if parameters %s in '%s' are used — function contains includeC (raw C code)",
						strings.Join(names, ", "), fn.Name),
					fn.GetNameLine(), fn.GetNameColumn(),
					fn.GetNameLine(), fn.GetNameColumn()+len(fn.Name),
					sa.CurrentFile)
			} else {
				for _, param := range unusedParams {
					errors.NewWarningSpan("2001",
						fmt.Sprintf("Unused parameter '%s' in function '%s'", param.Name, fn.Name),
						param.GetLine(), param.GetColumn(),
						param.GetLine(), param.GetColumn()+len(param.Name),
						sa.CurrentFile)
				}
			}
		}

		if fn.IsAsync {
			sa.checkUselessAsync(fn)
		}

		sa.checkUnusedVariables(fn.Body, used)
	} else {
		debug.Debug("Function %s has no body\n", fn.Name)
	}

	debug.Debug("analyzeFunction %s completed\n", fn.Name)
}

func (sa *SemanticAnalyzer) analyzeBlock(block *front.Block, isFunctionBody bool) {
	debug.Debug("analyzeBlock: %d statements, isFunctionBody=%v\n", len(block.Statements), isFunctionBody)
	for i, stmt := range block.Statements {
		debug.Debug("Statement %d: %T\n", i, stmt)
		sa.analyzeStatement(stmt)
		debug.Debug("Statement %d processed\n", i)
	}
	debug.Debug("analyzeBlock completed\n")
}

// analyzeStatement — как analyzeNode, но для statement-level CallExpr
// с async-функцией делает fire-and-forget (разрешено).
func (sa *SemanticAnalyzer) analyzeStatement(stmt front.Node) {
	if call, ok := stmt.(*front.CallExpr); ok {
		simpleName := call.Name
		if strings.Contains(simpleName, ".") {
			parts := strings.Split(simpleName, ".")
			simpleName = parts[len(parts)-1]
		}
		if target := sa.resolveFunction(simpleName); target != nil && target.IsAsync {
			// fire-and-forget: результат discard, ошибки — в глобальный обработчик
			sa.analyzeFireAndForget(call, target)
			return
		}
	}
	sa.analyzeNode(stmt)
}

// analyzeFireAndForget — анализ async-вызова в statement-позиции.
// Проверяет аргументы, но не ругается на «async как значение».
func (sa *SemanticAnalyzer) analyzeFireAndForget(call *front.CallExpr, target *front.Function) {
	// Проверка количества аргументов
	requiredParams := 0
	optionalParams := 0
	hasVariadic := false
	for _, p := range target.Params {
		if p.IsVariadic {
			hasVariadic = true
			break
		}
		if p.DefaultValue != nil {
			optionalParams++
		} else {
			requiredParams++
		}
	}
	totalNormal := requiredParams + optionalParams

	if hasVariadic {
		if len(call.Args) < requiredParams {
			sa.addError("1519", fmt.Sprintf(
				"Function '%s' expects at least %d arguments, got %d",
				call.Name, requiredParams, len(call.Args)),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return
		}
	} else {
		if len(call.Args) < requiredParams {
			sa.addError("1519", fmt.Sprintf(
				"Function '%s' expects at least %d arguments, got %d",
				call.Name, requiredParams, len(call.Args)),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return
		}
		if len(call.Args) > totalNormal {
			sa.addError("1519", fmt.Sprintf(
				"Function '%s' expects at most %d arguments, got %d",
				call.Name, totalNormal, len(call.Args)),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return
		}
	}

	// Проверка типов аргументов
	for i, arg := range call.Args {
		sa.analyzeNode(arg)
		if hasVariadic && i >= totalNormal {
			continue
		}
		argType := sa.getNodeType(arg)
		paramType := target.Params[i].Type
		if argType != paramType && argType != "" && paramType != "any" {
			sa.addError("1520",
				fmt.Sprintf("Argument %d type mismatch: expected '%s', got '%s'",
					i+1, paramType, argType),
				arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
		}
	}
}

func (sa *SemanticAnalyzer) analyzePostfix(pe *front.PostfixExpr) front.Node {
	sym := sa.CurrentScope.Resolve(pe.VarName)
	if sym == nil {
		sa.addError("1508",
			fmt.Sprintf("Undefined variable '%s'", pe.VarName),
			pe.GetLine(), pe.GetColumn(), sa.CurrentFile)
		return pe
	}
	if sym.Type != "int" {
		sa.addError("1564",
			fmt.Sprintf("'%s' requires an int variable, got '%s'", pe.Op, sym.Type),
			pe.GetLine(), pe.GetColumn(), sa.CurrentFile)
	}
	if sym.IsConst {
		sa.addError("1509",
			fmt.Sprintf("Cannot modify constant '%s'", pe.VarName),
			pe.GetLine(), pe.GetColumn(), sa.CurrentFile)
	}
	return pe
}

func (sa *SemanticAnalyzer) analyzeNode(node front.Node) front.Node {
	debug.Debug("analyzeNode: %T\n", node)
	switch n := node.(type) {
	case *front.VarDecl:
		return sa.analyzeVarDecl(n)
	case *front.NullLiteral:
		return n
	case *front.PostfixExpr:
		return sa.analyzePostfix(n)
	case *front.UnicodeLiteral:
		return n
	case *front.PromiseSet:
		return sa.analyzePromiseSet(n)
	case *front.AwaitExpr:
		return sa.analyzeAwait(n)
	case *front.BreakStmt:
		return sa.analyzeBreak(n)
	case *front.ContinueStmt:
		return sa.analyzeContinue(n)
	case *front.DoWhileStmt:
		return sa.analyzeDoWhile(n)
	case *front.ArrayIndex:
		return sa.analyzeArrayIndex(n)
	case *front.IknowIdoBlock:
		// Полностью пропускаем анализ — ни типов, ни объявлений, ни проверок.
		return n
	case *front.Assign:
		return sa.analyzeAssign(n)
	case *front.CharLiteral:
		return sa.analyzeChar(n)
	case *front.BinaryExpr:
		return sa.analyzeBinary(n)
	case *front.ArrayLiteral:
		for _, elem := range n.Elements {
			sa.analyzeNode(elem)
		}
		return n
	case *front.ThrowStmt:
		return sa.analyzeThrow(n)
	case *front.TryStmt:
		return sa.analyzeTry(n)
	case *front.TernaryExpr:
		return sa.analyzeTernary(n)
	case *front.UnaryExpr:
		return sa.analyzeUnary(n)
	case *front.Number:
		return sa.analyzeNumber(n)
	case *front.String:
		return sa.analyzeString(n)
	case *front.Ident:
		return sa.analyzeIdent(n)
	case *front.ReturnStmt:
		return sa.analyzeReturn(n)
	case *front.CallExpr:
		return sa.analyzeCall(n)
	case *front.CallRangeExpr:
		return sa.analyzeCallRange(n)
	case *front.TypeOf:
		return sa.analyzeTypeOf(n)
	case *front.Block:
		sa.analyzeBlock(n, false)
		return n
	case *front.IfStmt:
		return sa.analyzeIf(n)
	case *front.CaseStmt:
		return sa.analyzeCase(n)
	case *front.WhileStmt:
		return sa.analyzeWhile(n)
	case *front.ForInStmt:
		return sa.analyzeForIn(n)
	case *front.FieldAccess:
		return sa.analyzeFieldAccess(n)
	case *front.ErrorInstance:
		return sa.analyzeErrorInstance(n)
	case *front.RangeExpr:
		sa.analyzeNode(n.Start)
		sa.analyzeNode(n.End)
		return n
	case *front.FormatExpr:
		return sa.analyzeFormatExpr(n)
	case *front.IncludeC:
		debug.Debug("IncludeC node found\n")
		return n
	default:
		debug.Debug("Unknown node type: %T\n", n)
		return n
	}
}

func (sa *SemanticAnalyzer) analyzeFieldAccess(fa *front.FieldAccess) front.Node {
	// 1. Объект должен существовать
	sym := sa.CurrentScope.Resolve(fa.Object)
	if sym == nil {
		sa.addError("1514",
			fmt.Sprintf("Undefined identifier '%s'", fa.Object),
			fa.GetLine(), fa.GetColumn(), sa.CurrentFile)
		return fa
	}

	// 2. dict — доступ к ключу через точку
	if sym.Type == "dict" {
		return fa
	}

	// 3. Объект должен быть error-типом
	decl, ok := sa.ErrorTypes[sym.Type]
	if !ok {
		sa.addError("1539",
			fmt.Sprintf("'%s' is not an error type (got '%s')", fa.Object, sym.Type),
			fa.GetLine(), fa.GetColumn(), sa.CurrentFile)
		return fa
	}

	// 4. Поле должно существовать
	allFields := sa.collectErrorFields(decl)
	if _, exists := allFields[fa.Field]; !exists {
		sa.addError("1539",
			fmt.Sprintf("Error type '%s' has no field '%s'", sym.Type, fa.Field),
			fa.GetLine(), fa.GetColumn(), sa.CurrentFile)
		return fa
	}

	return fa
}

func (sa *SemanticAnalyzer) analyzeTry(try *front.TryStmt) front.Node {
	// Тело try
	sa.analyzeBlock(try.Body, false)

	// Catch-блоки
	for _, clause := range try.Catches {
		// Проверка типа
		if clause.TypeName != "" && clause.TypeName != "Error" {
			// Разрезаем возможный "strings.Foo" или "strings.Parent.Child"
			simpleName := clause.TypeName
			if idx := strings.LastIndex(simpleName, "."); idx >= 0 {
				simpleName = simpleName[idx+1:]
			}
			if _, ok := sa.ErrorTypes[simpleName]; !ok {
				sa.addError("1560",
					fmt.Sprintf("Unknown error type '%s' in catch", clause.TypeName),
					clause.GetLine(), clause.GetColumn(), sa.CurrentFile)
			}
		}

		// Если есть VarName — определяем переменную в scope catch
		if clause.VarName != "" {
			typeName := clause.TypeName
			if typeName == "" {
				typeName = "Error"
			}
			sa.CurrentScope.Define(clause.VarName, SYM_VARIABLE, typeName, false)
		}

		if clause.Body != nil {
			sa.analyzeBlock(clause.Body, false)
		}
	}

	sa.resetUnionCurrentTypes()
	return try
}

func (sa *SemanticAnalyzer) analyzeUnary(unary *front.UnaryExpr) front.Node {
	if unary.Op == "$" {
		sa.analyzeNode(unary.Expr)
		return unary
	}
	if unary.Op == "!" {
		operandType := sa.getNodeType(unary.Expr)
		if operandType == "void" {
			// !null — это ошибка (null не bool)
			sa.addError("1555",
				"Operator '!' cannot be applied to null",
				unary.GetLine(), unary.GetColumn(), sa.CurrentFile)
		} else if operandType != "bool" && operandType != "" {
			sa.addError("1526",
				fmt.Sprintf("Operator '!' requires bool, got '%s'", operandType),
				unary.GetLine(), unary.GetColumn(), sa.CurrentFile)
		}
		sa.analyzeNode(unary.Expr)
		return unary
	}
	if unary.Op == "-" {
		operandType := sa.getNodeType(unary.Expr)
		if !sa.isNumericType(operandType) && operandType != "" {
			sa.addError("1527",
				fmt.Sprintf("Unary '-' requires numeric, got '%s'", operandType),
				unary.GetLine(), unary.GetColumn(), sa.CurrentFile)
		}
		sa.analyzeNode(unary.Expr)
		return unary
	}
	return unary
}

func (sa *SemanticAnalyzer) analyzeVarDecl(decl *front.VarDecl) front.Node {
	// === ASYNC: T x = greet(...) ===
	if call, ok := decl.Expr.(*front.CallExpr); ok {
		simpleName := call.Name
		if strings.Contains(simpleName, ".") {
			parts := strings.Split(simpleName, ".")
			simpleName = parts[len(parts)-1]
		}
		if target := sa.resolveFunction(simpleName); target != nil && target.IsAsync {
			// async void нельзя присвоить переменной
			if target.ReturnType == "void" {
				sa.addError("0678",
					fmt.Sprintf("Cannot assign result of async void function '%s' to variable '%s' — it has no value",
						simpleName, decl.Name),
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			// Тип-чек: decl.Type должен совпадать с target.ReturnType
			if decl.Type != target.ReturnType {
				sa.addError("1507",
					fmt.Sprintf("Type mismatch: cannot assign async '%s' (%s) to '%s'",
						simpleName, target.ReturnType, decl.Type),
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			// Проверка аргументов — как у обычного вызова
			for i, arg := range call.Args {
				sa.analyzeNode(arg)
				if i < len(target.Params) {
					argType := sa.getNodeType(arg)
					paramType := target.Params[i].Type
					if argType != paramType && argType != "" && paramType != "any" {
						sa.addError("1520",
							fmt.Sprintf("Argument %d type mismatch: expected '%s', got '%s'",
								i+1, paramType, argType),
							arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
					}
				}
			}
			sym := sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
			sym.RealType = "promise"
			return decl
		}
	}

	// === Union T<...> ===
	if isUnionType(decl.Type) {
		types := parseUnionTypes(decl.Type)

		for _, t := range types {
			if isUnionType(t) {
				sa.addError("0611",
					"Cannot nest T<...> inside T<...>",
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			if t == "any" {
				sa.addError("0615",
					"Cannot use 'any' inside T<...> — union must list concrete types",
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
		}

		if decl.Expr != nil {
			exprType := sa.getNodeType(decl.Expr)
			if exprType == "void" {
				sa.addError("1557",
					"Cannot assign null to union — cannot predict future value type",
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			if !isTypeInUnion(decl.Type, exprType) {
				sa.addError("0612",
					fmt.Sprintf("Type '%s' is not in union '%s'", exprType, decl.Type),
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			decl.CurrentType = exprType
		}

		sym := sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
		sym.IsConst = decl.IsConst
		return decl
	}

	// === dict ===
	if decl.Type == "dict" {
		sym := sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
		sym.IsConst = decl.IsConst
		if decl.Expr != nil {
			if lit, ok := decl.Expr.(*front.DictLiteral); ok {
				sym.DictKeyTypes = make(map[string]string)
				for _, elem := range lit.Elements {
					keyType := sa.getNodeType(elem.Value)
					if keyType != "" {
						sym.DictKeyTypes[elem.Key] = keyType
					}
					sa.analyzeNode(elem.Value)
				}
			}
		}
		return decl
	}

	// === Массив arr[...] ===
	if decl.IsArray {
		if decl.ElemType == "void" {
			sa.addError("1543",
				"Cannot declare array of type 'void' — all elements would be null",
				decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
			return decl
		}

		// Собираем типы элементов
		elemTypes := []string{}

		if decl.Expr != nil {
			if arrLit, ok := decl.Expr.(*front.ArrayLiteral); ok {
				expectedElemType := decl.ElemType

				if expectedElemType == "" || expectedElemType == "any" {
					for _, elem := range arrLit.Elements {
						if _, isNull := elem.(*front.NullLiteral); isNull {
							sa.addError("1542",
								"Cannot use null in heterogeneous array — cannot infer its type",
								elem.GetLine(), elem.GetColumn(), sa.CurrentFile)
							elemTypes = append(elemTypes, "void")
							continue
						}
						elemTypes = append(elemTypes, sa.getNodeType(elem))
					}
				} else {
					for _, elem := range arrLit.Elements {
						if isArrayTypeSemantic(expectedElemType) {
							if _, ok := elem.(*front.ArrayLiteral); !ok {
								if _, isNull := elem.(*front.NullLiteral); isNull {
									sa.addError("1542",
										fmt.Sprintf("Cannot use null in array of type '%s'", expectedElemType),
										elem.GetLine(), elem.GetColumn(), sa.CurrentFile)
									elemTypes = append(elemTypes, "void")
									continue
								}
								sa.addError("1534",
									fmt.Sprintf("Array element type mismatch: expected '%s', got '%s'",
										expectedElemType, sa.getNodeType(elem)),
									elem.GetLine(), elem.GetColumn(), sa.CurrentFile)
								elemTypes = append(elemTypes, sa.getNodeType(elem))
							} else {
								elemTypes = append(elemTypes, expectedElemType)
							}
						} else {
							elemType := sa.getNodeType(elem)
							if elemType == "void" {
								elemTypes = append(elemTypes, "void")
								continue
							}
							if elemType != expectedElemType && elemType != "" {
								sa.addError("1534",
									fmt.Sprintf("Array element type mismatch: expected '%s', got '%s'",
										expectedElemType, elemType),
									elem.GetLine(), elem.GetColumn(), sa.CurrentFile)
							}
							elemTypes = append(elemTypes, elemType)
						}
					}
				}

				for _, elem := range arrLit.Elements {
					sa.analyzeNode(elem)
				}
			}
		}

		elemType := decl.ElemType
		if elemType == "" {
			elemType = "any"
		}
		// fullType := "arr[" + elemType + "]"
		sym := sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
		sym.IsConst = decl.IsConst
		return decl
	}

	// === Скаляр ===
	if existing := sa.CurrentScope.ResolveLocal(decl.Name); existing != nil {
		sa.addError("1505", fmt.Sprintf("Variable '%s' already declared in this scope", decl.Name),
			decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
		return decl
	}

	if !sa.isValidType(decl.Type) {
		sa.addError("1506", fmt.Sprintf("Unknown type '%s'", decl.Type),
			decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
		return decl
	}

	var exprType string
	if decl.Expr != nil {
		exprType = sa.getNodeType(decl.Expr)

		if exprType == "void" {
			if decl.Type == "void" {
				sa.addError("1558",
					"Cannot declare variable of type 'void'",
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			if decl.Type == "any" {
				sa.addError("1557",
					"Cannot assign null to 'any' — cannot predict future value type",
					decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
				return decl
			}
			sym := sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
			sym.IsConst = decl.IsConst
			return decl
		}

		if decl.Type == "any" {
			// any принимает любой не-void тип
		} else if decl.Type == "int" && exprType == "char" {
			// OK: неявная конверсия char → int
		} else if decl.Type == "char" && exprType == "int" {
			// OK: неявная конверсия int → char
		} else if decl.Type != exprType && exprType != "" {
			sa.addError("1507", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s'", exprType, decl.Type),
				decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
			return decl
		}
	}

	sym := sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
	sym.IsConst = decl.IsConst
	return decl
}

func (sa *SemanticAnalyzer) isPromiseVar(name string) bool {
	sym := sa.CurrentScope.Resolve(name)
	if sym == nil {
		return false
	}
	// Проверяем: инициализирована из async-вызова?
	// Простой признак: RealType установлен в "promise" (нужно ставить при analyzeVarDecl).
	return sym.RealType == "promise"
}

func (sa *SemanticAnalyzer) analyzeAssign(assign *front.Assign) front.Node {
	sym := sa.CurrentScope.Resolve(assign.Name)
	debug.Debug("analyzeAssign: %s\n", assign.Name)
	if sym != nil && sym.RealType == "promise" {
		sa.addError("0674",
			fmt.Sprintf("Cannot assign to promise variable '%s' — Promise is written only by the async function", assign.Name),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return assign
	}

	if sym == nil {
		sa.addError("1508", fmt.Sprintf("Undefined variable '%s'", assign.Name),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return assign
	}

	// === x[i] = ... ===
	if assign.Index != nil {
		if sym.IsConst {
			sa.addError("1509",
				fmt.Sprintf("Cannot modify constant '%s'", assign.Name),
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			return assign
		}
		if !isArrayTypeSemantic(sym.Type) && sym.Type != "dict" {
			sa.addError("1539",
				fmt.Sprintf("Cannot index non-array variable '%s' (type '%s')",
					assign.Name, sym.Type),
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			return assign
		}

		indexType := sa.getNodeType(assign.Index)
		if indexType != "int" && indexType != "string" && indexType != "" {
			sa.addError("1520",
				fmt.Sprintf("Array index must be int, got '%s'", indexType),
				assign.Index.GetLine(), assign.Index.GetColumn(), sa.CurrentFile)
		}

		// Compound op: +, -, *, /, %, **, или "" (обычное =)
		if assign.Op != "" && assign.Op != "=" {
			sa.checkCompoundAssignOp(assign, sym.Type, assign.Expr)
		} else {
			valueType := sa.getNodeType(assign.Expr)
			elemType := "any"
			if sym.Type != "dict" {
				elemType = parseArrayElemTypeSemantic(sym.Type)
				if elemType == "" {
					elemType = "any"
				}
			}
			if valueType == "void" {
				// null — допустимо для any, int[], string[] и т.д.
			} else if elemType != "any" && valueType != elemType && valueType != "" {
				sa.addError("1510",
					fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s' element",
						valueType, elemType),
					assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			}
		}

		sa.analyzeNode(assign.Index)
		sa.analyzeNode(assign.Expr)
		return assign
	}

	// === x.field = ... ===
	if assign.Field != "" {
		if sym.IsConst {
			sa.addError("1509",
				fmt.Sprintf("Cannot modify constant '%s'", assign.Name),
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			return assign
		}
		// ... существующая логика для field_set
		sa.analyzeNode(assign.Expr)
		return assign
	}

	// === x = ... (простое или compound) ===
	if sym.IsConst {
		debug.Debug("Variable %s is const\n", assign.Name)
		sa.addError("1509",
			fmt.Sprintf("Cannot assign to constant '%s'", assign.Name),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return assign
	}

	if assign.Expr == nil {
		return assign
	}

	// Compound assignment
	if assign.Op != "" && assign.Op != "=" {
		sa.checkCompoundAssignOp(assign, sym.Type, assign.Expr)
		sa.analyzeNode(assign.Expr)
		return assign
	}

	// Обычное присваивание
	exprType := sa.getNodeType(assign.Expr)

	if isUnionType(sym.Type) {
		if exprType == "void" {
			sym.CurrentType = ""
			return assign
		}
		if !isTypeInUnion(sym.Type, exprType) {
			sa.addError("0612",
				fmt.Sprintf("Type '%s' is not in union '%s'", exprType, sym.Type),
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			return assign
		}
		sym.CurrentType = exprType
		return assign
	}

	if exprType == "void" {
		if sym.Type == "any" {
			sa.addError("1557",
				"Cannot assign null to 'any' — cannot predict future value type",
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			return assign
		}
		return assign
	}

	if sym.Type == "any" {
		// ок
	} else if sym.Type == "int" && exprType == "char" {
		// ок
	} else if sym.Type == "char" && exprType == "int" {
		// ок
	} else if sym.Type != exprType && exprType != "" {
		sa.addError("1510", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s' (variable '%s')",
			exprType, sym.Type, assign.Name),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return assign
	}

	return assign
}

// checkCompoundAssignOp проверяет, что operator (из assign.Op) допустим
// для типа symType и типа правой части.
func (sa *SemanticAnalyzer) checkCompoundAssignOp(assign *front.Assign, symType string, expr front.Node) {
	exprType := sa.getNodeType(expr)
	op := strings.TrimSuffix(assign.Op, "=")

	// any — всё разрешено
	if symType == "any" {
		return
	}

	// string += string|char
	if op == "+" && symType == "string" {
		if exprType == "string" || exprType == "char" || exprType == "any" {
			return
		}
		sa.addError("1510",
			fmt.Sprintf("Cannot apply '%s' to string and '%s'", assign.Op, exprType),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return
	}

	// char += string|char
	if op == "+" && symType == "char" {
		if exprType == "string" || exprType == "char" || exprType == "any" {
			return
		}
		sa.addError("1510",
			fmt.Sprintf("Cannot apply '%s' to char and '%s'", assign.Op, exprType),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return
	}

	// числовые
	if sa.isNumericType(symType) {
		if op == "%" && symType != "int" {
			sa.addError("1511",
				fmt.Sprintf("'%%=' requires int, got '%s'", symType),
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
			return
		}
		if sa.isNumericType(exprType) {
			return
		}
		sa.addError("1510",
			fmt.Sprintf("Cannot apply '%s' to '%s' and '%s'", assign.Op, symType, exprType),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return
	}

	sa.addError("1511",
		fmt.Sprintf("Operator '%s' not supported for type '%s'", assign.Op, symType),
		assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
}

func (sa *SemanticAnalyzer) analyzeTypeOf(typeOf *front.TypeOf) front.Node {
	sa.analyzeNode(typeOf.Expr)
	return typeOf
}

func (sa *SemanticAnalyzer) analyzeBinary(bin *front.BinaryExpr) front.Node {
	left := sa.analyzeNode(bin.Left)
	right := sa.analyzeNode(bin.Right)

	leftType := sa.getNodeType(left)
	rightType := sa.getNodeType(right)

	// === Union ===
	if isUnionType(leftType) || isUnionType(rightType) {
		return sa.analyzeUnionBinary(bin, leftType, rightType)
	}

	// Деление на ноль-литерал
	if bin.Op == "/" {
		if num, ok := bin.Right.(*front.Number); ok {
			if num.Value == "0" || num.Value == "0.0" {
				errors.NewWarning("1541", "Division by zero",
					bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
			}
		}
	}

	// string + string → конкатенация
	if bin.Op == "+" && leftType == "string" && rightType == "string" {
		return bin
	}

	// char + char → string (конкатенация)
	if bin.Op == "+" && leftType == "char" && rightType == "char" {
		return bin
	}
	// char + string / string + char → string
	if bin.Op == "+" && (leftType == "char" && rightType == "string" || leftType == "string" && rightType == "char") {
		return bin
	}
	// char + int / int + char → int
	if bin.Op == "+" && ((leftType == "char" && rightType == "int") || (leftType == "int" && rightType == "char")) {
		return bin
	}

	// any в арифметике — runtime dispatch, разрешено
	if leftType == "any" || rightType == "any" {
		return bin
	}

	switch bin.Op {
	case "+", "-", "*", "/", "**":
		if leftType == "void" || rightType == "void" {
			sa.addError("1553",
				"Cannot use null in arithmetic operation",
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
			return bin
		}
		// Строковая конкатенация: + со строкой допустим
		if bin.Op == "+" && leftType == "string" && rightType == "string" {
			return bin
		}
		if !sa.isNumericType(leftType) || !sa.isNumericType(rightType) {
			sa.addError("1511", fmt.Sprintf("Arithmetic operation '%s' requires numeric types (got %s and %s)",
				bin.Op, leftType, rightType),
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
		}
	case "%":
		if leftType == "void" || rightType == "void" {
			sa.addError("1553",
				"Cannot use null in arithmetic operation",
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
			return bin
		}
		if leftType != "int" || rightType != "int" {
			sa.addError("1511", fmt.Sprintf("Modulo '%%' requires int (got %s and %s)",
				leftType, rightType),
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
		}
	case "&&", "||":
		if leftType != "bool" && leftType != "" {
			sa.addError("1528",
				fmt.Sprintf("Operator '%s' requires bool, got '%s'", bin.Op, leftType),
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
		}
		if rightType != "bool" && rightType != "" {
			sa.addError("1529",
				fmt.Sprintf("Operator '%s' requires bool, got '%s'", bin.Op, rightType),
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
		}
	case "<", ">":
		if !sa.isNumericType(leftType) || !sa.isNumericType(rightType) {
			sa.addError("1512", fmt.Sprintf("Comparison operation '%s' requires numeric types (got %s and %s)",
				bin.Op, leftType, rightType),
				bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
		}
	}

	return bin
}

// analyzeUnionBinary — операция над union
func (sa *SemanticAnalyzer) analyzeUnionBinary(bin *front.BinaryExpr, leftType, rightType string) front.Node {
	leftCT := sa.getCurrentType(bin.Left)
	rightCT := sa.getCurrentType(bin.Right)

	// Собираем все возможные комбинации
	leftOptions := []string{leftType}
	if isUnionType(leftType) {
		if leftCT != "" {
			leftOptions = []string{leftCT}
		} else {
			leftOptions = parseUnionTypes(leftType)
		}
	}
	rightOptions := []string{rightType}
	if isUnionType(rightType) {
		if rightCT != "" {
			rightOptions = []string{rightCT}
		} else {
			rightOptions = parseUnionTypes(rightType)
		}
	}

	validCount := 0
	invalidCombos := []string{}
	for _, l := range leftOptions {
		for _, r := range rightOptions {
			if sa.isBinaryOpValid(bin.Op, l, r) {
				validCount++
			} else {
				invalidCombos = append(invalidCombos, l+" "+bin.Op+" "+r)
			}
		}
	}

	totalCombos := len(leftOptions) * len(rightOptions)

	if validCount == 0 {
		sa.addError("0613",
			fmt.Sprintf("Operation '%s' is not valid for any combination in union", bin.Op),
			bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
	} else if validCount < totalCombos {
		sa.addError("0614",
			fmt.Sprintf("Operation '%s' may fail at runtime: not valid for %s",
				bin.Op, strings.Join(invalidCombos, ", ")),
			bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
	}

	return bin
}

// getCurrentType — узнать CurrentType выражения (для union-переменных)
func (sa *SemanticAnalyzer) getCurrentType(node front.Node) string {
	if ident, ok := node.(*front.Ident); ok {
		sym := sa.CurrentScope.Resolve(ident.Name)
		if sym != nil && isUnionType(sym.Type) {
			return sym.CurrentType
		}
	}
	return ""
}

// isBinaryOpValid — можно ли применить op к типам l и r
func (sa *SemanticAnalyzer) isBinaryOpValid(op, l, r string) bool {
	switch op {
	case "+":
		if l == "string" || r == "string" {
			return true // concat
		}
		return sa.isNumericType(l) && sa.isNumericType(r)
	case "-", "*", "/", "**":
		return sa.isNumericType(l) && sa.isNumericType(r)
	case "%":
		return l == "int" && r == "int"
	case "&&", "||":
		return l == "bool" && r == "bool"
	case "<", ">", "<=", ">=", "==", "!=":
		return sa.isNumericType(l) && sa.isNumericType(r)
	}
	return false
}

func (sa *SemanticAnalyzer) analyzeNumber(num *front.Number) front.Node {
	value := num.Value
	if strings.HasSuffix(value, "f") || strings.HasSuffix(value, "F") {
		value = value[:len(value)-1]
	}
	if _, err := strconv.Atoi(value); err != nil {
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			sa.addError("1513", fmt.Sprintf("Invalid number '%s'", num.Value),
				num.GetLine(), num.GetColumn(), sa.CurrentFile)
		}
	}
	return num
}

func (sa *SemanticAnalyzer) analyzeString(str *front.String) front.Node {
	return str
}

func (sa *SemanticAnalyzer) analyzeChar(ch *front.CharLiteral) front.Node {
	return ch
}

func (sa *SemanticAnalyzer) analyzeIdent(ident *front.Ident) front.Node {
	if ident.Name == "true" || ident.Name == "false" {
		return ident
	}

	sym := sa.CurrentScope.Resolve(ident.Name)
	if sym == nil {
		sa.addError("1514", fmt.Sprintf("Undefined identifier '%s'", ident.Name),
			ident.GetLine(), ident.GetColumn(), sa.CurrentFile)
	}
	return ident
}

func (sa *SemanticAnalyzer) analyzeReturn(ret *front.ReturnStmt) front.Node {
	if sa.CurrentFunction.ReturnType == "void" {
		if ret.Expr != nil {
			sa.addError("1515", "Cannot return value from void function",
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
		}
		return ret
	}

	// Union return
	if isUnionType(sa.CurrentFunction.ReturnType) {
		if ret.Expr == nil {
			sa.addError("1517",
				fmt.Sprintf("Expected return value of type '%s'", sa.CurrentFunction.ReturnType),
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
			return ret
		}
		exprType := sa.getNodeType(ret.Expr)
		if exprType == "void" {
			return ret
		}
		if !isTypeInUnion(sa.CurrentFunction.ReturnType, exprType) {
			sa.addError("0612",
				fmt.Sprintf("Return type '%s' is not in union '%s'",
					exprType, sa.CurrentFunction.ReturnType),
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
		}
		return ret
	}

	// Обычный return
	if ret.Expr != nil {
		exprType := sa.getNodeType(ret.Expr)
		if exprType == "void" {
			return ret
		}
		if sa.CurrentFunction.ReturnType != exprType && exprType != "" {
			sa.addError("1516", fmt.Sprintf("Return type mismatch: expected '%s', got '%s'",
				sa.CurrentFunction.ReturnType, exprType),
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
			return ret
		}
	} else {
		sa.addError("1517", fmt.Sprintf("Expected return value of type '%s'", sa.CurrentFunction.ReturnType),
			ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
		return ret
	}
	return ret
}

func (sa *SemanticAnalyzer) analyzeCall(call *front.CallExpr) front.Node {
	debug.Debug("analyzeCall: %s\n", call.Name)

	// Если Receiver задан — добавляем его в Args (как первый аргумент).
	args := call.Args
	if call.ReceiverNode != nil {
		args = append([]front.Node{call.ReceiverNode}, args...)
	} else if call.Receiver != "" {
		receiverNode := &front.Ident{
			Position: front.Position{
				Line:   call.GetLine(),
				Column: call.GetColumn(),
			},
			Name: call.Receiver,
		}
		args = append([]front.Node{receiverNode}, args...)
	}

	// Нормализуем "s.to_int" → "to_int", "io.sendln" → "sendln"
	simpleName := call.Name
	if strings.Contains(simpleName, ".") {
		parts := strings.Split(simpleName, ".")
		simpleName = parts[len(parts)-1]
	}
	if simpleName == "isAsyncEnded" {
		return sa.analyzeIsAsyncEnded(call, args)
	}

	builtinFuncs := map[string]bool{
		"to_int":         true,
		"to_float":       true,
		"to_double":      true,
		"isAsyncEnded":   true,
		"to_string":      true,
		"to_bool":        true,
		"to_arr":         true,
		"detruncate":     true,
		"getConfig":      true,
		"getConfigField": true,
	}
	if builtinFuncs[simpleName] {
		for _, arg := range args {
			sa.analyzeNode(arg)
		}
		return call
	}

	// Проверяем, не является ли имя переменной (не функцией)
	sym := sa.CurrentScope.Resolve(simpleName)
	if sym != nil && sym.Kind != SYM_FUNCTION {
		sa.addError("1539",
			fmt.Sprintf("'%s' is not a function", simpleName),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}

	targetFunc := sa.resolveFunction(simpleName)
	if targetFunc == nil {
		debug.Debug("Function %s not found\n", simpleName)
		sa.addError("1518", fmt.Sprintf("Undefined function '%s'", simpleName),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}
	debug.Debug("Function %s found, params=%d\n", simpleName, len(targetFunc.Params))

	// Считаем обязательные, опциональные и variadic
	requiredParams := 0
	optionalParams := 0
	hasVariadic := false
	for _, p := range targetFunc.Params {
		if p.IsVariadic {
			hasVariadic = true
			break
		}
		if p.DefaultValue != nil {
			optionalParams++
		} else {
			requiredParams++
		}
	}
	totalNormal := requiredParams + optionalParams

	if hasVariadic {
		if len(args) < requiredParams {
			sa.addError("1519", fmt.Sprintf(
				"Function '%s' expects at least %d arguments, got %d",
				simpleName, requiredParams, len(args)),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return call
		}
	} else {
		if len(args) < requiredParams {
			sa.addError("1519", fmt.Sprintf(
				"Function '%s' expects at least %d arguments, got %d",
				simpleName, requiredParams, len(args)),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return call
		}
		if len(args) > totalNormal {
			sa.addError("1519", fmt.Sprintf(
				"Function '%s' expects at most %d arguments, got %d",
				simpleName, totalNormal, len(args)),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return call
		}
	}

	// Сколько "реальных" (переданных) параметров идут до variadic
	realNormal := len(args)
	if hasVariadic && realNormal > totalNormal {
		realNormal = totalNormal
	}

	for i, arg := range args {
		sa.analyzeNode(arg)

		// variadic-хвост
		if hasVariadic && i >= totalNormal {
			continue
		}

		argType := sa.getNodeType(arg)
		paramType := targetFunc.Params[i].Type

		debug.Debug("Arg %d: type=%s, expected=%s\n", i, argType, paramType)

		// Union-параметр
		if isUnionType(paramType) {
			if argType == "void" {
				sa.addError("1552",
					fmt.Sprintf("Cannot pass null to union parameter %d", i+1),
					arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
				return call
			}
			if !isTypeInUnion(paramType, argType) && argType != "" {
				sa.addError("0612",
					fmt.Sprintf("Argument %d: type '%s' is not in union '%s'",
						i+1, argType, paramType),
					arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
				return call
			}
			continue
		}

		if argType == "void" && paramType == "any" {
			sa.addError("1552",
				fmt.Sprintf("Cannot pass null to 'any' parameter %d — cannot predict future value type", i+1),
				arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
			return call
		}

		if argType != paramType && argType != "" && paramType != "any" {
			debug.Debug("Type mismatch in argument %d\n", i)
			sa.addError("1520", fmt.Sprintf("Argument %d type mismatch: expected '%s', got '%s'",
				i+1, paramType, argType),
				arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
			return call
		}
	}
	_ = realNormal

	debug.Debug("analyzeCall completed successfully\n")
	return call
}

func (sa *SemanticAnalyzer) analyzeIsAsyncEnded(call *front.CallExpr, args []front.Node) front.Node {
	if len(args) != 1 {
		sa.addError("0666",
			"isAsyncEnded requires exactly one argument (async function name as string)",
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}
	lit, ok := args[0].(*front.String)
	if !ok {
		sa.addError("0667",
			"isAsyncEnded argument must be a string literal with the async function name",
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}
	target := sa.resolveFunction(lit.Value)
	if target == nil {
		sa.addError("1518",
			fmt.Sprintf("Undefined function '%s'", lit.Value),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
	} else if !target.IsAsync {
		sa.addError("0664",
			fmt.Sprintf("'%s' is not an async function", lit.Value),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
	}
	return call
}

func (sa *SemanticAnalyzer) analyzeCallRange(call *front.CallRangeExpr) front.Node {
	debug.Debug("analyzeCallRange: %s\n", call.Name)

	// Анализируем границы
	sa.analyzeNode(call.Range.Start)
	sa.analyzeNode(call.Range.End)
	for _, arg := range call.Extra {
		sa.analyzeNode(arg)
	}

	startVal := sa.getConstantInt(call.Range.Start)
	endVal := sa.getConstantInt(call.Range.End)

	if startVal < 0 || endVal < 0 {
		sa.addError("1533",
			fmt.Sprintf("Range call '%s' requires constant bounds", call.Name),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}

	rangeCount := 0
	if startVal <= endVal {
		rangeCount = endVal - startVal + 1
	} else {
		rangeCount = startVal - endVal + 1
	}

	totalArgs := rangeCount + len(call.Extra)

	targetFunc := sa.resolveFunction(call.Name)
	if targetFunc == nil {
		sa.addError("1518", fmt.Sprintf("Undefined function '%s'", call.Name),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}

	if totalArgs != len(targetFunc.Params) {
		sa.addError("1519", fmt.Sprintf(
			"Function '%s' expects %d arguments, got %d (range %d..%d expands to %d + %d extra)",
			call.Name, len(targetFunc.Params), totalArgs,
			startVal, endVal, rangeCount, len(call.Extra)),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}

	// Проверяем типы от range (все int)
	for i := 0; i < rangeCount; i++ {
		paramType := targetFunc.Params[i].Type
		if paramType != "int" && paramType != "any" {
			sa.addError("1520", fmt.Sprintf(
				"Range argument %d: expected 'int', but function '%s' param is '%s'",
				i+1, call.Name, paramType),
				call.GetLine(), call.GetColumn(), sa.CurrentFile)
			return call
		}
	}

	// Проверяем типы Extra-аргументов
	for i, arg := range call.Extra {
		argType := sa.getNodeType(arg)
		paramType := targetFunc.Params[rangeCount+i].Type
		if argType != paramType && argType != "" && paramType != "any" {
			sa.addError("1520", fmt.Sprintf(
				"Argument %d type mismatch: expected '%s', got '%s'",
				rangeCount+i+1, paramType, argType),
				arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
			return call
		}
	}

	return call
}

func (sa *SemanticAnalyzer) getConstantInt(node front.Node) int {
	switch n := node.(type) {
	case *front.Number:
		if v, err := strconv.Atoi(n.Value); err == nil {
			return v
		}
	case *front.UnaryExpr:
		if n.Op == "-" {
			inner := sa.getConstantInt(n.Expr)
			if inner >= 0 {
				return -inner
			}
		}
	}
	return -1
}

func (sa *SemanticAnalyzer) analyzeIf(ifStmt *front.IfStmt) front.Node {
	condType := sa.getNodeType(ifStmt.Condition)
	if condType != "bool" && condType != "" {
		sa.addError("1521", fmt.Sprintf("If condition must be boolean, got '%s'", condType),
			ifStmt.GetLine(), ifStmt.GetColumn(), sa.CurrentFile)
	}

	if ifStmt.Then != nil {
		sa.analyzeBlock(ifStmt.Then, false)
	}

	for _, elsif := range ifStmt.Elsifs {
		condType = sa.getNodeType(elsif.Condition)
		if condType != "bool" && condType != "" {
			sa.addError("1524", fmt.Sprintf("Elsif condition must be boolean, got '%s'", condType),
				elsif.GetLine(), elsif.GetColumn(), sa.CurrentFile)
		}
		if elsif.Then != nil {
			sa.analyzeBlock(elsif.Then, false)
		}
	}

	if ifStmt.Else != nil {
		sa.analyzeBlock(ifStmt.Else, false)
	}

	sa.resetUnionCurrentTypes()
	return ifStmt
}

func (sa *SemanticAnalyzer) analyzeCase(caseStmt *front.CaseStmt) front.Node {
	debug.Debug("analyzeCase: checking value\n")

	valueType := sa.getNodeType(caseStmt.Value)
	debug.Debug("Case value type: %s\n", valueType)

	seenPatterns := make(map[string]bool)
	for _, branch := range caseStmt.Branches {
		for _, pat := range branch.Patterns {
			patternType := sa.getNodeType(pat)
			debug.Debug("Pattern type: %s\n", patternType)

			if patternType != valueType && patternType != "" {
				sa.addError("1525",
					fmt.Sprintf("Pattern type mismatch: expected '%s', got '%s'",
						valueType, patternType),
					pat.GetLine(), pat.GetColumn(), sa.CurrentFile)
			}

			// Проверка дубликатов паттернов
			patKey := sa.getConstantValue(pat)
			if patKey != "" {
				if seenPatterns[patKey] {
					sa.addError("1540",
						fmt.Sprintf("Duplicate case pattern '%s'", patKey),
						pat.GetLine(), pat.GetColumn(), sa.CurrentFile)
				}
				seenPatterns[patKey] = true
			}
		}

		if branch.Body != nil {
			sa.analyzeBlock(branch.Body, false)
		}
	}

	if caseStmt.Default != nil {
		sa.analyzeBlock(caseStmt.Default, false)
	}

	sa.resetUnionCurrentTypes()
	return caseStmt
}

func (sa *SemanticAnalyzer) getConstantValue(node front.Node) string {
	switch n := node.(type) {
	case *front.Number:
		return n.Value
	case *front.String:
		return n.Value
	}
	return ""
}

func (sa *SemanticAnalyzer) analyzeWhile(while *front.WhileStmt) front.Node {
	sa.LoopDepth++
	defer func() { sa.LoopDepth-- }()

	if while.Condition != nil {
		condType := sa.getNodeType(while.Condition)
		if condType != "bool" && condType != "" {
			sa.addError("1522", fmt.Sprintf("While condition must be boolean, got '%s'", condType),
				while.GetLine(), while.GetColumn(), sa.CurrentFile)
		}
	}

	if while.Body != nil {
		sa.analyzeBlock(while.Body, false)
	}

	sa.resetUnionCurrentTypes()
	return while
}

func (sa *SemanticAnalyzer) analyzeForIn(forIn *front.ForInStmt) front.Node {
	sa.LoopDepth++
	defer func() { sa.LoopDepth-- }()

	iterType := sa.getNodeType(forIn.Iterable)

	// iterable должен быть arr[T], string или range
	var elemType string
	switch {
	case iterType == "string":
		elemType = "string"
	case isArrayTypeSemantic(iterType):
		elemType = parseArrayElemTypeSemantic(iterType)
		if elemType == "" {
			elemType = "any"
		}
	case iterType == "arr[int]":
		elemType = "int"
	default:
		sa.addError("0631",
			fmt.Sprintf("Cannot iterate over '%s' — expected arr or string", iterType),
			forIn.GetLine(), forIn.GetColumn(), sa.CurrentFile)
		return forIn
	}

	// Проверка типа переменной
	if forIn.VarType != elemType && forIn.VarType != "any" && elemType != "any" {
		sa.addError("0632",
			fmt.Sprintf("Element type mismatch: expected '%s', got '%s'",
				elemType, forIn.VarType),
			forIn.GetLine(), forIn.GetColumn(), sa.CurrentFile)
	}

	// Определяем переменную в scope
	sa.CurrentScope.Define(forIn.VarName, SYM_VARIABLE, forIn.VarType, false)

	if forIn.Body != nil {
		sa.analyzeBlock(forIn.Body, false)
	}

	sa.resetUnionCurrentTypes()
	return forIn
}

func (sa *SemanticAnalyzer) isValidType(typ string) bool {
	if isUnionType(typ) {
		for _, t := range parseUnionTypes(typ) {
			if !sa.isValidType(t) {
				return false
			}
			if isUnionType(t) {
				return false // вложенные запрещены
			}
			if t == "any" {
				return false // any запрещён в union
			}
		}
		return true
	}

	validTypes := map[string]bool{
		"int": true, "string": true, "float": true, "double": true,
		"bool": true, "char": true, "arr": true, "dict": true,
		"void": true, "any": true,
	}
	return validTypes[typ]
}

func (sa *SemanticAnalyzer) isNumericType(typ string) bool {
	numericTypes := map[string]bool{
		"int": true, "float": true, "double": true, "char": true,
	}
	return numericTypes[typ]
}

func (sa *SemanticAnalyzer) getNodeType(node front.Node) string {
	switch n := node.(type) {
	case *front.Number:
		if strings.HasSuffix(n.Value, "f") || strings.HasSuffix(n.Value, "F") {
			return "float"
		}
		if strings.Contains(n.Value, ".") {
			return "double"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.AwaitExpr:
		if sa.CurrentFunction == nil {
			return ""
		}
		switch e := n.Expr.(type) {
		case *front.CallExpr:
			simpleName := e.Name
			if strings.Contains(simpleName, ".") {
				parts := strings.Split(simpleName, ".")
				simpleName = parts[len(parts)-1]
			}
			if fn := sa.resolveFunction(simpleName); fn != nil {
				return fn.ReturnType
			}
		case *front.Ident:
			sym := sa.CurrentScope.Resolve(e.Name)
			if sym != nil {
				return sym.Type
			}
		}
		return ""

	case *front.PromiseSet:
		if sa.CurrentFunction != nil {
			return sa.CurrentFunction.ReturnType
		}
		return ""
	case *front.PostfixExpr:
		return "int"
	case *front.ArrayIndex:
		sym := sa.CurrentScope.Resolve(n.Name)
		if sym == nil {
			return ""
		}

		// dict → any
		if sym.Type == "dict" {
			return "any"
		}

		// arr[T] или arr → тип элемента
		if isArrayTypeSemantic(sym.Type) {
			// Если индекс — константа и есть ArrayElemTypes → реальный тип
			if len(sym.ArrayElemTypes) > 0 {
				if num, ok := n.Index.(*front.Number); ok {
					if i, err := strconv.Atoi(num.Value); err == nil {
						if i >= 0 && i < len(sym.ArrayElemTypes) {
							return sym.ArrayElemTypes[i]
						}
					}
				}
			}

			// Иначе — тип из arr[T] или "any"
			elemType := parseArrayElemTypeSemantic(sym.Type)
			if elemType == "" {
				elemType = "any"
			}
			return elemType
		}

		return ""
	case *front.UnicodeLiteral:
		return "char"
	case *front.CharLiteral:
		return "char"
	case *front.NullLiteral:
		return "void"
	case *front.DictLiteral:
		return "dict"
	case *front.ArrayLiteral:
		return "arr[any]"
	case *front.TypeOf:
		return "string"
	case *front.ErrorInstance:
		return n.TypeName
	case *front.Ident:
		if n.Name == "true" || n.Name == "false" {
			return "bool"
		}
		sym := sa.CurrentScope.Resolve(n.Name)
		if sym != nil {
			return sym.Type
		}
		return ""
	case *front.FieldAccess:
		sym := sa.CurrentScope.Resolve(n.Object)
		if sym == nil {
			return ""
		}

		// dict — ищем реальный тип ключа
		if sym.Type == "dict" {
			if sym.DictKeyTypes != nil {
				if t, ok := sym.DictKeyTypes[n.Field]; ok {
					return t
				}
			}
			return "any"
		}

		// error-тип
		decl, ok := sa.ErrorTypes[sym.Type]
		if !ok {
			return ""
		}
		allFields := sa.collectErrorFields(decl)
		if field, exists := allFields[n.Field]; exists {
			return field.Type
		}
		return ""
	case *front.BinaryExpr:
		debug.Debug("getNodeType BinaryExpr: op=%s\n", n.Op)
		leftType := sa.getNodeType(n.Left)
		rightType := sa.getNodeType(n.Right)
		debug.Debug("  leftType=%s, rightType=%s\n", leftType, rightType)

		// Сравнения и логика — bool
		if n.Op == "&&" || n.Op == "||" {
			return "bool"
		}

		if n.Op == "<" || n.Op == ">" || n.Op == "==" || n.Op == "!=" || n.Op == "<=" || n.Op == ">=" {
			return "bool"
		}

		// any в операнде → any (тип неизвестен)
		if leftType == "any" || rightType == "any" {
			return "any"
		}

		// string + string → string
		if n.Op == "+" && leftType == "string" && rightType == "string" {
			return "string"
		}
		// char + char → string
		if n.Op == "+" && leftType == "char" && rightType == "char" {
			return "string"
		}
		// char + string / string + char → string
		if n.Op == "+" && ((leftType == "char" && rightType == "string") || (leftType == "string" && rightType == "char")) {
			return "string"
		}

		isLeftNumeric := leftType == "int" || leftType == "float" || leftType == "double" || leftType == "char"
		isRightNumeric := rightType == "int" || rightType == "float" || rightType == "double" || rightType == "char"

		if isLeftNumeric && isRightNumeric {
			if leftType == "double" || rightType == "double" {
				return "double"
			}
			if leftType == "float" || rightType == "float" {
				return "float"
			}
			return "int"
		}

		return "int"

	case *front.TernaryExpr:
		thenType := sa.getNodeType(n.Then)
		elseType := sa.getNodeType(n.Else)
		if thenType == elseType {
			return thenType
		}
		if thenType == "any" || elseType == "any" {
			return "any"
		}
		return thenType

	case *front.CallExpr:
		debug.Debug("getNodeType CallExpr: %s\n", n.Name)

		// Нормализуем "s.to_int" → "to_int", "io.sendln" → "sendln"
		simpleName := n.Name
		if strings.Contains(simpleName, ".") {
			parts := strings.Split(simpleName, ".")
			simpleName = parts[len(parts)-1]
		}

		// Builtins
		switch simpleName {
		case "to_int":
			return "int"
		case "to_float":
			return "float"
		case "to_double":
			return "double"
		case "to_string":
			return "string"
		case "to_bool":
			return "bool"
		case "to_arr":
			return "arr[any]"
		case "detruncate":
			return "string"
		case "getConfig":
			return "dict"
		case "getConfigField":
			return "any"
		}

		for _, fn := range sa.Program.Functions {
			if fn.Name == simpleName {
				debug.Debug("  found in current file: %s -> %s\n", simpleName, fn.ReturnType)
				return fn.ReturnType
			}
		}

		if fn, ok := sa.ImportedFuncs[simpleName]; ok {
			debug.Debug("  found in imports: %s -> %s\n", simpleName, fn.ReturnType)
			return fn.ReturnType
		}

		if fn, ok := sa.ImportedFuncs[n.Name]; ok {
			debug.Debug("  found in imports (full): %s -> %s\n", n.Name, fn.ReturnType)
			return fn.ReturnType
		}

		// Fallback: скан по fn.Name
		for _, fn := range sa.ImportedFuncs {
			if fn.Name == simpleName {
				debug.Debug("  found in imports (scan): %s -> %s\n", simpleName, fn.ReturnType)
				return fn.ReturnType
			}
		}

		// Fallback: AllFunctions
		for _, fn := range sa.Program.AllFunctions {
			if fn.Name == simpleName {
				debug.Debug("  found in AllFunctions: %s -> %s\n", simpleName, fn.ReturnType)
				return fn.ReturnType
			}
		}

		debug.Debug("  function %s not found\n", n.Name)
		return ""

	case *front.FormatExpr:
		if n.Mode == "Nf" && n.N == 0 {
			return "int"
		}
		return sa.getNodeType(n.Expr)
	case *front.UnaryExpr:
		if n.Op == "$" {
			return "string"
		}
		return sa.getNodeType(n.Expr)

	case *front.RangeExpr:
		return "arr[int]"
	case *front.CallRangeExpr:
		for _, fn := range sa.Program.Functions {
			if fn.Name == n.Name {
				return fn.ReturnType
			}
		}
		if fn, ok := sa.ImportedFuncs[n.Name]; ok {
			return fn.ReturnType
		}
		return ""

	case *front.VarDecl:
		return n.Type

	case *front.Assign:
		sym := sa.CurrentScope.Resolve(n.Name)
		if sym != nil {
			return sym.Type
		}
		return ""

	default:
		return ""
	}
}

func (sa *SemanticAnalyzer) addError(code, message string, line, col int, file string) {
	if code == "" {
		code = "0000"
	}
	if file == "" {
		file = sa.CurrentFile
	}
	sa.Errors = append(sa.Errors, errors.SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
	})
	// Пишем в глобальный errors, чтобы printErrorReport видел
	errors.NewError(code, message, line, col, file)
}

func parseArrayElemTypeSemantic(elemType string) string {
	if elemType == "" {
		return ""
	}
	if !strings.HasPrefix(elemType, "arr[") {
		return elemType
	}
	return elemType[4 : len(elemType)-1]
}

func isArrayTypeSemantic(t string) bool {
	return t == "arr" || strings.HasPrefix(t, "arr[")
}

func (sa *SemanticAnalyzer) analyzeTernary(t *front.TernaryExpr) front.Node {
	condType := sa.getNodeType(t.Condition)
	if condType != "bool" && condType != "" {
		sa.addError("1530",
			fmt.Sprintf("Ternary condition must be bool, got '%s'", condType),
			t.GetLine(), t.GetColumn(), sa.CurrentFile)
	}

	sa.analyzeNode(t.Condition)

	thenType := sa.getNodeType(t.Then)
	elseType := sa.getNodeType(t.Else)

	if thenType != elseType && thenType != "" && elseType != "" {
		sa.addError("1531",
			fmt.Sprintf("Ternary branches type mismatch: '%s' vs '%s'", thenType, elseType),
			t.GetLine(), t.GetColumn(), sa.CurrentFile)
	}

	sa.analyzeNode(t.Then)
	sa.analyzeNode(t.Else)

	return t
}

// ============================================================================
// Helpers
// ============================================================================

func (sa *SemanticAnalyzer) hasReturn(block *front.Block) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Statements {
		if sa.nodeHasReturn(stmt) {
			return true
		}
	}
	return false
}

func (sa *SemanticAnalyzer) nodeHasReturn(node front.Node) bool {
	switch n := node.(type) {
	case *front.ReturnStmt:
		return true
	case *front.DoWhileStmt:
		return false
	case *front.Block:
		return sa.hasReturn(n)
	case *front.TryStmt:
		// try + все catch возвращают → true
		if n.Body != nil && sa.hasReturn(n.Body) {
			return true
		}
		allCatchesReturn := true
		for _, clause := range n.Catches {
			if clause.Body == nil || !sa.hasReturn(clause.Body) {
				allCatchesReturn = false
				break
			}
		}
		if len(n.Catches) > 0 && allCatchesReturn {
			return true
		}
		return false
	case *front.IfStmt:
		if sa.hasReturn(n.Then) {
			return true
		}
		allElsifsReturn := true
		for _, elsif := range n.Elsifs {
			if !sa.hasReturn(elsif.Then) {
				allElsifsReturn = false
				break
			}
		}
		if n.Else != nil && sa.hasReturn(n.Else) && allElsifsReturn {
			return true
		}
		return false
	case *front.CaseStmt:
		if n.Default != nil && sa.hasReturn(n.Default) {
			for _, branch := range n.Branches {
				if !sa.hasReturn(branch.Body) {
					return false
				}
			}
			return true
		}
		return false
	case *front.WhileStmt:
		return false
	case *front.ForInStmt:
		return false
	}
	return false
}

func (sa *SemanticAnalyzer) collectUsedIdents(node front.Node, used map[string]bool) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *front.Block:
		for _, stmt := range n.Statements {
			if stmt != nil {
				sa.collectUsedIdents(stmt, used)
			}
		}
	case *front.PostfixExpr:
		used[n.VarName] = true
	case *front.AwaitExpr:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.PromiseSet:
		if n.Value != nil {
			sa.collectUsedIdents(n.Value, used)
		}
	case *front.RangeExpr:
		if n.Start != nil {
			sa.collectUsedIdents(n.Start, used)
		}
		if n.End != nil {
			sa.collectUsedIdents(n.End, used)
		}
	case *front.BreakStmt:
		// ничего
	case *front.ContinueStmt:
		// ничего
	case *front.DoWhileStmt:
		if n.Body != nil {
			sa.collectUsedIdents(n.Body, used)
		}
		if n.Condition != nil {
			sa.collectUsedIdents(n.Condition, used)
		}
	case *front.IncludeC:
		// includeC — сырой C, обрабатывается отдельно через containsIncludeC
	case *front.VarDecl:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.Assign:
		used[n.Name] = true
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.Ident:
		used[n.Name] = true
	case *front.BinaryExpr:
		if n.Left != nil {
			sa.collectUsedIdents(n.Left, used)
		}
		if n.Right != nil {
			sa.collectUsedIdents(n.Right, used)
		}
	case *front.UnaryExpr:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.CallExpr:
		for _, arg := range n.Args {
			if arg != nil {
				sa.collectUsedIdents(arg, used)
			}
		}
	case *front.CallRangeExpr:
		if n.Range != nil {
			if n.Range.Start != nil {
				sa.collectUsedIdents(n.Range.Start, used)
			}
			if n.Range.End != nil {
				sa.collectUsedIdents(n.Range.End, used)
			}
		}
		for _, arg := range n.Extra {
			if arg != nil {
				sa.collectUsedIdents(arg, used)
			}
		}
	case *front.ReturnStmt:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.IfStmt:
		if n.Condition != nil {
			sa.collectUsedIdents(n.Condition, used)
		}
		if n.Then != nil {
			sa.collectUsedIdents(n.Then, used)
		}
		for _, elsif := range n.Elsifs {
			if elsif.Condition != nil {
				sa.collectUsedIdents(elsif.Condition, used)
			}
			if elsif.Then != nil {
				sa.collectUsedIdents(elsif.Then, used)
			}
		}
		if n.Else != nil {
			sa.collectUsedIdents(n.Else, used)
		}
	case *front.WhileStmt:
		if n.Condition != nil {
			sa.collectUsedIdents(n.Condition, used)
		}
		if n.Body != nil {
			sa.collectUsedIdents(n.Body, used)
		}
	case *front.ForInStmt:
		if n.Iterable != nil {
			sa.collectUsedIdents(n.Iterable, used)
		}
		if n.Body != nil {
			sa.collectUsedIdents(n.Body, used)
		}
	case *front.TryStmt:
		if n.Body != nil {
			sa.collectUsedIdents(n.Body, used)
		}
		for _, clause := range n.Catches {
			if clause.VarName != "" {
				used[clause.VarName] = true
			}
			if clause.Body != nil {
				sa.collectUsedIdents(clause.Body, used)
			}
		}
	case *front.CaseStmt:
		if n.Value != nil {
			sa.collectUsedIdents(n.Value, used)
		}
		for _, branch := range n.Branches {
			for _, pat := range branch.Patterns {
				sa.collectUsedIdents(pat, used)
			}
			if branch.Body != nil {
				sa.collectUsedIdents(branch.Body, used)
			}
		}
		if n.Default != nil {
			sa.collectUsedIdents(n.Default, used)
		}
	case *front.TypeOf:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.ArrayLiteral:
		for _, elem := range n.Elements {
			if elem != nil {
				sa.collectUsedIdents(elem, used)
			}
		}
	case *front.ArrayIndex:
		used[n.Name] = true
		if n.Index != nil {
			sa.collectUsedIdents(n.Index, used)
		}
	case *front.ArrayLength:
		used[n.Name] = true
	case *front.ArrayAdd:
		used[n.Name] = true
		if n.Elem != nil {
			sa.collectUsedIdents(n.Elem, used)
		}
	case *front.TernaryExpr:
		if n.Condition != nil {
			sa.collectUsedIdents(n.Condition, used)
		}
		if n.Then != nil {
			sa.collectUsedIdents(n.Then, used)
		}
		if n.Else != nil {
			sa.collectUsedIdents(n.Else, used)
		}
	case *front.ThrowStmt:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.ErrorInstance:
		if n.Fields != nil {
			for _, value := range n.Fields {
				if value != nil {
					sa.collectUsedIdents(value, used)
				}
			}
		}
	case *front.FieldAccess:
		used[n.Object] = true
	}
}

func (sa *SemanticAnalyzer) checkUnusedVariables(block *front.Block, used map[string]bool) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		switch n := stmt.(type) {
		case *front.VarDecl:
			if !used[n.Name] {
				errors.NewWarning("2000",
					fmt.Sprintf("Unused variable '%s'", n.Name),
					n.GetLine(), n.GetColumn(), sa.CurrentFile)
			}
		case *front.IfStmt:
			sa.checkUnusedVariables(n.Then, used)
			for _, elsif := range n.Elsifs {
				sa.checkUnusedVariables(elsif.Then, used)
			}
			if n.Else != nil {
				sa.checkUnusedVariables(n.Else, used)
			}
		case *front.WhileStmt:
			sa.checkUnusedVariables(n.Body, used)
		case *front.ForInStmt:
			sa.checkUnusedVariables(n.Body, used)
		case *front.Block:
			sa.checkUnusedVariables(n, used)
		case *front.CaseStmt:
			for _, branch := range n.Branches {
				sa.checkUnusedVariables(branch.Body, used)
			}
			if n.Default != nil {
				sa.checkUnusedVariables(n.Default, used)
			}
		}
	}
}

func (sa *SemanticAnalyzer) registerErrorDecl(decl *front.ErrorDecl) {
	// Проверка дублей
	if _, exists := sa.ErrorTypes[decl.Name]; exists {
		sa.addError("1500",
			fmt.Sprintf("Error type '%s' already declared", decl.Name),
			decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
		return
	}

	// Родитель должен существовать
	parent, ok := sa.ErrorTypes[decl.Parent]
	if !ok {
		sa.addError("1549",
			fmt.Sprintf("Unknown parent error type '%s'", decl.Parent),
			decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
		return
	}

	// Проверка полей на конфликты с родителем
	parentFields := make(map[string]*front.ErrorField)
	for _, f := range parent.Fields {
		parentFields[f.Name] = f
	}

	for _, field := range decl.Fields {
		if parentField, ok := parentFields[field.Name]; ok {
			if parentField.Type != field.Type {
				sa.addError("1550",
					fmt.Sprintf("Field '%s' type mismatch with parent '%s': expected '%s', got '%s'",
						field.Name, decl.Parent, parentField.Type, field.Type),
					field.GetLine(), field.GetColumn(), sa.CurrentFile)
			}
		}
	}

	// Проверка типов полей
	for _, field := range decl.Fields {
		if !sa.isValidType(field.Type) {
			sa.addError("1506",
				fmt.Sprintf("Unknown field type '%s'", field.Type),
				field.GetLine(), field.GetColumn(), sa.CurrentFile)
		}
	}

	// Регистрация
	sa.ErrorTypes[decl.Name] = decl
	sa.GlobalScope.Define(decl.Name, SYM_CONST, decl.Name, true)
}

func (sa *SemanticAnalyzer) analyzeErrorInstance(inst *front.ErrorInstance) front.Node {
	// Тип должен существовать
	decl, ok := sa.ErrorTypes[inst.TypeName]
	if !ok {
		sa.addError("1560",
			fmt.Sprintf("Unknown error type '%s'", inst.TypeName),
			inst.GetLine(), inst.GetColumn(), sa.CurrentFile)
		return inst
	}

	// Собираем все поля из цепочки наследования
	allFields := sa.collectErrorFields(decl)

	// Проверяем, что все переданные поля существуют
	for fieldName, value := range inst.Fields {
		field, exists := allFields[fieldName]
		if !exists {
			sa.addError("1547",
				fmt.Sprintf("Error type '%s' has no field '%s'", inst.TypeName, fieldName),
				value.GetLine(), value.GetColumn(), sa.CurrentFile)
			continue
		}

		// Проверяем тип значения
		valueType := sa.getNodeType(value)
		if valueType != field.Type && valueType != "" {
			sa.addError("1532",
				fmt.Sprintf("Field '%s' type mismatch: expected '%s', got '%s'",
					fieldName, field.Type, valueType),
				value.GetLine(), value.GetColumn(), sa.CurrentFile)
		}

		// Анализируем значение
		sa.analyzeNode(value)
	}

	// Проверяем, что все поля без default переданы
	for fieldName, field := range allFields {
		if _, passed := inst.Fields[fieldName]; !passed {
			if field.DefaultValue == nil {
				sa.addError("1551",
					fmt.Sprintf("Missing required field '%s' in error '%s'",
						fieldName, inst.TypeName),
					inst.GetLine(), inst.GetColumn(), sa.CurrentFile)
			}
		}
	}

	return inst
}

// collectErrorFields собирает все поля из цепочки наследования
func (sa *SemanticAnalyzer) collectErrorFields(decl *front.ErrorDecl) map[string]*front.ErrorField {
	fields := make(map[string]*front.ErrorField)

	// Сначала родители (чтобы потомки могли переопределять)
	if decl.Parent != "" {
		if parent, ok := sa.ErrorTypes[decl.Parent]; ok {
			parentFields := sa.collectErrorFields(parent)
			for name, field := range parentFields {
				fields[name] = field
			}
		}
	}

	// Потом свои
	for _, field := range decl.Fields {
		fields[field.Name] = field
	}

	return fields
}

func (sa *SemanticAnalyzer) analyzeThrow(throw *front.ThrowStmt) front.Node {
	if throw.Expr == nil {
		sa.addError("0000", "throw requires an expression",
			throw.GetLine(), throw.GetColumn(), sa.CurrentFile)
		return throw
	}

	exprType := sa.getNodeType(throw.Expr)

	// Проверяем, что это Error-like тип
	if _, ok := sa.ErrorTypes[exprType]; !ok {
		sa.addError("1548",
			fmt.Sprintf("Cannot throw non-error type '%s'", exprType),
			throw.GetLine(), throw.GetColumn(), sa.CurrentFile)
		return throw
	}

	sa.analyzeNode(throw.Expr)
	return throw
}

func (sa *SemanticAnalyzer) containsIncludeC(node front.Node) bool {
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *front.IncludeC:
		return true
	case *front.Block:
		for _, stmt := range n.Statements {
			if sa.containsIncludeC(stmt) {
				return true
			}
		}
	case *front.IfStmt:
		if n.Then != nil && sa.containsIncludeC(n.Then) {
			return true
		}
		for _, elsif := range n.Elsifs {
			if elsif.Then != nil && sa.containsIncludeC(elsif.Then) {
				return true
			}
		}
		if n.Else != nil && sa.containsIncludeC(n.Else) {
			return true
		}
	case *front.WhileStmt:
		return sa.containsIncludeC(n.Body)
	case *front.ForInStmt:
		if n.Body != nil {
			return sa.containsIncludeC(n.Body)
		}
		return false
	case *front.TryStmt:
		if n.Body != nil && sa.containsIncludeC(n.Body) {
			return true
		}
		for _, clause := range n.Catches {
			if clause.Body != nil && sa.containsIncludeC(clause.Body) {
				return true
			}
		}
	case *front.CaseStmt:
		for _, branch := range n.Branches {
			if branch.Body != nil && sa.containsIncludeC(branch.Body) {
				return true
			}
		}
		if n.Default != nil && sa.containsIncludeC(n.Default) {
			return true
		}
	}
	return false
}

func (sa *SemanticAnalyzer) addErrorSpan(code, message string, line, col, endLine, endCol int, file string) {
	if code == "" {
		code = "0000"
	}
	if file == "" {
		file = sa.CurrentFile
	}
	sa.Errors = append(sa.Errors, errors.SkorpionError{
		Code:      code,
		Message:   message,
		Line:      line,
		Column:    col,
		EndLine:   endLine,
		EndColumn: endCol,
		File:      file,
	})
	errors.NewErrorSpan(code, message, line, col, endLine, endCol, file)
}

// ============================================================================
// Union types (T<...>)
// ============================================================================

// isUnionType проверяет, является ли тип union "T<A,B,...>"
func isUnionType(t string) bool {
	return strings.HasPrefix(t, "T<") && strings.HasSuffix(t, ">")
}

// parseUnionTypes извлекает список типов из "T<A,B,C>"
func parseUnionTypes(t string) []string {
	if !isUnionType(t) {
		return nil
	}
	inner := t[2 : len(t)-1]
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// isTypeInUnion проверяет, входит ли t в union
func isTypeInUnion(union, t string) bool {
	types := parseUnionTypes(union)
	for _, u := range types {
		if u == t {
			return true
		}
	}
	return false
}

// resetUnionCurrentTypes сбрасывает CurrentType всех union-символов
// во всех scope от текущего до глобального.
func (sa *SemanticAnalyzer) resetUnionCurrentTypes() {
	for s := sa.CurrentScope; s != nil; s = s.Parent {
		for _, sym := range s.Symbols {
			if isUnionType(sym.Type) {
				sym.CurrentType = ""
			}
		}
	}
}

// analyzeFormatExpr — проверка ^Nf / ^X...
func (sa *SemanticAnalyzer) analyzeFormatExpr(fe *front.FormatExpr) front.Node {
	sa.analyzeNode(fe.Expr)

	exprType := sa.getNodeType(fe.Expr)

	// Только для numeric
	if !sa.isNumericType(exprType) && exprType != "" {
		sa.addError("0626",
			fmt.Sprintf("Format operator '^' can only be applied to numeric types, got '%s'", exprType),
			fe.GetLine(), fe.GetColumn(), sa.CurrentFile)
		return fe
	}

	return fe
}

func (sa *SemanticAnalyzer) analyzeArrayIndex(idx *front.ArrayIndex) front.Node {
	sym := sa.CurrentScope.Resolve(idx.Name)
	if sym == nil {
		sa.addError("1514",
			fmt.Sprintf("Undefined identifier '%s'", idx.Name),
			idx.GetLine(), idx.GetColumn(), sa.CurrentFile)
		return idx
	}

	if sym.Type == "dict" {
		sa.analyzeNode(idx.Index)
		return idx
	}

	if !isArrayTypeSemantic(sym.Type) {
		sa.addError("1539",
			fmt.Sprintf("Cannot index non-array variable '%s' (type '%s')",
				idx.Name, sym.Type),
			idx.GetLine(), idx.GetColumn(), sa.CurrentFile)
		return idx
	}

	// Индекс — int
	indexType := sa.getNodeType(idx.Index)
	if indexType != "int" && indexType != "" {
		sa.addError("1520",
			fmt.Sprintf("Array index must be int, got '%s'", indexType),
			idx.Index.GetLine(), idx.Index.GetColumn(), sa.CurrentFile)
	}

	sa.analyzeNode(idx.Index)
	return idx
}

func (sa *SemanticAnalyzer) analyzeBreak(br *front.BreakStmt) front.Node {
	if sa.LoopDepth == 0 {
		sa.addError("1561",
			"'break' outside of a loop",
			br.GetLine(), br.GetColumn(), sa.CurrentFile)
	}
	return br
}

func (sa *SemanticAnalyzer) analyzeContinue(cont *front.ContinueStmt) front.Node {
	if sa.LoopDepth == 0 {
		sa.addError("1562",
			"'continue' outside of a loop",
			cont.GetLine(), cont.GetColumn(), sa.CurrentFile)
	}
	return cont
}

func (sa *SemanticAnalyzer) analyzeDoWhile(dw *front.DoWhileStmt) front.Node {
	sa.LoopDepth++
	defer func() { sa.LoopDepth-- }()

	if dw.Body != nil {
		sa.analyzeBlock(dw.Body, false)
	}

	if dw.Condition != nil {
		condType := sa.getNodeType(dw.Condition)
		if condType != "bool" && condType != "" {
			sa.addError("1563",
				fmt.Sprintf("Do-while condition must be boolean, got '%s'", condType),
				dw.GetLine(), dw.GetColumn(), sa.CurrentFile)
		}
		sa.analyzeNode(dw.Condition)
	}

	return dw
}

func (sa *SemanticAnalyzer) analyzePromiseSet(ps *front.PromiseSet) front.Node {
	if sa.CurrentFunction == nil || !sa.CurrentFunction.IsAsync {
		sa.addError("0662",
			"'Promise[...]' can only be used inside an async function",
			ps.GetLine(), ps.GetColumn(), sa.CurrentFile)
		return ps
	}

	// Только литерал: Number, String, CharLiteral, UnicodeLiteral,
	// NullLiteral, true, false
	if !isLiteralNode(ps.Value) {
		sa.addError("0671",
			"Promise[...] requires a literal (number, string, char, true, false, null)",
			ps.Value.GetLine(), ps.Value.GetColumn(), sa.CurrentFile)
		return ps
	}

	valueType := sa.getNodeType(ps.Value)
	if valueType != "" && valueType != sa.CurrentFunction.ReturnType {
		sa.addError("0669",
			fmt.Sprintf("Promise[...] value type '%s' does not match async return type '%s'",
				valueType, sa.CurrentFunction.ReturnType),
			ps.GetLine(), ps.GetColumn(), sa.CurrentFile)
	}

	sa.analyzeNode(ps.Value)
	return ps
}

// isLiteralNode — литерал (разрешён в Promise[...])
func isLiteralNode(n front.Node) bool {
	switch n.(type) {
	case *front.Number, *front.String, *front.CharLiteral,
		*front.UnicodeLiteral, *front.NullLiteral:
		return true
	case *front.Ident:
		id := n.(*front.Ident)
		return id.Name == "true" || id.Name == "false"
	}
	return false
}

func (sa *SemanticAnalyzer) analyzeAwait(aw *front.AwaitExpr) front.Node {
	switch e := aw.Expr.(type) {
	case *front.CallExpr:
		simpleName := e.Name
		if strings.Contains(simpleName, ".") {
			parts := strings.Split(simpleName, ".")
			simpleName = parts[len(parts)-1]
		}
		target := sa.resolveFunction(simpleName)
		if target == nil {
			sa.addError("1518",
				fmt.Sprintf("Undefined function '%s'", simpleName),
				aw.GetLine(), aw.GetColumn(), sa.CurrentFile)
			return aw
		}
		if !target.IsAsync {
			sa.addError("0664",
				fmt.Sprintf("'await' applied to '%s', which is not an async function", simpleName),
				aw.GetLine(), aw.GetColumn(), sa.CurrentFile)
			return aw
		}
		// async void разрешён: await дождётся завершения, значения нет.
		sa.analyzeNode(e)
	case *front.Ident:
		sym := sa.CurrentScope.Resolve(e.Name)
		if sym == nil {
			sa.addError("1514",
				fmt.Sprintf("Undefined identifier '%s'", e.Name),
				aw.GetLine(), aw.GetColumn(), sa.CurrentFile)
			return aw
		}
		// если это promise-переменная (RealType == "promise"), её Type — declared тип.
		// async void переменных быть не может (0678), так что здесь Type != "void".
	default:
		sa.addError("0663",
			"'await' requires a function call or a promise variable",
			aw.GetLine(), aw.GetColumn(), sa.CurrentFile)
	}
	return aw
}
