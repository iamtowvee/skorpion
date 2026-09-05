package backend

import (
	"fmt"
	"skrp/res/front"
	"strings"
)

type Pipeline struct {
	Program      *front.Program
	IR           *IRProgram
	TempCounter  int
	LabelCounter int
}

func NewPipeline(prog *front.Program) *Pipeline {
	return &Pipeline{
		Program:      prog,
		IR:           &IRProgram{Functions: []IRFunction{}, Globals: []IRGlobal{}, Imports: []IRImport{}},
		TempCounter:  0,
		LabelCounter: 0,
	}
}

func (p *Pipeline) Process() *IRProgram {
	// 1. Обрабатываем импорты
	for _, imp := range p.Program.Imports {
		p.IR.Imports = append(p.IR.Imports, IRImport{
			Path:  imp.Path,
			Alias: imp.Alias,
			All:   imp.All,
		})
	}

	// 2. Обрабатываем ВСЕ функции
	processed := make(map[string]bool)
	for _, fn := range p.Program.Functions {
		if processed[fn.Name] {
			continue
		}
		processed[fn.Name] = true
		p.processFunction(fn)
	}

	// 3. ЗАПУСКАЕМ GC!
	gc := NewGCAnalyzer(p.Program)
	gc.Analyze(p.IR)

	return p.IR
}

func (p *Pipeline) processFunction(fn *front.Function) {
	irFn := IRFunction{
		Name:         fn.Name,
		ReturnType:   fn.ReturnType,
		IsExport:     fn.IsExport,
		Params:       []IRParam{},
		Locals:       []string{},
		Instructions: []IRInstruction{},
	}

	// Параметры
	for _, param := range fn.Params {
		irFn.Params = append(irFn.Params, IRParam{
			Name: param.Name,
			Type: param.Type,
		})
	}

	// Тело функции
	if fn.Body != nil {
		p.processBlock(fn.Body, &irFn)
	}

	// Добавляем неявный return для void функций
	if fn.ReturnType == "void" && len(irFn.Instructions) > 0 {
		lastIns := irFn.Instructions[len(irFn.Instructions)-1]
		if lastIns.Op != "ret" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: "ret",
			})
		}
	}

	p.IR.Functions = append(p.IR.Functions, irFn)
}

func (p *Pipeline) processBlock(block *front.Block, irFn *IRFunction) {
	for _, stmt := range block.Statements {
		p.processNode(stmt, irFn)
	}
}

func (p *Pipeline) processNode(node front.Node, irFn *IRFunction) {
	switch n := node.(type) {
	case *front.VarDecl:
		p.processVarDecl(n, irFn)
	case *front.Assign:
		p.processAssign(n, irFn)
	case *front.BinaryExpr:
		p.processBinary(n, irFn)
	case *front.UnaryExpr:
		p.processUnary(n, irFn)
	case *front.Number:
	case *front.String:
	case *front.Ident:
	case *front.ReturnStmt:
		p.processReturn(n, irFn)
	case *front.CallExpr:
		p.processCall(n, irFn)
	case *front.IfStmt:
		p.processIf(n, irFn)
	case *front.WhileStmt:
		p.processWhile(n, irFn)
	case *front.ForStmt:
		p.processFor(n, irFn)
	case *front.IncludeC:
		// Вставляем C код как инструкцию в функцию
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "inline_c",
			Arg1: n.Code,
		})
	}
}

func (p *Pipeline) processUnary(unary *front.UnaryExpr, irFn *IRFunction) string {
	if unary.Op == "$" {
		// Преобразование в строку
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "to_string",
			Result: result,
			Arg1:   expr,
		})
		return result
	}
	return "0"
}

func (p *Pipeline) processVarDecl(decl *front.VarDecl, irFn *IRFunction) {
	// Определяем тип переменной для C
	cType := p.typeToC(decl.Type)

	// Регистрируем локальную переменную с правильным типом
	irFn.Locals = append(irFn.Locals, cType+" "+decl.Name)

	// Если есть инициализация
	if decl.Expr != nil {
		exprResult := p.processExpression(decl.Expr, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: decl.Name,
			Arg1:   exprResult,
		})
	}
}

func (p *Pipeline) typeToC(typ string) string {
	switch typ {
	case "int":
		return "int"
	case "string":
		return "sk_string"
	case "float":
		return "float"
	case "double":
		return "double"
	case "bool":
		return "sk_bool"
	case "char":
		return "char"
	case "void":
		return "void"
	case "arr":
		return "void*"
	case "dict":
		return "void*"
	case "any":
		return "void*"
	default:
		return "int"
	}
}

func (p *Pipeline) processAssign(assign *front.Assign, irFn *IRFunction) {
	if assign.Expr != nil {
		exprResult := p.processExpression(assign.Expr, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: assign.Name,
			Arg1:   exprResult,
		})
	}
}

func (p *Pipeline) processBinary(bin *front.BinaryExpr, irFn *IRFunction) string {
	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	// Проверяем конкатенацию строк
	leftIsString := p.isStringValue(left, irFn)
	rightIsString := p.isStringValue(right, irFn)

	if bin.Op == "+" && (leftIsString || rightIsString) {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "strcat",
			Result: result,
			Arg1:   left,
			Arg2:   right,
		})
		return result
	}

	// Для сравнений возвращаем выражение целиком, а не temp
	if bin.Op == "<" || bin.Op == ">" || bin.Op == "==" || bin.Op == "!=" || bin.Op == "<=" || bin.Op == ">=" {
		// Возвращаем выражение как строку для использования в if
		return fmt.Sprintf("(%s %s %s)", left, bin.Op, right)
	}

	// Арифметика
	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     bin.Op,
		Result: result,
		Arg1:   left,
		Arg2:   right,
	})

	return result
}

// isStringValue проверяет, является ли значение строкой
func (p *Pipeline) isStringValue(value string, irFn *IRFunction) bool {
	// Если это строковая константа
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return true
	}

	// Если это переменная, проверяем её тип
	for _, local := range irFn.Locals {
		if strings.Contains(local, value) {
			// Если объявлена как sk_string
			if strings.Contains(local, "sk_string") {
				return true
			}
		}
	}

	// Проверяем инструкции, которые создают строки
	for _, ins := range irFn.Instructions {
		if ins.Result == value {
			switch ins.Op {
			case "to_string", "strcat":
				return true
			case "=":
				// Если присваивается строковая константа
				if strings.HasPrefix(ins.Arg1, "\"") && strings.HasSuffix(ins.Arg1, "\"") {
					return true
				}
			}
		}
	}

	return false
}

func (p *Pipeline) processExpression(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		return n.Value
	case *front.String:
		return fmt.Sprintf(`"%s"`, n.Value)
	case *front.Ident:
		return n.Name
	case *front.BinaryExpr:
		return p.processBinary(n, irFn)
	case *front.CallExpr:
		// Для вызова функции как выражения (с возвращаемым значением)
		return p.processCallExpr(n, irFn)
	case *front.UnaryExpr:
		if n.Op == "$" {
			return p.processUnary(n, irFn)
		}
		return "0"
	default:
		return "0"
	}
}

func (p *Pipeline) processReturn(ret *front.ReturnStmt, irFn *IRFunction) {
	if ret.Expr != nil {
		exprResult := p.processExpression(ret.Expr, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "ret",
			Arg1: exprResult,
		})
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: "ret",
		})
	}
}

func (p *Pipeline) processCall(call *front.CallExpr, irFn *IRFunction) {
	// Находим целевую функцию
	var targetFunc *front.Function
	for _, fn := range p.Program.Functions {
		if fn.Name == call.Name {
			targetFunc = fn
			break
		}
	}

	// Если функция не найдена, пробуем убрать префикс модуля
	if targetFunc == nil && strings.Contains(call.Name, ".") {
		parts := strings.Split(call.Name, ".")
		simpleName := parts[len(parts)-1]
		for _, fn := range p.Program.Functions {
			if fn.Name == simpleName {
				targetFunc = fn
				break
			}
		}
	}

	// Подготавливаем аргументы с учётом значений по умолчанию
	args := []string{}
	argIndex := 0

	if targetFunc != nil {
		for _, param := range targetFunc.Params {
			var argExpr front.Node

			// Если есть переданный аргумент
			if argIndex < len(call.Args) {
				argExpr = call.Args[argIndex]
				argIndex++
			} else if param.DefaultValue != nil {
				// Используем значение по умолчанию
				argExpr = param.DefaultValue
			} else {
				// Ошибка: нет аргумента и нет значения по умолчанию
				args = append(args, "0") // fallback
				continue
			}

			args = append(args, p.processExpression(argExpr, irFn))
		}
	} else {
		// Если функция не найдена, просто передаём все аргументы как есть
		for _, arg := range call.Args {
			args = append(args, p.processExpression(arg, irFn))
		}
	}

	argsStr := strings.Join(args, ", ")

	// Убираем префикс модуля
	funcName := call.Name
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		funcName = parts[len(parts)-1]
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   "call",
		Arg1: funcName,
		Arg2: argsStr,
	})
}

func (p *Pipeline) processCallExpr(call *front.CallExpr, irFn *IRFunction) string {
	args := []string{}
	for _, arg := range call.Args {
		args = append(args, p.processExpression(arg, irFn))
	}

	argsStr := ""
	if len(args) > 0 {
		argsStr = args[0]
		for i := 1; i < len(args); i++ {
			argsStr += ", " + args[i]
		}
	}

	// Находим функцию и её тип возврата
	returnType := "sk_string" // по умолчанию
	funcName := call.Name

	// Убираем префикс модуля для поиска
	simpleName := funcName
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		simpleName = parts[len(parts)-1]
	}

	// Ищем функцию в программе
	for _, fn := range p.Program.Functions {
		if fn.Name == simpleName || fn.Name == funcName {
			switch fn.ReturnType {
			case "int":
				returnType = "int"
			case "string":
				returnType = "sk_string"
			case "float":
				returnType = "float"
			case "double":
				returnType = "double"
			case "bool":
				returnType = "sk_bool"
			case "void":
				returnType = "void"
			default:
				returnType = "sk_string"
			}
			break
		}
	}

	// Убираем префикс модуля для вызова
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		funcName = parts[len(parts)-1]
	}

	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       funcName,
		Arg2:       argsStr,
		ReturnType: returnType,
	})

	return result
}

func (p *Pipeline) processIf(ifStmt *front.IfStmt, irFn *IRFunction) {
	// Генерируем условие
	condResult := p.processExpression(ifStmt.Condition, irFn)

	ifLabel := p.newLabel()
	nextLabel := p.newLabel()

	// Проверяем условие
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "if",
		Result: condResult, // Это уже выражение вида (i < 5)
		Arg1:   ifLabel,
		Arg2:   nextLabel,
	})

	// Блок then
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: ifLabel,
	})
	if ifStmt.Then != nil {
		p.processBlock(ifStmt.Then, irFn)
	}

	endLabel := p.newLabel()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "goto",
		Result: endLabel,
	})

	// Обрабатываем elsif
	currentLabel := nextLabel
	for _, elsif := range ifStmt.Elsifs {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: currentLabel,
		})

		condResult = p.processExpression(elsif.Condition, irFn)
		elsifLabel := p.newLabel()
		nextLabel = p.newLabel()

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "if",
			Result: condResult,
			Arg1:   elsifLabel,
			Arg2:   nextLabel,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: elsifLabel,
		})
		if elsif.Then != nil {
			p.processBlock(elsif.Then, irFn)
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "goto",
			Result: endLabel,
		})

		currentLabel = nextLabel
	}

	// Обрабатываем else
	if ifStmt.Else != nil {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: currentLabel,
		})
		p.processBlock(ifStmt.Else, irFn)
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: currentLabel,
		})
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: endLabel,
	})
}

func (p *Pipeline) processWhile(while *front.WhileStmt, irFn *IRFunction) {
	startLabel := p.newLabel()
	bodyLabel := p.newLabel()
	endLabel := p.newLabel()

	// start:
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: startLabel,
	})

	// if !cond goto end
	condResult := p.processExpression(while.Condition, irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "if",
		Result: condResult,
		Arg1:   bodyLabel,
		Arg2:   endLabel,
	})

	// body:
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: bodyLabel,
	})

	if while.Body != nil {
		p.processBlock(while.Body, irFn)
	}

	// goto start
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "goto",
		Result: startLabel,
	})

	// end:
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: endLabel,
	})
}

func (p *Pipeline) processFor(forStmt *front.ForStmt, irFn *IRFunction) {
	startLabel := p.newLabel()
	bodyLabel := p.newLabel()
	endLabel := p.newLabel()

	if forStmt.Init != nil {
		p.processNode(forStmt.Init, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: startLabel,
	})

	if forStmt.Cond != nil {
		condResult := p.processExpression(forStmt.Cond, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "if",
			Result: condResult,
			Arg1:   bodyLabel,
			Arg2:   endLabel,
		})
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "goto",
			Result: bodyLabel,
		})
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: bodyLabel,
	})

	if forStmt.Body != nil {
		p.processBlock(forStmt.Body, irFn)
	}

	if forStmt.Post != nil {
		p.processNode(forStmt.Post, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "goto",
		Result: startLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: endLabel,
	})
}

// isStringTemp проверяет, является ли временная переменная строкой
func (p *Pipeline) isStringTemp(name string, irFn *IRFunction) bool {
	// Проверяем, не является ли это строковой константой
	if strings.HasPrefix(name, "\"") && strings.HasSuffix(name, "\"") {
		return true
	}

	// Проверяем инструкции, которые создают строки
	for _, ins := range irFn.Instructions {
		if ins.Result == name {
			switch ins.Op {
			case "to_string", "strcat":
				return true
			case "=":
				// Если присваивается строковая константа
				if strings.HasPrefix(ins.Arg1, "\"") && strings.HasSuffix(ins.Arg1, "\"") {
					return true
				}
			}
		}
	}
	return false
}

func (p *Pipeline) newTemp() string {
	p.TempCounter++
	return fmt.Sprintf("t%d", p.TempCounter)
}

func (p *Pipeline) newLabel() string {
	p.LabelCounter++
	return fmt.Sprintf("L%d", p.LabelCounter)
}
