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
	Program         *front.Program
	Errors          []errors.SkorpionError
	ImportedFuncs   map[string]*front.Function
	ImportManager   *front.ImportManager
}

func NewSemanticAnalyzer(prog *front.Program) *SemanticAnalyzer {
	sa := &SemanticAnalyzer{
		Program:       prog,
		GlobalScope:   NewScope(nil, true),
		ImportedFuncs: make(map[string]*front.Function),
		ImportManager: nil,
	}
	return sa
}

func (sa *SemanticAnalyzer) SetImportManager(im *front.ImportManager) {
	sa.ImportManager = im
	sa.registerImportedFunctions()
}

func (sa *SemanticAnalyzer) registerImportedFunctions() {
	if sa.ImportManager == nil {
		return
	}

	allFunctions := sa.ImportManager.GetAllFunctions()
	for _, fn := range allFunctions {
		if front.IsExportable(fn) {
			for _, imp := range sa.Program.Imports {
				// Сохраняем оригинальное имя функции (без префикса)
				// Но ключ для поиска используем с префиксом
				var key string

				if imp.Alias != "" {
					key = imp.Alias + "." + fn.Name
				} else if imp.All {
					key = fn.Name
				} else {
					moduleName := front.GetModuleName(imp.Path)
					key = moduleName + "." + fn.Name
				}

				sa.ImportedFuncs[key] = fn
			}
		}
	}
}

func (sa *SemanticAnalyzer) resolveFunction(name string) *front.Function {
	debug.Debug("resolveFunction: %s\n", name)

	// Сначала ищем в текущем файле
	for _, fn := range sa.Program.Functions {
		if fn.Name == name {
			debug.Debug("Found in current file: %s\n", name)
			return fn
		}
	}

	// Потом в импортированных
	if fn, ok := sa.ImportedFuncs[name]; ok {
		debug.Debug("Found in imports: %s\n", name)
		return fn
	}

	// Проверяем с префиксом модуля (io.sendln → sendln)
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		simpleName := parts[len(parts)-1]
		debug.Debug("Trying simple name: %s\n", simpleName)

		// Ищем в текущем файле
		for _, fn := range sa.Program.Functions {
			if fn.Name == simpleName {
				debug.Debug("Found in current file (simple): %s\n", simpleName)
				return fn
			}
		}

		// Ищем в импортированных
		if fn, ok := sa.ImportedFuncs[simpleName]; ok {
			debug.Debug("Found in imports (simple): %s\n", simpleName)
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
		sa.CurrentScope = NewScope(sa.GlobalScope, false)
		sa.analyzeFunction(fn)

		if len(sa.Errors) > 0 {
			debug.Debug("Errors found after analyzing %s: %d\n", fn.Name, len(sa.Errors))
			// Выводим все ошибки
			for _, err := range sa.Errors {
				debug.Debug("Error: %s: %s\n", err.Code, err.Message)
			}
			return false
		}
	}

	debug.Debug("All functions analyzed, checking for errors...\n")

	// Здесь может быть ошибка!
	debug.Debug("Total errors: %d\n", len(sa.Errors))

	if len(sa.Errors) > 0 {
		debug.Debug("Errors found at the end: %d\n", len(sa.Errors))
		for _, err := range sa.Errors {
			debug.Debug("Final Error: %s: %s\n", err.Code, err.Message)
		}
	}

	return len(sa.Errors) == 0
}

func (sa *SemanticAnalyzer) isImportedFunction(name string) bool {
	debug.Debug("isImportedFunction: checking %s\n", name)
	// Проверяем по всем импортированным функциям
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
}

func (sa *SemanticAnalyzer) registerFunction(fn *front.Function) {
	if existing := sa.GlobalScope.Resolve(fn.Name); existing != nil {
		sa.addError("1003", fmt.Sprintf("Function '%s' already declared", fn.Name), 0, 0, "")
		return
	}
	sa.GlobalScope.Define(fn.Name, SYM_FUNCTION, fn.ReturnType, fn.IsExport)
}

func (sa *SemanticAnalyzer) checkMain() bool {
	mainSym := sa.GlobalScope.Resolve("main")
	if mainSym == nil {
		sa.addError("1004", "No main() function found", 0, 0, "")
		return false
	}

	if mainSym.Type != "void" {
		sa.addError("1005", "main() must return void", 0, 0, "")
		return false
	}

	for _, fn := range sa.Program.Functions {
		if fn.Name == "main" {
			if len(fn.Params) != 1 {
				sa.addError("1006", "main() must take exactly one parameter (arr args)", 0, 0, "")
				return false
			}
			if fn.Params[0].Type != "arr" {
				sa.addError("1007", "main() parameter must be of type 'arr'", 0, 0, "")
				return false
			}
			break
		}
	}

	return true
}

func (sa *SemanticAnalyzer) analyzeFunction(fn *front.Function) {
	debug.Debug("analyzeFunction: %s\n", fn.Name)

	sa.CurrentScope = NewScope(sa.GlobalScope, false)
	defer func() {
		debug.Debug("analyzeFunction: exiting %s\n", fn.Name)
		sa.CurrentScope = sa.CurrentScope.Parent
	}()

	// Регистрируем параметры
	for _, param := range fn.Params {
		debug.Debug("Adding parameter: %s %s\n", param.Name, param.Type)
		sa.CurrentScope.Define(param.Name, SYM_VARIABLE, param.Type, false)
	}

	if fn.Body != nil {
		debug.Debug("Analyzing body of %s\n", fn.Name)
		sa.analyzeBlock(fn.Body, true)
		debug.Debug("Body of %s analyzed\n", fn.Name)
	} else {
		debug.Debug("Function %s has no body\n", fn.Name)
	}

	debug.Debug("analyzeFunction %s completed\n", fn.Name)
}

func (sa *SemanticAnalyzer) analyzeBlock(block *front.Block, isFunctionBody bool) {
	debug.Debug("analyzeBlock: %d statements, isFunctionBody=%v\n", len(block.Statements), isFunctionBody)
	for i, stmt := range block.Statements {
		debug.Debug("Statement %d: %T\n", i, stmt)
		sa.analyzeNode(stmt)
		debug.Debug("Statement %d processed\n", i)
	}
	debug.Debug("analyzeBlock completed\n")
}

func (sa *SemanticAnalyzer) analyzeNode(node front.Node) front.Node {
	debug.Debug("analyzeNode: %T\n", node)
	switch n := node.(type) {
	case *front.VarDecl:
		return sa.analyzeVarDecl(n)
	case *front.Assign:
		return sa.analyzeAssign(n)
	case *front.BinaryExpr:
		return sa.analyzeBinary(n)
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
	case *front.Block:
		sa.analyzeBlock(n, false)
		return n
	case *front.IfStmt:
		return sa.analyzeIf(n)
	case *front.WhileStmt:
		return sa.analyzeWhile(n)
	case *front.ForStmt:
		return sa.analyzeFor(n)
	case *front.IncludeC:
		debug.Debug("IncludeC node found\n")
		return n
	default:
		debug.Debug("Unknown node type: %T\n", n)
		return n
	}
}

func (sa *SemanticAnalyzer) analyzeUnary(unary *front.UnaryExpr) front.Node {
	if unary.Op == "$" {
		sa.analyzeNode(unary.Expr)
		return unary
	}
	return unary
}

func (sa *SemanticAnalyzer) analyzeVarDecl(decl *front.VarDecl) front.Node {
	// Проверяем, что переменная не объявлена дважды
	if existing := sa.CurrentScope.ResolveLocal(decl.Name); existing != nil {
		sa.addError("1008", fmt.Sprintf("Variable '%s' already declared in this scope", decl.Name), 0, 0, "")
		return decl
	}

	// Проверяем тип
	if !sa.isValidType(decl.Type) {
		sa.addError("1009", fmt.Sprintf("Unknown type '%s'", decl.Type), 0, 0, "")
		return decl
	}

	// Анализируем выражение инициализации
	var exprType string
	if decl.Expr != nil {
		exprType = sa.getNodeType(decl.Expr)

		// Проверка соответствия типов
		if decl.Type == "any" {
			// any принимает любой тип
		} else if decl.Type != exprType && exprType != "" {
			sa.addError("1010", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s'", exprType, decl.Type), 0, 0, "")
			return decl
		}
	}

	// Регистрируем переменную
	sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
	return decl
}

func (sa *SemanticAnalyzer) analyzeAssign(assign *front.Assign) front.Node {
	debug.Debug("analyzeAssign: %s\n", assign.Name)

	// Проверяем, что переменная существует
	sym := sa.CurrentScope.Resolve(assign.Name)
	if sym == nil {
		debug.Debug("Variable %s not found\n", assign.Name)
		sa.addError("1011", fmt.Sprintf("Undefined variable '%s'", assign.Name), 0, 0, "")
		return assign
	}
	debug.Debug("Variable %s found, type=%s\n", assign.Name, sym.Type)

	// Проверяем, что переменная не константа
	if sym.IsConst {
		debug.Debug("Variable %s is const\n", assign.Name)
		sa.addError("1012", fmt.Sprintf("Cannot assign to constant '%s'", assign.Name), 0, 0, "")
		return assign
	}

	// Анализируем выражение
	if assign.Expr != nil {
		exprType := sa.getNodeType(assign.Expr)
		debug.Debug("Expression type: %s\n", exprType)

		// Проверка соответствия типов
		if sym.Type == "any" {
			debug.Debug("any type, accepting any value\n")
		} else if sym.Type != exprType && exprType != "" {
			debug.Debug("Type mismatch: %s vs %s\n", exprType, sym.Type)
			sa.addError("1013", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s' (variable '%s')",
				exprType, sym.Type, assign.Name), 0, 0, "")
			return assign
		} else {
			debug.Debug("Types match: %s == %s\n", exprType, sym.Type)
		}
	} else {
		debug.Debug("No expression to analyze\n")
	}

	debug.Debug("analyzeAssign completed successfully\n")
	return assign
}

func (sa *SemanticAnalyzer) analyzeBinary(bin *front.BinaryExpr) front.Node {
	left := sa.analyzeNode(bin.Left)
	right := sa.analyzeNode(bin.Right)

	leftType := sa.getNodeType(left)
	rightType := sa.getNodeType(right)

	// Проверка операций
	switch bin.Op {
	case "+", "-", "*", "/":
		// Арифметические операции допустимы для чисел
		if !sa.isNumericType(leftType) || !sa.isNumericType(rightType) {
			sa.addError("1014", fmt.Sprintf("Arithmetic operation '%s' requires numeric types (got %s and %s)",
				bin.Op, leftType, rightType), 0, 0, "")
		}
	case "<", ">":
		// Сравнения допустимы для чисел
		if !sa.isNumericType(leftType) || !sa.isNumericType(rightType) {
			sa.addError("1015", fmt.Sprintf("Comparison operation '%s' requires numeric types (got %s and %s)",
				bin.Op, leftType, rightType), 0, 0, "")
		}
	}

	return bin
}

func (sa *SemanticAnalyzer) analyzeNumber(num *front.Number) front.Node {
	// Проверяем, что это валидное число
	if _, err := strconv.Atoi(num.Value); err != nil {
		if _, err := strconv.ParseFloat(num.Value, 64); err != nil {
			sa.addError("1016", fmt.Sprintf("Invalid number '%s'", num.Value), 0, 0, "")
		}
	}
	return num
}

func (sa *SemanticAnalyzer) analyzeString(str *front.String) front.Node {
	return str
}

func (sa *SemanticAnalyzer) analyzeIdent(ident *front.Ident) front.Node {
	// Проверяем, что идентификатор существует
	sym := sa.CurrentScope.Resolve(ident.Name)
	if sym == nil {
		sa.addError("1017", fmt.Sprintf("Undefined identifier '%s'", ident.Name), 0, 0, "")
	}
	return ident
}

func (sa *SemanticAnalyzer) analyzeReturn(ret *front.ReturnStmt) front.Node {
	if ret.Expr != nil {
		exprType := sa.getNodeType(ret.Expr)

		// Проверяем соответствие типу возврата функции
		if sa.CurrentFunction.ReturnType == "void" {
			sa.addError("1018", "Cannot return value from void function", 0, 0, "")
			return ret
		}

		if sa.CurrentFunction.ReturnType != exprType && exprType != "" {
			sa.addError("1019", fmt.Sprintf("Return type mismatch: expected '%s', got '%s'",
				sa.CurrentFunction.ReturnType, exprType), 0, 0, "")
			return ret
		}
	} else {
		// return без значения
		if sa.CurrentFunction.ReturnType != "void" {
			sa.addError("1020", fmt.Sprintf("Expected return value of type '%s'", sa.CurrentFunction.ReturnType), 0, 0, "")
			return ret
		}
	}
	return ret
}

func (sa *SemanticAnalyzer) analyzeCall(call *front.CallExpr) front.Node {
	debug.Debug("analyzeCall: %s\n", call.Name)

	// Проверяем, что функция существует
	targetFunc := sa.resolveFunction(call.Name)
	if targetFunc == nil {
		debug.Debug("Function %s not found\n", call.Name)
		sa.addError("1021", fmt.Sprintf("Undefined function '%s'", call.Name), 0, 0, "")
		return call
	}
	debug.Debug("Function %s found, params=%d\n", call.Name, len(targetFunc.Params))

	// Проверяем количество аргументов
	if len(call.Args) != len(targetFunc.Params) {
		debug.Debug("Argument count mismatch: expected %d, got %d\n", len(targetFunc.Params), len(call.Args))
		sa.addError("1023", fmt.Sprintf("Function '%s' expects %d arguments, got %d",
			call.Name, len(targetFunc.Params), len(call.Args)), 0, 0, "")
		return call
	}

	// Проверяем типы аргументов
	for i, arg := range call.Args {
		argType := sa.getNodeType(arg)
		paramType := targetFunc.Params[i].Type

		debug.Debug("Arg %d: type=%s, expected=%s\n", i, argType, paramType)

		if argType != paramType && argType != "" && paramType != "any" {
			debug.Debug("Type mismatch in argument %d\n", i)
			sa.addError("1024", fmt.Sprintf("Argument %d type mismatch: expected '%s', got '%s'",
				i+1, paramType, argType), 0, 0, "")
			return call
		}
	}

	debug.Debug("analyzeCall completed successfully\n")
	return call
}

func (sa *SemanticAnalyzer) analyzeIf(ifStmt *front.IfStmt) front.Node {
	// Проверяем условие if
	condType := sa.getNodeType(ifStmt.Condition)
	if condType != "bool" && condType != "" {
		sa.addError("1025", fmt.Sprintf("If condition must be boolean, got '%s'", condType), 0, 0, "")
	}

	// Анализируем блок then
	if ifStmt.Then != nil {
		sa.analyzeBlock(ifStmt.Then, false)
	}

	// Анализируем все elsif
	for _, elsif := range ifStmt.Elsifs {
		condType = sa.getNodeType(elsif.Condition)
		if condType != "bool" && condType != "" {
			sa.addError("1028", fmt.Sprintf("Elsif condition must be boolean, got '%s'", condType), 0, 0, "")
		}
		if elsif.Then != nil {
			sa.analyzeBlock(elsif.Then, false)
		}
	}

	// Анализируем блок else
	if ifStmt.Else != nil {
		sa.analyzeBlock(ifStmt.Else, false)
	}

	return ifStmt
}

func (sa *SemanticAnalyzer) analyzeWhile(while *front.WhileStmt) front.Node {
	// Проверяем условие
	if while.Condition != nil {
		condType := sa.getNodeType(while.Condition)
		if condType != "bool" && condType != "" {
			sa.addError("1026", fmt.Sprintf("While condition must be boolean, got '%s'", condType), 0, 0, "")
		}
	}

	// Анализируем тело
	if while.Body != nil {
		sa.analyzeBlock(while.Body, false)
	}

	return while
}

func (sa *SemanticAnalyzer) analyzeFor(forStmt *front.ForStmt) front.Node {
	// Анализируем инициализацию
	if forStmt.Init != nil {
		sa.analyzeNode(forStmt.Init)
	}

	// Анализируем условие
	if forStmt.Cond != nil {
		condType := sa.getNodeType(forStmt.Cond)
		if condType != "bool" && condType != "" {
			sa.addError("1027", fmt.Sprintf("For condition must be boolean, got '%s'", condType), 0, 0, "")
		}
	}

	// Анализируем пост-выражение
	if forStmt.Post != nil {
		sa.analyzeNode(forStmt.Post)
	}

	// Анализируем тело
	if forStmt.Body != nil {
		sa.analyzeBlock(forStmt.Body, false)
	}

	return forStmt
}

// Вспомогательные методы
func (sa *SemanticAnalyzer) isValidType(typ string) bool {
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
		if strings.Contains(n.Value, ".") {
			return "float"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.Ident:
		sym := sa.CurrentScope.Resolve(n.Name)
		if sym != nil {
			return sym.Type
		}
		return ""
	case *front.BinaryExpr:
		debug.Debug("getNodeType BinaryExpr: op=%s\n", n.Op)
		leftType := sa.getNodeType(n.Left)
		rightType := sa.getNodeType(n.Right)
		debug.Debug("  leftType=%s, rightType=%s\n", leftType, rightType)

		// СРАВНЕНИЯ ВОЗВРАЩАЮТ BOOL!
		if n.Op == "<" || n.Op == ">" || n.Op == "==" || n.Op == "!=" || n.Op == "<=" || n.Op == ">=" {
			debug.Debug("  comparison operator, returning bool\n")
			return "bool"
		}

		// Для арифметических операций
		isLeftNumeric := leftType == "int" || leftType == "float" || leftType == "double"
		isRightNumeric := rightType == "int" || rightType == "float" || rightType == "double"

		if isLeftNumeric && isRightNumeric {
			if leftType == "float" || rightType == "float" {
				return "float"
			}
			if leftType == "double" || rightType == "double" {
				return "double"
			}
			return "int"
		}

		if n.Op == "+" && (leftType == "string" || rightType == "string") {
			return "string"
		}

		return "int"

	case *front.CallExpr:
		debug.Debug("getNodeType CallExpr: %s\n", n.Name)

		// Проверяем в текущем файле
		for _, fn := range sa.Program.Functions {
			if fn.Name == n.Name {
				debug.Debug("  found in current file: %s -> %s\n", fn.Name, fn.ReturnType)
				return fn.ReturnType
			}
		}

		// Проверяем в импортированных функциях (по полному имени)
		if fn, ok := sa.ImportedFuncs[n.Name]; ok {
			debug.Debug("  found in imports: %s -> %s\n", n.Name, fn.ReturnType)
			return fn.ReturnType
		}

		// Проверяем с префиксом модуля (io.to_string → to_string)
		if strings.Contains(n.Name, ".") {
			parts := strings.Split(n.Name, ".")
			simpleName := parts[len(parts)-1]
			debug.Debug("  checking simple name: %s\n", simpleName)

			// Ищем в текущем файле
			for _, fn := range sa.Program.Functions {
				if fn.Name == simpleName {
					debug.Debug("  found in current file (simple): %s -> %s\n", simpleName, fn.ReturnType)
					return fn.ReturnType
				}
			}

			// Ищем в импортированных
			if fn, ok := sa.ImportedFuncs[simpleName]; ok {
				debug.Debug("  found in imports (simple): %s -> %s\n", simpleName, fn.ReturnType)
				return fn.ReturnType
			}
		}

		debug.Debug("  function %s not found\n", n.Name)
		return ""

	case *front.UnaryExpr:
		if n.Op == "$" {
			return "string"
		}
		return sa.getNodeType(n.Expr)

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
	sa.Errors = append(sa.Errors, errors.SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
	})
}
