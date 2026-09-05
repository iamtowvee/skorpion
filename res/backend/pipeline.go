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
	for _, imp := range p.Program.Imports {
		p.IR.Imports = append(p.IR.Imports, IRImport{
			Path:  imp.Path,
			Alias: imp.Alias,
			All:   imp.All,
		})
	}

	processed := make(map[string]bool)
	for _, fn := range p.Program.Functions {
		if processed[fn.Name] {
			continue
		}
		processed[fn.Name] = true
		p.processFunction(fn)
	}

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

	for _, param := range fn.Params {
		irFn.Params = append(irFn.Params, IRParam{
			Name: param.Name,
			Type: param.Type,
		})
	}

	if fn.Body != nil {
		p.processBlock(fn.Body, &irFn)
	}

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
	case *front.CaseStmt:
		p.processCase(n, irFn)
	case *front.WhileStmt:
		p.processWhile(n, irFn)
	case *front.ForStmt:
		p.processFor(n, irFn)
	case *front.IncludeC:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "inline_c",
			Arg1: n.Code,
		})
	}
}

func (p *Pipeline) processUnary(unary *front.UnaryExpr, irFn *IRFunction) string {
	if unary.Op == "$" {
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()

		// Проверяем, является ли выражение any
		if p.isAnyValue(expr, irFn) {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "any_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "to_string",
				Result: result,
				Arg1:   expr,
			})
		}
		return result
	}
	return "0"
}

func (p *Pipeline) processVarDecl(decl *front.VarDecl, irFn *IRFunction) {
	cType := p.typeToC(decl.Type)
	irFn.Locals = append(irFn.Locals, cType+" "+decl.Name)

	if decl.Expr != nil {
		exprResult := p.processExpression(decl.Expr, irFn)

		// Если переменная типа ANY и инициализируется значением, оборачиваем
		if decl.Type == "any" {
			tempVar := p.newTemp()
			wrapperFunc := p.getAnyWrapper(exprResult, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempVar,
				Arg1:       wrapperFunc,
				Arg2:       exprResult,
				ReturnType: "sk_any",
			})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   tempVar,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   exprResult,
			})
		}
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
		return "sk_any"
	default:
		return "int"
	}
}

func (p *Pipeline) processAssign(assign *front.Assign, irFn *IRFunction) {
	if assign.Expr != nil {
		exprResult := p.processExpression(assign.Expr, irFn)

		// Проверяем тип переменной
		var varType string
		for _, local := range irFn.Locals {
			if strings.Contains(local, assign.Name) {
				if strings.Contains(local, "sk_any") {
					varType = "any"
				} else if strings.Contains(local, "sk_string") {
					varType = "string"
				} else if strings.Contains(local, "int") {
					varType = "int"
				}
				break
			}
		}

		// Если переменная ANY, а присваиваем конкретное значение
		if varType == "any" {
			tempVar := p.newTemp()
			wrapperFunc := p.getAnyWrapper(exprResult, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempVar,
				Arg1:       wrapperFunc,
				Arg2:       exprResult,
				ReturnType: "sk_any",
			})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: assign.Name,
				Arg1:   tempVar,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: assign.Name,
				Arg1:   exprResult,
			})
		}
	}
}

func (p *Pipeline) processBinary(bin *front.BinaryExpr, irFn *IRFunction) string {
	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	// Определяем типы операндов
	leftIsString := p.isStringValue(left, irFn) || p.isAnyStringValue(left, irFn)
	rightIsString := p.isStringValue(right, irFn) || p.isAnyStringValue(right, irFn)

	// Конкатенация строк
	if bin.Op == "+" && (leftIsString || rightIsString) {
		// Если левый операнд any, распаковываем
		leftVal := left
		if p.isAnyValue(left, irFn) {
			tempVar := p.newTemp()
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempVar,
				Arg1:       "any_get_string",
				Arg2:       left,
				ReturnType: "sk_string",
			})
			leftVal = tempVar
		}
		// Если правый операнд any, распаковываем
		rightVal := right
		if p.isAnyValue(right, irFn) {
			tempVar := p.newTemp()
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempVar,
				Arg1:       "any_get_string",
				Arg2:       right,
				ReturnType: "sk_string",
			})
			rightVal = tempVar
		}
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "strcat",
			Result: result,
			Arg1:   leftVal,
			Arg2:   rightVal,
		})
		return result
	}

	// Арифметика — проверяем, не any ли операнды
	leftVal := left
	if p.isAnyValue(left, irFn) {
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       "any_get_int",
			Arg2:       left,
			ReturnType: "int",
		})
		leftVal = tempVar
	}
	rightVal := right
	if p.isAnyValue(right, irFn) {
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       "any_get_int",
			Arg2:       right,
			ReturnType: "int",
		})
		rightVal = tempVar
	}

	// Сравнения
	if bin.Op == "<" || bin.Op == ">" || bin.Op == "==" || bin.Op == "!=" || bin.Op == "<=" || bin.Op == ">=" {
		return fmt.Sprintf("(%s %s %s)", leftVal, bin.Op, rightVal)
	}

	// Арифметика
	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     bin.Op,
		Result: result,
		Arg1:   leftVal,
		Arg2:   rightVal,
	})

	return result
}

func (p *Pipeline) isStringValue(value string, irFn *IRFunction) bool {
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return true
	}

	for _, local := range irFn.Locals {
		if strings.Contains(local, value) && strings.Contains(local, "sk_string") {
			return true
		}
	}

	for _, ins := range irFn.Instructions {
		if ins.Result == value {
			switch ins.Op {
			case "to_string", "strcat":
				return true
			case "=":
				if strings.HasPrefix(ins.Arg1, "\"") && strings.HasSuffix(ins.Arg1, "\"") {
					return true
				}
			}
		}
	}

	return false
}

func (p *Pipeline) isAnyStringValue(value string, irFn *IRFunction) bool {
	// Проверяем, что это any и его реальный тип - строка
	// Пока просто проверяем через isAnyValue
	return p.isAnyValue(value, irFn)
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
	var targetFunc *front.Function
	for _, fn := range p.Program.Functions {
		if fn.Name == call.Name {
			targetFunc = fn
			break
		}
	}

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

	args := []string{}
	argIndex := 0

	if targetFunc != nil {
		for _, param := range targetFunc.Params {
			var argExpr front.Node

			if argIndex < len(call.Args) {
				argExpr = call.Args[argIndex]
				argIndex++
			} else if param.DefaultValue != nil {
				argExpr = param.DefaultValue
			} else {
				args = append(args, "0")
				continue
			}

			argValue := p.processExpression(argExpr, irFn)

			// Если параметр ANY — оборачиваем
			if param.Type == "any" {
				tempVar := p.newTemp()
				wrapperFunc := p.getAnyWrapper(argValue, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tempVar,
					Arg1:       wrapperFunc,
					Arg2:       argValue,
					ReturnType: "sk_any",
				})
				args = append(args, tempVar)
				continue
			}

			// Если передаём ANY в функцию с конкретным типом — распаковываем
			if p.isAnyValue(argValue, irFn) {
				tempVar := p.newTemp()
				var getterFunc string
				switch param.Type {
				case "int":
					getterFunc = "any_get_int"
				case "string":
					getterFunc = "any_get_string"
				case "float":
					getterFunc = "any_get_float"
				case "double":
					getterFunc = "any_get_double"
				case "bool":
					getterFunc = "any_get_bool"
				default:
					getterFunc = "any_get_ptr"
				}
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tempVar,
					Arg1:       getterFunc,
					Arg2:       argValue,
					ReturnType: param.Type,
				})
				args = append(args, tempVar)
			} else {
				args = append(args, argValue)
			}
		}
	} else {
		for _, arg := range call.Args {
			args = append(args, p.processExpression(arg, irFn))
		}
	}

	argsStr := strings.Join(args, ", ")

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

func (p *Pipeline) getAnyWrapper(value string, irFn *IRFunction) string {
	argType := p.inferType(value)
	switch argType {
	case "string":
		return "any_string"
	case "float":
		return "any_float"
	case "double":
		return "any_double"
	case "bool":
		return "any_bool"
	default:
		return "any_int"
	}
}

func (p *Pipeline) isAnyValue(value string, irFn *IRFunction) bool {
	for _, local := range irFn.Locals {
		if strings.Contains(local, value) && strings.Contains(local, "sk_any") {
			return true
		}
	}

	for _, ins := range irFn.Instructions {
		if ins.Result == value && ins.Op == "call" {
			if strings.Contains(ins.Arg1, "any_") {
				return true
			}
		}
	}
	return false
}

func (p *Pipeline) processCallExpr(call *front.CallExpr, irFn *IRFunction) string {
	var targetFunc *front.Function
	for _, fn := range p.Program.Functions {
		if fn.Name == call.Name {
			targetFunc = fn
			break
		}
	}

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

	args := []string{}
	argIndex := 0

	if targetFunc != nil {
		for _, param := range targetFunc.Params {
			var argExpr front.Node

			if argIndex < len(call.Args) {
				argExpr = call.Args[argIndex]
				argIndex++
			} else if param.DefaultValue != nil {
				argExpr = param.DefaultValue
			} else {
				args = append(args, "0")
				continue
			}

			argValue := p.processExpression(argExpr, irFn)

			if param.Type == "any" {
				tempVar := p.newTemp()
				wrapperFunc := p.getAnyWrapper(argValue, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tempVar,
					Arg1:       wrapperFunc,
					Arg2:       argValue,
					ReturnType: "sk_any",
				})
				args = append(args, tempVar)
				continue
			}

			if p.isAnyValue(argValue, irFn) {
				tempVar := p.newTemp()
				var getterFunc string
				switch param.Type {
				case "int":
					getterFunc = "any_get_int"
				case "string":
					getterFunc = "any_get_string"
				case "float":
					getterFunc = "any_get_float"
				case "double":
					getterFunc = "any_get_double"
				case "bool":
					getterFunc = "any_get_bool"
				default:
					getterFunc = "any_get_ptr"
				}
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tempVar,
					Arg1:       getterFunc,
					Arg2:       argValue,
					ReturnType: param.Type,
				})
				args = append(args, tempVar)
			} else {
				args = append(args, argValue)
			}
		}
	} else {
		for _, arg := range call.Args {
			args = append(args, p.processExpression(arg, irFn))
		}
	}

	argsStr := strings.Join(args, ", ")

	returnType := "sk_string"
	funcName := call.Name

	simpleName := funcName
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		simpleName = parts[len(parts)-1]
	}

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
	condResult := p.processExpression(ifStmt.Condition, irFn)

	ifLabel := p.newLabel()
	nextLabel := p.newLabel()

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "if",
		Result: condResult,
		Arg1:   ifLabel,
		Arg2:   nextLabel,
	})

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

func (p *Pipeline) processCase(caseStmt *front.CaseStmt, irFn *IRFunction) {
	value := p.processExpression(caseStmt.Value, irFn)

	endLabel := p.newLabel()

	for _, branch := range caseStmt.Branches {
		pattern := p.processExpression(branch.Pattern, irFn)
		branchLabel := p.newLabel()
		nextLabel := p.newLabel()

		cmp := fmt.Sprintf("(%s == %s)", value, pattern)

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "if",
			Result: cmp,
			Arg1:   branchLabel,
			Arg2:   nextLabel,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: branchLabel,
		})

		if branch.Body != nil {
			p.processBlock(branch.Body, irFn)
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "goto",
			Result: endLabel,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: nextLabel,
		})
	}

	if caseStmt.Default != nil {
		defaultLabel := p.newLabel()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: defaultLabel,
		})
		p.processBlock(caseStmt.Default, irFn)
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

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: startLabel,
	})

	condResult := p.processExpression(while.Condition, irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "if",
		Result: condResult,
		Arg1:   bodyLabel,
		Arg2:   endLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: bodyLabel,
	})

	if while.Body != nil {
		p.processBlock(while.Body, irFn)
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

func (p *Pipeline) newTemp() string {
	p.TempCounter++
	return fmt.Sprintf("t%d", p.TempCounter)
}

func (p *Pipeline) newLabel() string {
	p.LabelCounter++
	return fmt.Sprintf("L%d", p.LabelCounter)
}

func (p *Pipeline) inferType(value string) string {
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return "string"
	}
	if strings.Contains(value, ".") {
		return "double"
	}
	if value == "true" || value == "false" {
		return "bool"
	}
	if len(value) > 0 && (value[0] >= '0' && value[0] <= '9' || value[0] == '-') {
		return "int"
	}
	return "int"
}
