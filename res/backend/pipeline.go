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
	case *front.TypeOf:
		p.processTypeOf(n, irFn)
	case *front.Assign:
		p.processAssign(n, irFn)
	case *front.BinaryExpr:
		p.processBinary(n, irFn)
	case *front.UnaryExpr:
		p.processUnary(n, irFn)
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
	exprType := p.getExprType(typeOf.Expr, irFn)
	expr := p.processExpression(typeOf.Expr, irFn)
	result := p.newTemp()

	if exprType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "typeof_any",
			Result: result,
			Arg1:   expr,
		})
	} else {
		varType := exprType
		if varType == "" {
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
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()

		switch exprType {
		case "any":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "any_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		case "arr":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_array_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		case "string":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: result,
				Arg1:   expr,
			})
		case "bool":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "cast_bool_to_str",
				Result: result,
				Arg1:   expr,
			})
		case "float", "double":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "cast_num_to_str",
				Result: result,
				Arg1:   expr,
			})
		default:
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
				var elemType int
				elemSize := "sizeof(sk_any)"
				elemType = 5

				if decl.ElemType != "" {
					switch decl.ElemType {
					case "int":
						elemType = 0
						elemSize = "sizeof(int)"
					case "string":
						elemType = 1
						elemSize = "sizeof(sk_string)"
					case "float":
						elemType = 2
						elemSize = "sizeof(float)"
					case "double":
						elemType = 3
						elemSize = "sizeof(double)"
					case "bool":
						elemType = 4
						elemSize = "sizeof(sk_bool)"
					default:
						elemType = 5
						elemSize = "sizeof(sk_any)"
					}
				}

				irFn.Locals = append(irFn.Locals, "sk_array* "+decl.Name)

				if irFn.ArrayElemTypes == nil {
					irFn.ArrayElemTypes = make(map[string]string)
				}
				irFn.ArrayElemTypes[decl.Name] = decl.ElemType

				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     decl.Name,
					Arg1:       "sk_array_new",
					Arg2:       fmt.Sprintf("%s, %d", elemSize, elemType),
					ReturnType: "sk_array*",
				})

				for _, elem := range arrLit.Elements {
					elemTypeName := p.getExprType(elem, irFn)
					val := p.processExpression(elem, irFn)

					if decl.ElemType != "" {
						// Типизированный массив
						var pushFunc string
						switch decl.ElemType {
						case "int":
							pushFunc = "sk_array_push_int"
						case "string":
							pushFunc = "sk_array_push_string"
						case "float":
							pushFunc = "sk_array_push_float"
						case "double":
							pushFunc = "sk_array_push_double"
						case "bool":
							pushFunc = "sk_array_push_bool"
						default:
							pushFunc = "sk_array_push_any"
						}
						irFn.Instructions = append(irFn.Instructions, IRInstruction{
							Op:   "call",
							Arg1: pushFunc,
							Arg2: decl.Name + ", " + val,
						})
					} else {
						// Нетипизированный массив — оборачиваем в any
						if elemTypeName == "any" {
							irFn.Instructions = append(irFn.Instructions, IRInstruction{
								Op:   "call",
								Arg1: "sk_array_push_any",
								Arg2: decl.Name + ", " + val,
							})
						} else {
							wrapper := p.getAnyWrapperByType(elemTypeName)
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
								Arg1: "sk_array_push_any",
								Arg2: decl.Name + ", " + tempVar,
							})
						}
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
		exprType := p.getExprType(decl.Expr, irFn)
		exprResult := p.processExpression(decl.Expr, irFn)

		if decl.Type == "any" {
			if exprType == "any" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: decl.Name,
					Arg1:   exprResult,
				})
			} else {
				tempVar := p.newTemp()
				wrapperFunc := p.getAnyWrapperByType(exprType)
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
			}
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
		return "sk_array*"
	case "dict":
		return "void*"
	case "any":
		return "sk_any"
	default:
		return "int"
	}
}

func (p *Pipeline) processAssign(assign *front.Assign, irFn *IRFunction) {
	if assign.Expr == nil {
		return
	}

	exprType := p.getExprType(assign.Expr, irFn)
	exprResult := p.processExpression(assign.Expr, irFn)

	// Определяем тип переменной
	varType := ""
	for _, local := range irFn.Locals {
		parts := strings.Fields(local)
		if len(parts) >= 2 && parts[len(parts)-1] == assign.Name {
			switch parts[0] {
			case "int":
				varType = "int"
			case "sk_string":
				varType = "string"
			case "float":
				varType = "float"
			case "double":
				varType = "double"
			case "sk_bool":
				varType = "bool"
			case "sk_array*":
				varType = "arr"
			case "sk_any":
				varType = "any"
			}
			break
		}
	}

	if varType == "any" {
		if exprType == "any" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: assign.Name,
				Arg1:   exprResult,
			})
		} else {
			tempVar := p.newTemp()
			wrapperFunc := p.getAnyWrapperByType(exprType)
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
		}
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: assign.Name,
			Arg1:   exprResult,
		})
	}
}

func (p *Pipeline) processBinary(bin *front.BinaryExpr, irFn *IRFunction) string {
	leftType := p.getExprType(bin.Left, irFn)
	rightType := p.getExprType(bin.Right, irFn)

	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	// Конкатенация строк
	if bin.Op == "+" && (leftType == "string" || rightType == "string") {
		leftVal := left
		if leftType == "any" {
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
		rightVal := right
		if rightType == "any" {
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

	// Арифметика/сравнения — распаковываем any если надо
	leftVal := left
	if leftType == "any" {
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
	if rightType == "any" {
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
	case *front.ArrayIndex:
		return p.processArrayIndex(n, irFn)
	case *front.ArrayLength:
		return p.processArrayLength(n, irFn)
	case *front.ArrayAdd:
		return p.processArrayAdd(n, irFn)
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
	targetFunc := p.findFunction(call.Name)

	args := []string{}

	if targetFunc != nil {
		argIndex := 0
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

			args = append(args, p.prepareArg(argExpr, param.Type, irFn))
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

	targetFunc := p.findFunction(call.Name)

	args := []string{}

	if targetFunc != nil {
		argIndex := 0
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

			args = append(args, p.prepareArg(argExpr, param.Type, irFn))
		}
	} else {
		for _, arg := range call.Args {
			args = append(args, p.processExpression(arg, irFn))
		}
	}

	argsStr := strings.Join(args, ", ")

	// Определяем тип возврата
	returnType := "sk_string"
	funcName := call.Name
	simpleName := funcName
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		simpleName = parts[len(parts)-1]
	}

	for _, fn := range p.Program.Functions {
		if fn.Name == simpleName || fn.Name == funcName {
			returnType = p.typeToC(fn.ReturnType)
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

// prepareArg подготавливает аргумент: оборачивает в any или распаковывает из any
func (p *Pipeline) prepareArg(argExpr front.Node, paramType string, irFn *IRFunction) string {
	argType := p.getExprType(argExpr, irFn)
	argValue := p.processExpression(argExpr, irFn)

	if paramType == "any" {
		if argType == "any" {
			return argValue
		}
		wrapper := p.getAnyWrapperByType(argType)
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       wrapper,
			Arg2:       argValue,
			ReturnType: "sk_any",
		})
		return tempVar
	}

	if argType == "any" {
		getter := p.getAnyGetterByType(paramType)
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       getter,
			Arg2:       argValue,
			ReturnType: p.typeToC(paramType),
		})
		return tempVar
	}

	return argValue
}

func (p *Pipeline) findFunction(name string) *front.Function {
	for _, fn := range p.Program.Functions {
		if fn.Name == name {
			return fn
		}
	}
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		simpleName := parts[len(parts)-1]
		for _, fn := range p.Program.Functions {
			if fn.Name == simpleName {
				return fn
			}
		}
	}
	return nil
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
	valueType := p.getExprType(caseStmt.Value, irFn)

	// Распаковываем any если нужно
	if valueType == "any" {
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       "any_get_int",
			Arg2:       value,
			ReturnType: "int",
		})
		value = tempVar
	}

	endLabel := p.newLabel()

	for _, branch := range caseStmt.Branches {
		patternType := p.getExprType(branch.Pattern, irFn)
		pattern := p.processExpression(branch.Pattern, irFn)

		if patternType == "any" {
			tempVar := p.newTemp()
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempVar,
				Arg1:       "any_get_int",
				Arg2:       pattern,
				ReturnType: "int",
			})
			pattern = tempVar
		}

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

func (p *Pipeline) processArrayIndex(idx *front.ArrayIndex, irFn *IRFunction) string {
	elemType := ""
	if irFn.ArrayElemTypes != nil {
		elemType = irFn.ArrayElemTypes[idx.Name]
	}

	result := p.newTemp()
	index := p.processExpression(idx.Index, irFn)

	if elemType == "" || elemType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "array_get_any",
			Result:     result,
			Arg1:       idx.Name,
			Arg2:       index,
			ReturnType: "sk_any",
		})
	} else {
		cType := p.typeToC(elemType)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "array_get_typed",
			Result:     result,
			Arg1:       idx.Name,
			Arg2:       index,
			ReturnType: cType,
		})
	}
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
	elemType := p.getExprType(add.Elem, irFn)
	elemVal := p.processExpression(add.Elem, irFn)

	result := p.newTemp()

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "call",
		Result: result,
		Arg1:   "sk_array_copy",
		Arg2:   add.Name,
	})

	if elemType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "call",
			Arg1: "sk_array_push",
			Arg2: result + ", &" + elemVal,
		})
	} else {
		wrapper := p.getAnyWrapperByType(elemType)
		tempVar := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       wrapper,
			Arg2:       elemVal,
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

// ============ Встроенные функции ============

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

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()

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

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()

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
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "0.0",
		})
		return result
	}

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()

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

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()

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
	case "arr":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_array_to_string",
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

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()

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
	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_array_new",
		Arg2:       "sizeof(sk_any), 5",
		ReturnType: "sk_array*",
	})

	for _, arg := range call.Args {
		argType := p.getExprType(arg, irFn)
		val := p.processExpression(arg, irFn)

		if argType == "any" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:   "call",
				Arg1: "sk_array_push_any",
				Arg2: result + ", " + val,
			})
		} else {
			wrapper := p.getAnyWrapperByType(argType)
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
				Arg1: "sk_array_push_any",
				Arg2: result + ", " + tempVar,
			})
		}
	}

	return result
}

// ============ Система типов ============

// getExprType возвращает реальный тип AST-узла
func (p *Pipeline) getExprType(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		if strings.Contains(n.Value, ".") {
			return "double"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.Ident:
		// true/false — bool
		if n.Name == "true" || n.Name == "false" {
			return "bool"
		}
		// Параметры
		for _, param := range irFn.Params {
			if param.Name == n.Name {
				return param.Type
			}
		}
		// Локальные переменные
		for _, local := range irFn.Locals {
			parts := strings.Fields(local)
			if len(parts) >= 2 {
				name := parts[len(parts)-1]
				if name == n.Name {
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
		}
		return ""
	case *front.BinaryExpr:
		leftType := p.getExprType(n.Left, irFn)
		rightType := p.getExprType(n.Right, irFn)
		if n.Op == "+" && (leftType == "string" || rightType == "string") {
			return "string"
		}
		if n.Op == "<" || n.Op == ">" || n.Op == "==" || n.Op == "!=" || n.Op == "<=" || n.Op == ">=" {
			return "bool"
		}
		if leftType == "double" || rightType == "double" {
			return "double"
		}
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		if leftType == "int" && rightType == "int" {
			return "int"
		}
		return "int"
	case *front.UnaryExpr:
		if n.Op == "$" {
			return "string"
		}
		return p.getExprType(n.Expr, irFn)
	case *front.CallExpr:
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
		for _, fn := range p.Program.Functions {
			if fn.Name == n.Name {
				return fn.ReturnType
			}
		}
		return ""
	case *front.TypeOf:
		return "string"
	case *front.ArrayLiteral:
		return "arr"
	case *front.ArrayIndex:
		if irFn.ArrayElemTypes != nil {
			elemType, ok := irFn.ArrayElemTypes[n.Name]
			if ok {
				if elemType == "" {
					return "any"
				}
				return elemType
			}
		}
		return ""
	case *front.ArrayLength:
		return "int"
	case *front.ArrayAdd:
		return "arr"
	default:
		return ""
	}
}

func (p *Pipeline) getAnyWrapperByType(t string) string {
	switch t {
	case "int":
		return "any_int"
	case "string":
		return "any_string"
	case "float":
		return "any_float"
	case "double":
		return "any_double"
	case "bool":
		return "any_bool"
	case "arr":
		return "any_ptr"
	default:
		return "any_int"
	}
}

func (p *Pipeline) getAnyGetterByType(t string) string {
	switch t {
	case "int":
		return "any_get_int"
	case "string":
		return "any_get_string"
	case "float":
		return "any_get_float"
	case "double":
		return "any_get_double"
	case "bool":
		return "any_get_bool"
	case "arr":
		return "any_get_ptr"
	default:
		return "any_get_int"
	}
}

func (p *Pipeline) newTemp() string {
	p.TempCounter++
	return fmt.Sprintf("t%d", p.TempCounter)
}

func (p *Pipeline) newLabel() string {
	p.LabelCounter++
	return fmt.Sprintf("L%d", p.LabelCounter)
}
