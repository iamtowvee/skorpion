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
	debug.Debug("resolveFunction: %s", name)

	for _, fn := range sa.Program.Functions {
		if fn.Name == name {
			debug.Debug("Found in current file: %s\n", name)
			return fn
		}
	}

	if fn, ok := sa.ImportedFuncs[name]; ok {
		debug.Debug("Found in imports: %s\n", name)
		return fn
	}

	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		simpleName := parts[len(parts)-1]
		debug.Debug("Trying simple name: %s\n", simpleName)

		for _, fn := range sa.Program.Functions {
			if fn.Name == simpleName {
				debug.Debug("Found in current file (simple): %s\n", simpleName)
				return fn
			}
		}

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
		"to_arr":    "arr",
	}
	for name, retType := range builtins {
		sa.GlobalScope.Define(name, SYM_FUNCTION, retType, true)
	}
}

func (sa *SemanticAnalyzer) registerFunction(fn *front.Function) {
	if existing := sa.GlobalScope.Resolve(fn.Name); existing != nil {
		sa.addError("1500", fmt.Sprintf("Function '%s' already declared", fn.Name),
			fn.GetLine(), fn.GetColumn(), fn.File)
		return
	}
	sa.GlobalScope.Define(fn.Name, SYM_FUNCTION, fn.ReturnType, fn.IsExport)
}

func (sa *SemanticAnalyzer) checkMain() bool {
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
					fn.GetLine(), fn.GetColumn(), fn.File)
				return false
			}
			if fn.Params[0].Type != "arr" {
				sa.addError("1504", "main() parameter must be of type 'arr'",
					fn.GetLine(), fn.GetColumn(), fn.File)
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
		sa.CurrentScope.Define(param.Name, SYM_VARIABLE, param.Type, false)
	}

	if fn.Body != nil {
		debug.Debug("Analyzing body of %s\n", fn.Name)
		sa.analyzeBlock(fn.Body, true)
		debug.Debug("Body of %s analyzed\n", fn.Name)

		// Проверка: non-void функция должна иметь return
		if fn.ReturnType != "void" && !sa.hasReturn(fn.Body) {
			sa.addError("1535",
				fmt.Sprintf("Function '%s' must return a value of type '%s'", fn.Name, fn.ReturnType),
				fn.GetLine(), fn.GetColumn(), sa.CurrentFile)
		}

		// Проверка: пустое тело функции
		if len(fn.Body.Statements) == 0 {
			errors.NewWarning("2003",
				fmt.Sprintf("Empty function body in '%s'", fn.Name),
				fn.GetLine(), fn.GetColumn(), sa.CurrentFile)
		}

		// Проверка: неиспользуемые параметры
		used := make(map[string]bool)
		sa.collectUsedIdents(fn.Body, used)
		for _, param := range fn.Params {
			if !used[param.Name] {
				errors.NewWarning("2001",
					fmt.Sprintf("Unused parameter '%s' in function '%s'", param.Name, fn.Name),
					param.GetLine(), param.GetColumn(), sa.CurrentFile)
			}
		}

		// Проверка: неиспользуемые переменные
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
	case *front.ArrayLiteral:
		for _, elem := range n.Elements {
			sa.analyzeNode(elem)
		}
		return n
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
	case *front.ForStmt:
		return sa.analyzeFor(n)
	case *front.RangeExpr:
		sa.analyzeNode(n.Start)
		sa.analyzeNode(n.End)
		return n
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
	if unary.Op == "!" {
		operandType := sa.getNodeType(unary.Expr)
		if operandType != "bool" && operandType != "" {
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
	if decl.IsArray {
		if decl.Expr != nil {
			if arrLit, ok := decl.Expr.(*front.ArrayLiteral); ok {
				expectedElemType := decl.ElemType

				if expectedElemType != "" && expectedElemType != "any" {
					for _, elem := range arrLit.Elements {
						if isArrayTypeSemantic(expectedElemType) {
							if _, ok := elem.(*front.ArrayLiteral); !ok {
								sa.addError("1534",
									fmt.Sprintf("Array element type mismatch: expected '%s', got '%s'",
										expectedElemType, sa.getNodeType(elem)),
									elem.GetLine(), elem.GetColumn(), sa.CurrentFile)
							}
						} else {
							elemType := sa.getNodeType(elem)
							if elemType != expectedElemType && elemType != "" {
								sa.addError("1534",
									fmt.Sprintf("Array element type mismatch: expected '%s', got '%s'",
										expectedElemType, elemType),
									elem.GetLine(), elem.GetColumn(), sa.CurrentFile)
							}
						}
					}
				}

				for _, elem := range arrLit.Elements {
					sa.analyzeNode(elem)
				}
			}
		}
		sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, "arr", false)
		return decl
	}

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

		if decl.Type == "any" {
			// any принимает любой тип
		} else if decl.Type != exprType && exprType != "" {
			sa.addError("1507", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s'", exprType, decl.Type),
				decl.GetLine(), decl.GetColumn(), sa.CurrentFile)
			return decl
		}
	}

	sa.CurrentScope.Define(decl.Name, SYM_VARIABLE, decl.Type, false)
	return decl
}

func (sa *SemanticAnalyzer) analyzeAssign(assign *front.Assign) front.Node {
	debug.Debug("analyzeAssign: %s\n", assign.Name)

	sym := sa.CurrentScope.Resolve(assign.Name)
	if sym == nil {
		debug.Debug("Variable %s not found\n", assign.Name)
		sa.addError("1508", fmt.Sprintf("Undefined variable '%s'", assign.Name),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return assign
	}
	debug.Debug("Variable %s found, type=%s\n", assign.Name, sym.Type)

	if sym.IsConst {
		debug.Debug("Variable %s is const\n", assign.Name)
		sa.addError("1509", fmt.Sprintf("Cannot assign to constant '%s'", assign.Name),
			assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
		return assign
	}

	if assign.Expr != nil {
		exprType := sa.getNodeType(assign.Expr)
		debug.Debug("Expression type: %s\n", exprType)

		if sym.Type == "any" {
			debug.Debug("any type, accepting any value\n")
		} else if sym.Type != exprType && exprType != "" {
			debug.Debug("Type mismatch: %s vs %s\n", exprType, sym.Type)
			sa.addError("1510", fmt.Sprintf("Type mismatch: cannot assign '%s' to '%s' (variable '%s')",
				exprType, sym.Type, assign.Name),
				assign.GetLine(), assign.GetColumn(), sa.CurrentFile)
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

func (sa *SemanticAnalyzer) analyzeTypeOf(typeOf *front.TypeOf) front.Node {
	sa.analyzeNode(typeOf.Expr)
	return typeOf
}

func (sa *SemanticAnalyzer) analyzeBinary(bin *front.BinaryExpr) front.Node {
	left := sa.analyzeNode(bin.Left)
	right := sa.analyzeNode(bin.Right)

	leftType := sa.getNodeType(left)
	rightType := sa.getNodeType(right)

	// Деление на ноль-литерал
	if bin.Op == "/" {
		if num, ok := bin.Right.(*front.Number); ok {
			if num.Value == "0" || num.Value == "0.0" {
				errors.NewWarning("1541", "Division by zero",
					bin.GetLine(), bin.GetColumn(), sa.CurrentFile)
			}
		}
	}

	switch bin.Op {
	case "+", "-", "*", "/":
		if !sa.isNumericType(leftType) || !sa.isNumericType(rightType) {
			sa.addError("1511", fmt.Sprintf("Arithmetic operation '%s' requires numeric types (got %s and %s)",
				bin.Op, leftType, rightType),
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

func (sa *SemanticAnalyzer) analyzeNumber(num *front.Number) front.Node {
	if _, err := strconv.Atoi(num.Value); err != nil {
		if _, err := strconv.ParseFloat(num.Value, 64); err != nil {
			sa.addError("1513", fmt.Sprintf("Invalid number '%s'", num.Value),
				num.GetLine(), num.GetColumn(), sa.CurrentFile)
		}
	}
	return num
}

func (sa *SemanticAnalyzer) analyzeString(str *front.String) front.Node {
	return str
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
	if ret.Expr != nil {
		exprType := sa.getNodeType(ret.Expr)

		if sa.CurrentFunction.ReturnType == "void" {
			sa.addError("1515", "Cannot return value from void function",
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
			return ret
		}

		if sa.CurrentFunction.ReturnType != exprType && exprType != "" {
			sa.addError("1516", fmt.Sprintf("Return type mismatch: expected '%s', got '%s'",
				sa.CurrentFunction.ReturnType, exprType),
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
			return ret
		}
	} else {
		if sa.CurrentFunction.ReturnType != "void" {
			sa.addError("1517", fmt.Sprintf("Expected return value of type '%s'", sa.CurrentFunction.ReturnType),
				ret.GetLine(), ret.GetColumn(), sa.CurrentFile)
			return ret
		}
	}
	return ret
}

func (sa *SemanticAnalyzer) analyzeCall(call *front.CallExpr) front.Node {
	debug.Debug("analyzeCall: %s\n", call.Name)

	builtinFuncs := map[string]bool{
		"to_int":    true,
		"to_float":  true,
		"to_double": true,
		"to_string": true,
		"to_bool":   true,
		"to_arr":    true,
	}
	if builtinFuncs[call.Name] {
		for _, arg := range call.Args {
			sa.analyzeNode(arg)
		}
		return call
	}

	// Проверяем, не является ли имя переменной (не функцией)
	sym := sa.CurrentScope.Resolve(call.Name)
	if sym != nil && sym.Kind != SYM_FUNCTION {
		sa.addError("1539",
			fmt.Sprintf("'%s' is not a function", call.Name),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}

	targetFunc := sa.resolveFunction(call.Name)
	if targetFunc == nil {
		debug.Debug("Function %s not found\n", call.Name)
		sa.addError("1518", fmt.Sprintf("Undefined function '%s'", call.Name),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}
	debug.Debug("Function %s found, params=%d\n", call.Name, len(targetFunc.Params))

	if len(call.Args) != len(targetFunc.Params) {
		debug.Debug("Argument count mismatch: expected %d, got %d\n", len(targetFunc.Params), len(call.Args))
		sa.addError("1519", fmt.Sprintf("Function '%s' expects %d arguments, got %d",
			call.Name, len(targetFunc.Params), len(call.Args)),
			call.GetLine(), call.GetColumn(), sa.CurrentFile)
		return call
	}

	for i, arg := range call.Args {
		argType := sa.getNodeType(arg)
		paramType := targetFunc.Params[i].Type

		debug.Debug("Arg %d: type=%s, expected=%s\n", i, argType, paramType)

		if argType != paramType && argType != "" && paramType != "any" {
			debug.Debug("Type mismatch in argument %d\n", i)
			sa.addError("1520", fmt.Sprintf("Argument %d type mismatch: expected '%s', got '%s'",
				i+1, paramType, argType),
				arg.GetLine(), arg.GetColumn(), sa.CurrentFile)
			return call
		}
	}

	debug.Debug("analyzeCall completed successfully\n")
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

	return ifStmt
}

func (sa *SemanticAnalyzer) analyzeCase(caseStmt *front.CaseStmt) front.Node {
	debug.Debug("analyzeCase: checking value\n")

	valueType := sa.getNodeType(caseStmt.Value)
	debug.Debug("Case value type: %s\n", valueType)

	seenPatterns := make(map[string]bool)
	for _, branch := range caseStmt.Branches {
		patternType := sa.getNodeType(branch.Pattern)
		debug.Debug("Pattern type: %s\n", patternType)

		if patternType != valueType && patternType != "" {
			sa.addError("1525",
				fmt.Sprintf("Pattern type mismatch: expected '%s', got '%s'",
					valueType, patternType),
				branch.GetLine(), branch.GetColumn(), sa.CurrentFile)
		}

		// Проверка дубликатов паттернов
		patKey := sa.getConstantValue(branch.Pattern)
		if patKey != "" {
			if seenPatterns[patKey] {
				sa.addError("1540",
					fmt.Sprintf("Duplicate case pattern '%s'", patKey),
					branch.GetLine(), branch.GetColumn(), sa.CurrentFile)
			}
			seenPatterns[patKey] = true
		}

		if branch.Body != nil {
			sa.analyzeBlock(branch.Body, false)
		}
	}

	if caseStmt.Default != nil {
		sa.analyzeBlock(caseStmt.Default, false)
	}

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

	return while
}

func (sa *SemanticAnalyzer) analyzeFor(forStmt *front.ForStmt) front.Node {
	if forStmt.Init != nil {
		sa.analyzeNode(forStmt.Init)
	}

	if forStmt.Cond != nil {
		condType := sa.getNodeType(forStmt.Cond)
		if condType != "bool" && condType != "" {
			sa.addError("1523", fmt.Sprintf("For condition must be boolean, got '%s'", condType),
				forStmt.GetLine(), forStmt.GetColumn(), sa.CurrentFile)
		}
	}

	if forStmt.Post != nil {
		sa.analyzeNode(forStmt.Post)
	}

	if forStmt.Body != nil {
		sa.analyzeBlock(forStmt.Body, false)
	}

	return forStmt
}

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
	case *front.ArrayLiteral:
		return "arr"
	case *front.TypeOf:
		return "string"
	case *front.Ident:
		if n.Name == "true" || n.Name == "false" {
			return "bool"
		}
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

		if n.Op == "&&" || n.Op == "||" {
			return "bool"
		}

		if n.Op == "<" || n.Op == ">" || n.Op == "==" || n.Op == "!=" || n.Op == "<=" || n.Op == ">=" {
			return "bool"
		}

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

		for _, fn := range sa.Program.Functions {
			if fn.Name == n.Name {
				debug.Debug("  found in current file: %s -> %s\n", fn.Name, fn.ReturnType)
				return fn.ReturnType
			}
		}

		if fn, ok := sa.ImportedFuncs[n.Name]; ok {
			debug.Debug("  found in imports: %s -> %s\n", n.Name, fn.ReturnType)
			return fn.ReturnType
		}

		if strings.Contains(n.Name, ".") {
			parts := strings.Split(n.Name, ".")
			simpleName := parts[len(parts)-1]
			debug.Debug("  checking simple name: %s\n", simpleName)

			for _, fn := range sa.Program.Functions {
				if fn.Name == simpleName {
					debug.Debug("  found in current file (simple): %s -> %s\n", simpleName, fn.ReturnType)
					return fn.ReturnType
				}
			}

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

	case *front.RangeExpr:
		return "arr"
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
	case *front.Block:
		return sa.hasReturn(n)
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
	case *front.ForStmt:
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
			sa.collectUsedIdents(stmt, used)
		}
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
		sa.collectUsedIdents(n.Left, used)
		sa.collectUsedIdents(n.Right, used)
	case *front.UnaryExpr:
		sa.collectUsedIdents(n.Expr, used)
	case *front.CallExpr:
		for _, arg := range n.Args {
			sa.collectUsedIdents(arg, used)
		}
	case *front.CallRangeExpr:
		sa.collectUsedIdents(n.Range.Start, used)
		sa.collectUsedIdents(n.Range.End, used)
		for _, arg := range n.Extra {
			sa.collectUsedIdents(arg, used)
		}
	case *front.ReturnStmt:
		if n.Expr != nil {
			sa.collectUsedIdents(n.Expr, used)
		}
	case *front.IfStmt:
		sa.collectUsedIdents(n.Condition, used)
		sa.collectUsedIdents(n.Then, used)
		for _, elsif := range n.Elsifs {
			sa.collectUsedIdents(elsif.Condition, used)
			sa.collectUsedIdents(elsif.Then, used)
		}
		sa.collectUsedIdents(n.Else, used)
	case *front.WhileStmt:
		sa.collectUsedIdents(n.Condition, used)
		sa.collectUsedIdents(n.Body, used)
	case *front.ForStmt:
		sa.collectUsedIdents(n.Init, used)
		sa.collectUsedIdents(n.Cond, used)
		sa.collectUsedIdents(n.Post, used)
		sa.collectUsedIdents(n.Body, used)
	case *front.CaseStmt:
		sa.collectUsedIdents(n.Value, used)
		for _, branch := range n.Branches {
			sa.collectUsedIdents(branch.Pattern, used)
			sa.collectUsedIdents(branch.Body, used)
		}
		sa.collectUsedIdents(n.Default, used)
	case *front.TypeOf:
		sa.collectUsedIdents(n.Expr, used)
	case *front.ArrayLiteral:
		for _, elem := range n.Elements {
			sa.collectUsedIdents(elem, used)
		}
	case *front.ArrayIndex:
		used[n.Name] = true
		sa.collectUsedIdents(n.Index, used)
	case *front.ArrayLength:
		used[n.Name] = true
	case *front.ArrayAdd:
		used[n.Name] = true
		sa.collectUsedIdents(n.Elem, used)
	case *front.TernaryExpr:
		sa.collectUsedIdents(n.Condition, used)
		sa.collectUsedIdents(n.Then, used)
		sa.collectUsedIdents(n.Else, used)
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
		case *front.ForStmt:
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
