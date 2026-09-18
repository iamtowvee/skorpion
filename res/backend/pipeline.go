package backend

import (
	"fmt"
	"skrp/res/debug"
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
	case *front.TypeOf:
		p.processTypeOf(n, irFn)
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
	case *front.ArrayIndex:
		p.processArrayIndex(n, irFn)
	case *front.ArrayLength:
		p.processArrayLength(n, irFn)
	case *front.ArrayAdd:
		p.processArrayAdd(n, irFn)
	}
}

func (p *Pipeline) processTypeOf(typeOf *front.TypeOf, irFn *IRFunction) string {
	expr := p.processExpression(typeOf.Expr, irFn)
	result := p.newTemp()

	// Проверяем, является ли выражение any
	if p.isAnyValue(expr, irFn) {
		// Для any нужно проверить type поле
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "typeof_any",
			Result: result,
			Arg1:   expr,
		})
	} else {
		// Для конкретных типов — просто возвращаем строку
		var varType string
		if p.isStringValue(expr, irFn) || strings.HasPrefix(expr, "\"") {
			varType = "string"
		} else if p.isIntValue(expr, irFn) {
			varType = "int"
		} else if p.isFloatValue(expr, irFn) {
			varType = "float"
		} else if p.isBoolValue(expr, irFn) {
			varType = "bool"
		} else if p.isArrayValue(expr, irFn) {
			varType = "arr"
		} else {
			varType = "unknown"
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf(`"%s"`, varType),
		})
	}

	return result
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
	if decl.IsArray {
		if decl.Expr != nil {
			if arrLit, ok := decl.Expr.(*front.ArrayLiteral); ok {
				// Определяем тип элемента
				var elemTypeStr string
				debug.Debug(elemTypeStr)
				var elemType int
				elemSize := "sizeof(sk_any)"
				elemType = 5 // any по умолчанию

				if decl.ElemType != "" {
					switch decl.ElemType {
					case "int":
						elemTypeStr = "int"
						elemType = 0
						elemSize = "sizeof(int)"
					case "string":
						elemTypeStr = "sk_string"
						elemType = 1
						elemSize = "sizeof(sk_string)"
					case "float":
						elemTypeStr = "float"
						elemType = 2
						elemSize = "sizeof(float)"
					case "double":
						elemTypeStr = "double"
						elemType = 3
						elemSize = "sizeof(double)"
					case "bool":
						elemTypeStr = "sk_bool"
						elemType = 4
						elemSize = "sizeof(sk_bool)"
					default:
						elemTypeStr = "sk_any"
						elemType = 5
						elemSize = "sizeof(sk_any)"
					}
				}

				// Объявляем массив как sk_array*
				irFn.Locals = append(irFn.Locals, "sk_array* "+decl.Name)

				// sk_array* arr = sk_array_new(elem_size, elem_type);
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     decl.Name,
					Arg1:       "sk_array_new",
					Arg2:       fmt.Sprintf("%s, %d", elemSize, elemType),
					ReturnType: "sk_array*",
				})

				// push each element
				for _, elem := range arrLit.Elements {
					val := p.processExpression(elem, irFn)

					// Если нетипизированный массив, оборачиваем в any
					if decl.ElemType == "" {
						wrapper := p.getAnyWrapper(val, irFn)
						tempVar := p.newTemp()
						irFn.Instructions = append(irFn.Instructions, IRInstruction{
							Op:         "call",
							Result:     tempVar,
							Arg1:       wrapper,
							Arg2:       val,
							ReturnType: "sk_any",
						})
						irFn.Instructions = append(irFn.Instructions, IRInstruction{
							Op:   "call",
							Arg1: "sk_array_push",
							Arg2: decl.Name + ", &" + tempVar,
						})
					} else {
						// Для типизированного массива просто передаём адрес
						irFn.Instructions = append(irFn.Instructions, IRInstruction{
							Op:   "call",
							Arg1: "sk_array_push",
							Arg2: decl.Name + ", &" + val,
						})
					}
				}
			}
		}
		return
	}

	// Обычная переменная
	cType := p.typeToC(decl.Type)
	irFn.Locals = append(irFn.Locals, cType+" "+decl.Name)

	if decl.Expr != nil {
		exprResult := p.processExpression(decl.Expr, irFn)

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
	case *front.TypeOf:
		return p.processTypeOf(n, irFn)
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
	// Проверяем параметры
	for _, param := range irFn.Params {
		if param.Name == value && param.Type == "any" {
			return true
		}
	}
	// Проверяем локальные переменные
	for _, local := range irFn.Locals {
		if strings.Contains(local, value) && strings.Contains(local, "sk_any") {
			return true
		}
	}
	// Проверяем инструкции
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
	switch call.Name {
	case "to_int":
		return p.processToInt(call, irFn)
	case "to_float":
		return p.processToFloat(call, irFn)
	case "to_double":
		return p.processToDouble(call, irFn)
	case "to_string":
		return p.processToString(call, irFn)
	case "to_bool":
		return p.processToBool(call, irFn)
	case "to_arr":
		return p.processToArr(call, irFn)
	}

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

// isIntValue проверяет, является ли значение int
func (p *Pipeline) isIntValue(value string, irFn *IRFunction) bool {
	// Проверяем локальные переменные
	for _, local := range irFn.Locals {
		// local это "int i" или "sk_string s"
		parts := strings.Fields(local)
		if len(parts) >= 2 && parts[1] == value {
			if parts[0] == "int" {
				return true
			}
		}
	}

	// Проверяем, что это числовой литерал (не строка, не bool)
	if len(value) > 0 && (value[0] >= '0' && value[0] <= '9' || value[0] == '-') {
		if strings.Contains(value, ".") {
			return false
		}
		return true
	}

	return false
}

// isFloatValue проверяет, является ли значение float/double
func (p *Pipeline) isFloatValue(value string, irFn *IRFunction) bool {
	// Проверяем локальные переменные
	for _, local := range irFn.Locals {
		parts := strings.Fields(local)
		if len(parts) >= 2 && parts[1] == value {
			if parts[0] == "float" || parts[0] == "double" {
				return true
			}
		}
	}

	// Проверяем, что это числовой литерал с точкой
	if len(value) > 0 && (value[0] >= '0' && value[0] <= '9' || value[0] == '-') {
		if strings.Contains(value, ".") {
			return true
		}
	}

	return false
}

// isBoolValue проверяет, является ли значение bool
func (p *Pipeline) isBoolValue(value string, irFn *IRFunction) bool {
	// Проверяем локальные переменные
	for _, local := range irFn.Locals {
		parts := strings.Fields(local)
		if len(parts) >= 2 && parts[1] == value {
			if parts[0] == "sk_bool" || parts[0] == "bool" {
				return true
			}
		}
	}

	// Проверяем литералы true/false
	if value == "true" || value == "false" {
		return true
	}

	return false
}

// isArrayValue проверяет, является ли значение массивом
func (p *Pipeline) isArrayValue(value string, irFn *IRFunction) bool {
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return true
	}

	for _, local := range irFn.Locals {
		if strings.Contains(local, value) && strings.Contains(local, "sk_array*") {
			return true
		}
	}

	// Проверяем инструкции, где результат — sk_array*
	for _, ins := range irFn.Instructions {
		if ins.Result == value {
			switch ins.Op {
			case "call":
				if ins.Arg1 == "sk_array_new" || ins.Arg1 == "sk_array_copy" {
					return true
				}
			}
		}
	}

	return false
}

func (p *Pipeline) processArrayIndex(idx *front.ArrayIndex, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "array_get",
		Result: result,
		Arg1:   idx.Name,
		Arg2:   p.processExpression(idx.Index, irFn),
	})
	return result
}

func (p *Pipeline) processArrayLength(length *front.ArrayLength, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "array_len",
		Result: result,
		Arg1:   length.Name,
	})
	return result
}

func (p *Pipeline) processArrayAdd(add *front.ArrayAdd, irFn *IRFunction) string {
	// arr + el → sk_array_copy(arr); sk_array_push(new, &el)
	elemVal := p.processExpression(add.Elem, irFn)

	// Определяем тип элемента из существующего массива
	// Пока просто копируем и пушим
	result := p.newTemp()

	// sk_array* new = sk_array_copy(arr)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "call",
		Result: result,
		Arg1:   "sk_array_copy",
		Arg2:   add.Name,
	})

	// sk_array_push(new, &elem)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   "call",
		Arg1: "sk_array_push",
		Arg2: result + ", &" + elemVal,
	})

	return result
}

func (p *Pipeline) processToInt(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "0",
		})
		return result
	}

	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	argType := p.inferExprType(call.Args[0], irFn)

	switch argType {
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_bool_to_int",
			Result: result,
			Arg1:   arg,
		})
	case "float", "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_num_to_int",
			Result: result,
			Arg1:   arg,
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_str_to_int",
			Result: result,
			Arg1:   arg,
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_int",
			Arg2:       arg,
			ReturnType: "int",
		})
	default:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   arg,
		})
	}

	return result
}

func (p *Pipeline) processToFloat(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "0.0",
		})
		return result
	}

	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	argType := p.inferExprType(call.Args[0], irFn)

	switch argType {
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_bool_to_float",
			Result: result,
			Arg1:   arg,
		})
	case "int", "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_num_to_float",
			Result: result,
			Arg1:   arg,
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_str_to_float",
			Result: result,
			Arg1:   arg,
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_float",
			Arg2:       arg,
			ReturnType: "float",
		})
	default:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   arg,
		})
	}

	return result
}

func (p *Pipeline) processToDouble(call *front.CallExpr, irFn *IRFunction) string {
	// аналогично to_float, но ReturnType double
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "0.0",
		})
		return result
	}

	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	argType := p.inferExprType(call.Args[0], irFn)

	switch argType {
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_bool_to_double",
			Result: result,
			Arg1:   arg,
		})
	case "int", "float":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_num_to_double",
			Result: result,
			Arg1:   arg,
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_str_to_double",
			Result: result,
			Arg1:   arg,
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_double",
			Arg2:       arg,
			ReturnType: "double",
		})
	default:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   arg,
		})
	}

	return result
}

func (p *Pipeline) processToString(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   `""`,
		})
		return result
	}

	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	argType := p.inferExprType(call.Args[0], irFn)

	switch argType {
	case "int":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "to_string",
			Result: result,
			Arg1:   arg,
		})
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_bool_to_str",
			Result: result,
			Arg1:   arg,
		})
	case "float", "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_num_to_str",
			Result: result,
			Arg1:   arg,
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_string",
			Arg2:       arg,
			ReturnType: "sk_string",
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   arg,
		})
	default:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "to_string",
			Result: result,
			Arg1:   arg,
		})
	}

	return result
}

func (p *Pipeline) processToBool(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "0",
		})
		return result
	}

	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	argType := p.inferExprType(call.Args[0], irFn)

	switch argType {
	case "int", "float", "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_num_to_bool",
			Result: result,
			Arg1:   arg,
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "cast_str_to_bool",
			Result: result,
			Arg1:   arg,
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_bool",
			Arg2:       arg,
			ReturnType: "sk_bool",
		})
	default:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   arg,
		})
	}

	return result
}

func (p *Pipeline) processToArr(call *front.CallExpr, irFn *IRFunction) string {
	// to_arr(a, b, c) → создаём массив с этими элементами
	result := p.newTemp()

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_array_new",
		Arg2:       "sizeof(sk_any), 5",
		ReturnType: "sk_array*",
	})

	for _, arg := range call.Args {
		val := p.processExpression(arg, irFn)
		wrapper := p.getAnyWrapper(val, irFn)
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       wrapper,
			Arg2:       val,
			ReturnType: "sk_any",
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "call",
			Arg1: "sk_array_push",
			Arg2: result + ", &" + tempVar,
		})
	}

	return result
}

func (p *Pipeline) inferExprType(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		if strings.Contains(n.Value, ".") {
			return "double"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.Ident:
		// Проверяем параметры функции
		for _, param := range irFn.Params {
			if param.Name == n.Name {
				switch param.Type {
				case "int":
					return "int"
				case "string":
					return "string"
				case "float":
					return "float"
				case "double":
					return "double"
				case "bool":
					return "bool"
				case "arr":
					return "arr"
				case "any":
					return "any"
				}
			}
		}
		// Проверяем локальные переменные
		if p.isAnyValue(n.Name, irFn) {
			return "any"
		}
		for _, local := range irFn.Locals {
			parts := strings.Fields(local)
			if len(parts) >= 2 && parts[1] == n.Name {
				switch parts[0] {
				case "int":
					return "int"
				case "sk_string":
					return "string"
				case "float":
					return "float"
				case "double":
					return "double"
				case "sk_bool":
					return "bool"
				case "sk_array*":
					return "arr"
				case "sk_any":
					return "any"
				}
			}
		}
		return ""
	case *front.CallExpr:
		// Встроенные функции
		switch n.Name {
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
			return "arr"
		}
		// Пользовательские функции
		for _, fn := range p.Program.Functions {
			if fn.Name == n.Name {
				return fn.ReturnType
			}
		}
		return ""
	case *front.UnaryExpr:
		if n.Op == "$" {
			return "string"
		}
		return p.inferExprType(n.Expr, irFn)
	case *front.TypeOf:
		return "string"
	case *front.ArrayLiteral:
		return "arr"
	case *front.ArrayIndex:
		return ""
	case *front.ArrayLength:
		return "int"
	case *front.ArrayAdd:
		return "arr"
	default:
		return ""
	}
}
