package midlevel

import (
	"fmt"
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
}

func NewSemanticAnalyzer(prog *front.Program) *SemanticAnalyzer {
	sa := &SemanticAnalyzer{
		Program:     prog,
		GlobalScope: NewScope(nil, true),
	}
	return sa
}

func (sa *SemanticAnalyzer) Analyze() bool {
	// Инициализация встроенных типов (как символы)
	sa.initBuiltinTypes()

	// 1. Регистрация всех функций в глобальной области
	for _, fn := range sa.Program.Functions {
		sa.registerFunction(fn)
	}

	// 2. Проверка наличия main()
	if !sa.checkMain() {
		return false
	}

	// 3. Анализ каждой функции
	for _, fn := range sa.Program.Functions {
		sa.CurrentFunction = fn
		sa.CurrentScope = NewScope(sa.GlobalScope, false)
		sa.analyzeFunction(fn)
	}

	return len(sa.Errors) == 0
}

func (sa *SemanticAnalyzer) initBuiltinTypes() {
	// Регистрируем базовые типы как символы-типы
	types := []string{"int", "string", "float", "double", "bool", "char", "arr", "dict", "void", "any"}
	for _, t := range types {
		sa.GlobalScope.Define("type_"+t, SYM_CONST, "type", true)
	}
}

func (sa *SemanticAnalyzer) registerFunction(fn *front.Function) {
	// Проверяем, что функция не объявлена дважды
	if existing := sa.GlobalScope.Resolve(fn.Name); existing != nil {
		sa.addError("1003", fmt.Sprintf("Function '%s' already declared", fn.Name), 0, 0, "")
		return
	}

	// Регистрируем функцию
	sa.GlobalScope.Define(fn.Name, SYM_FUNCTION, fn.ReturnType, fn.IsExport)
}

func (sa *SemanticAnalyzer) checkMain() bool {
	mainSym := sa.GlobalScope.Resolve("main")
	if mainSym == nil {
		sa.addError("1004", "No main() function found", 0, 0, "")
		return false
	}

	// Проверяем, что main имеет тип void
	if mainSym.Type != "void" {
		sa.addError("1005", "main() must return void", 0, 0, "")
		return false
	}

	// Ищем саму функцию, чтобы проверить параметры
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
	// Вход в новую область видимости
	sa.CurrentScope = NewScope(sa.GlobalScope, false)
	defer func() {
		sa.CurrentScope = sa.CurrentScope.Parent
	}()

	// Регистрируем параметры
	for _, param := range fn.Params {
		sa.CurrentScope.Define(param.Name, SYM_VARIABLE, param.Type, false)
	}

	// Анализируем тело функции
	if fn.Body != nil {
		sa.analyzeBlock(fn.Body, true)
	}
}

func (sa *SemanticAnalyzer) analyzeBlock(block *front.Block, isFunctionBody bool) {
	for _, stmt := range block.Statements {
		sa.analyzeNode(stmt)
	}
}

func (sa *SemanticAnalyzer) analyzeNode(node front.Node) front.Node {
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
		return n
	default:
		return n
	}
}

func (sa *SemanticAnalyzer) analyzeUnary(unary *front.UnaryExpr) front.Node {
	if unary.Op == "$" {
		// $ преобразует любой тип в строку
		sa.analyzeNode(unary.Expr)
		// Тип результата - string
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
	// Проверяем, что переменная существует
	sym := sa.CurrentScope.Resolve(assign.Name)
	if sym == nil {
		sa.addError("1011", fmt.Sprintf("Undefined variable '%s'", assign.Name), 0, 0, "")
		return assign
	}

	// Проверяем, что переменная не константа
	if sym.IsConst {
		sa.addError("1012", fmt.Sprintf("Cannot assign to constant '%s'", assign.Name), 0, 0, "")
		return assign
	}

	// Анализируем выражение
	if assign.Expr != nil {
		exprType := sa.getNodeType(assign.Expr)

		// Проверка соответствия типов
		if sym.Type == "any" {
			// any принимает любой тип
		} else if sym.Type != exprType && exprType != "" {
			sa.addError("1013", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s' (variable '%s')",
				exprType, sym.Type, assign.Name), 0, 0, "")
			return assign
		}
	}

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
	// Проверяем, что функция существует
	sym := sa.CurrentScope.Resolve(call.Name)
	if sym == nil {
		sa.addError("1021", fmt.Sprintf("Undefined function '%s'", call.Name), 0, 0, "")
		return call
	}

	if sym.Kind != SYM_FUNCTION {
		sa.addError("1022", fmt.Sprintf("'%s' is not a function", call.Name), 0, 0, "")
		return call
	}

	// Находим саму функцию для проверки параметров
	var targetFunc *front.Function
	for _, fn := range sa.Program.Functions {
		if fn.Name == call.Name {
			targetFunc = fn
			break
		}
	}

	if targetFunc != nil {
		// Проверяем количество аргументов
		if len(call.Args) != len(targetFunc.Params) {
			sa.addError("1023", fmt.Sprintf("Function '%s' expects %d arguments, got %d",
				call.Name, len(targetFunc.Params), len(call.Args)), 0, 0, "")
			return call
		}

		// Проверяем типы аргументов
		for i, arg := range call.Args {
			argType := sa.getNodeType(arg)
			paramType := targetFunc.Params[i].Type

			if argType != paramType && argType != "" && paramType != "any" {
				sa.addError("1024", fmt.Sprintf("Argument %d type mismatch: expected '%s', got '%s'",
					i+1, paramType, argType), 0, 0, "")
			}
		}
	}

	return call
}

func (sa *SemanticAnalyzer) analyzeIf(ifStmt *front.IfStmt) front.Node {
	// Анализируем условие
	condType := sa.getNodeType(ifStmt.Condition)
	if condType != "bool" && condType != "" {
		sa.addError("1025", fmt.Sprintf("If condition must be boolean, got '%s'", condType), 0, 0, "")
	}

	// Анализируем блок then
	if ifStmt.Then != nil {
		sa.analyzeBlock(ifStmt.Then, false)
	}

	// Анализируем блок else
	if ifStmt.Else != nil {
		sa.analyzeBlock(ifStmt.Else, false)
	}

	return ifStmt
}

func (sa *SemanticAnalyzer) analyzeWhile(while *front.WhileStmt) front.Node {
	// Анализируем условие
	condType := sa.getNodeType(while.Condition)
	if condType != "bool" && condType != "" {
		sa.addError("1026", fmt.Sprintf("While condition must be boolean, got '%s'", condType), 0, 0, "")
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
		leftType := sa.getNodeType(n.Left)
		rightType := sa.getNodeType(n.Right)
		// Для простоты возвращаем тип левой части
		if leftType != "" {
			return leftType
		}
		return rightType
	case *front.CallExpr:
		sym := sa.CurrentScope.Resolve(n.Name)
		if sym != nil {
			return sym.Type
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
	sa.Errors = append(sa.Errors, errors.SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
	})
}
