package backend

import (
	"fmt"
	"skrp/res/front"
	"strconv"
	"strings"
)

type Pipeline struct {
	Program      *front.Program
	IR           *IRProgram
	TempCounter  int
	LabelCounter int
	TryFrames    []string
	TryDepth     int
}

func NewPipeline(prog *front.Program) *Pipeline {
	return &Pipeline{
		Program:      prog,
		IR:           &IRProgram{Functions: []IRFunction{}, Globals: []IRGlobal{}, Imports: []IRImport{}},
		TempCounter:  0,
		LabelCounter: 0,
		TryFrames:    []string{},
		TryDepth:     0,
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

	// Обрабатываем ErrorDecls
	for _, decl := range p.Program.ErrorDecls {
		irDecl := IRErrorDecl{
			Name:   decl.Name,
			Parent: decl.Parent,
			Fields: []IRErrorField{},
		}
		for _, field := range decl.Fields {
			irDecl.Fields = append(irDecl.Fields, IRErrorField{
				Name: field.Name,
				Type: field.Type,
			})
		}
		p.IR.ErrorDecls = append(p.IR.ErrorDecls, irDecl)
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
		Name:           fn.Name,
		ReturnType:     fn.ReturnType,
		IsExport:       fn.IsExport,
		File:           fn.File,
		Params:         []IRParam{},
		Locals:         []string{},
		Instructions:   []IRInstruction{},
		ArrayElemTypes: make(map[string]string),
	}

	for _, param := range fn.Params {
		irFn.Params = append(irFn.Params, IRParam{
			Name: param.Name,
			Type: param.Type,
		})
	}

	p.TryFrames = []string{}
	p.TryDepth = 0

	if fn.Body != nil {
		p.processBlock(fn.Body, &irFn)
	}

	if fn.ReturnType == "void" && len(irFn.Instructions) > 0 {
		lastIns := irFn.Instructions[len(irFn.Instructions)-1]
		if lastIns.Op != "ret" {
			for i := len(p.TryFrames) - 1; i >= 0; i-- {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   "try_pop",
					Arg1: p.TryFrames[i],
				})
			}
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
	case *front.ThrowStmt:
		p.processThrow(n, irFn)
	case *front.TryStmt:
		p.processTry(n, irFn)
	case *front.FieldAccess:
		p.processFieldAccess(n, irFn)
	case *front.ErrorInstance:
		p.processErrorInstance(n, irFn)
	case *front.TernaryExpr:
		p.processTernary(n, irFn)
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
	case *front.RangeExpr:
		p.processRange(n, irFn)
	case *front.CallRangeExpr:
		p.processCallRange(n, irFn)
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

func (p *Pipeline) typeToC(typ string) string {
	switch typ {
	case "int":
		return "sk_int"
	case "string":
		return "sk_string"
	case "float":
		return "sk_float"
	case "double":
		return "sk_double"
	case "bool":
		return "sk_bool"
	case "char":
		return "char"
	case "void":
		return "void"
	case "arr":
		return "sk_arr"
	case "dict":
		return "void*"
	case "any":
		return "sk_any"
	case "null":
		return "void*"
	default:
		return "sk_int"
	}
}

func (p *Pipeline) processFieldAccess(fa *front.FieldAccess, irFn *IRFunction) string {
	fieldType := "int"

	// Ищем тип поля в ErrorDecls
	for _, decl := range p.Program.ErrorDecls {
		if decl.Name == fa.Object {
			for _, f := range decl.Fields {
				if f.Name == fa.Field {
					fieldType = f.Type
					break
				}
			}
		}
	}

	// Встроенный Error.msg
	if fa.Field == "msg" {
		fieldType = "string"
	}

	result := p.newTemp()
	cType := p.typeToC(fieldType)
	irFn.Locals = append(irFn.Locals, cType+" "+result)

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "field_access",
		Result:     result,
		Arg1:       fa.Object,
		Arg2:       fa.Field,
		ReturnType: cType,
	})

	return result
}

func (p *Pipeline) processTry(try *front.TryStmt, irFn *IRFunction) {
	p.TryDepth++
	defer func() { p.TryDepth-- }()

	endLabel := p.newLabel()

	catchLabels := make([]string, len(try.Catches))
	nextCatchLabels := make([]string, len(try.Catches))
	for i := range try.Catches {
		catchLabels[i] = p.newLabel()
		nextCatchLabels[i] = p.newLabel()
	}

	frameVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, "SkTryFrame "+frameVar)
	errorVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, "void* "+errorVar)

	p.TryFrames = append(p.TryFrames, frameVar)

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "try_push",
		Result: frameVar,
		Arg1:   catchLabels[0],
	})

	p.processBlock(try.Body, irFn)

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "goto",
		Result: endLabel,
	})

	framePopped := false

	for i, clause := range try.Catches {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: catchLabels[i],
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "try_pop",
			Arg1: frameVar,
		})
		if !framePopped {
			if len(p.TryFrames) > 0 && p.TryFrames[len(p.TryFrames)-1] == frameVar {
				p.TryFrames = p.TryFrames[:len(p.TryFrames)-1]
			}
			framePopped = true
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "try_get_error",
			Result: errorVar,
			Arg1:   frameVar,
		})

		if clause.TypeName != "" && clause.TypeName != "Error" {
			typeCheckVar := p.newTemp()
			irFn.Locals = append(irFn.Locals, "int "+typeCheckVar)

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "error_type_match",
				Result:     typeCheckVar,
				Arg1:       errorVar,
				Arg2:       fmt.Sprintf(`"%s"`, p.fullErrorTypePath(clause.TypeName)),
				ReturnType: "int",
			})

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "if",
				Result: typeCheckVar,
				Arg1:   catchLabels[i] + "_body",
				Arg2:   nextCatchLabels[i],
			})

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "label",
				Result: catchLabels[i] + "_body",
			})
		}

		if clause.VarName != "" {
			typeName := clause.TypeName
			if typeName == "" {
				typeName = "Error"
			}
			irFn.Locals = append(irFn.Locals, typeName+"* "+clause.VarName)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "error_cast",
				Result: clause.VarName,
				Arg1:   errorVar,
				Arg2:   typeName,
			})
		}

		p.processBlock(clause.Body, irFn)

		// Освобождаем ошибку после catch
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "error_free",
			Arg1: errorVar,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "goto",
			Result: endLabel,
		})

		if clause.TypeName != "" && clause.TypeName != "Error" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "label",
				Result: nextCatchLabels[i],
			})
		}
	}

	lastIsCatchAll := len(try.Catches) > 0 && (try.Catches[len(try.Catches)-1].TypeName == "" || try.Catches[len(try.Catches)-1].TypeName == "Error")
	if !lastIsCatchAll {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "try_get_error",
			Result: errorVar,
			Arg1:   frameVar,
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "throw",
			Arg1:   errorVar,
			Arg2:   fmt.Sprintf(`"%s"`, irFn.File),
			Line:   try.GetLine(),
			Column: try.GetColumn(),
		})
	}

	if !framePopped {
		if len(p.TryFrames) > 0 && p.TryFrames[len(p.TryFrames)-1] == frameVar {
			p.TryFrames = p.TryFrames[:len(p.TryFrames)-1]
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "try_pop",
			Arg1: frameVar,
		})
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: endLabel,
	})
}

func (p *Pipeline) processTypeOf(typeOf *front.TypeOf, irFn *IRFunction) string {
	exprType := p.getExprType(typeOf.Expr, irFn)
	expr := p.processExpression(typeOf.Expr, irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_string "+result)

	// type(null) → "void"
	if exprType == "void" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_string "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_new",
			Arg2:       `"void"`,
			ReturnType: "sk_string",
		})
		return result
	}

	if exprType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "typeof_any",
			Result:     result,
			Arg1:       expr,
			ReturnType: "sk_string",
		})
	} else {
		varType := exprType
		if varType == "" {
			varType = "unknown"
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_new",
			Arg2:       fmt.Sprintf(`"%s"`, varType),
			ReturnType: "sk_string",
		})
	}

	return result
}

func (p *Pipeline) processErrorInstance(inst *front.ErrorInstance, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, inst.TypeName+"* "+result)

	// Выделяем в куче
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "error_instance_create",
		Result:     result,
		Arg1:       fmt.Sprintf(`"%s"`, p.fullErrorTypePath(inst.TypeName)),
		Arg2:       inst.TypeName,
		ReturnType: inst.TypeName + "*",
	})

	for fieldName, value := range inst.Fields {
		val := p.processExpression(value, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "field_set",
			Result: result,
			Arg1:   fieldName,
			Arg2:   val,
		})
	}

	return result
}

func (p *Pipeline) fullErrorTypePath(typeName string) string {
	if typeName == "Error" {
		return "Error"
	}

	// Строим путь от корня
	var decl *front.ErrorDecl
	for _, d := range p.Program.ErrorDecls {
		if d.Name == typeName {
			decl = d
			break
		}
	}
	if decl == nil {
		return "Error." + typeName
	}

	// Собираем цепочку имён
	path := []string{typeName}
	parent := decl.Parent
	for parent != "" && parent != "Error" {
		path = append([]string{parent}, path...)

		var parentDecl *front.ErrorDecl
		for _, d := range p.Program.ErrorDecls {
			if d.Name == parent {
				parentDecl = d
				break
			}
		}
		if parentDecl == nil {
			break
		}
		parent = parentDecl.Parent
	}

	return "Error." + strings.Join(path, ".")
}

func (p *Pipeline) processThrow(throw *front.ThrowStmt, irFn *IRFunction) {
	expr := p.processExpression(throw.Expr, irFn)

	fileName := irFn.File
	if fileName == "" {
		fileName = "<unknown>"
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "throw",
		Arg1:   expr,
		Arg2:   fmt.Sprintf(`"%s"`, fileName),
		Line:   throw.GetLine(),
		Column: throw.GetColumn(),
	})
}

func (p *Pipeline) processUnary(unary *front.UnaryExpr, irFn *IRFunction) string {
	if unary.Op == "!" {
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)

		if exprType == "any" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "any_to_bool",
				Arg2:       expr,
				ReturnType: "sk_bool",
			})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_bool_not",
				Arg2:       result,
				ReturnType: "sk_bool",
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_bool_not",
				Arg2:       expr,
				ReturnType: "sk_bool",
			})
		}
		return result
	}
	if unary.Op == "-" {
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()
		cType := p.typeToC(exprType)
		irFn.Locals = append(irFn.Locals, cType+" "+result)

		fnName := ""
		switch exprType {
		case "int":
			fnName = "sk_int_neg"
		case "float":
			fnName = "sk_float_neg"
		case "double":
			fnName = "sk_double_neg"
		}
		if fnName != "" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       fnName,
				Arg2:       expr,
				ReturnType: cType,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: result,
				Arg1:   expr,
			})
		}
		return result
	}
	if unary.Op == "$" {
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_string "+result)

		if exprType == "any" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "any_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		} else if isArrayType(exprType) {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_arr_to_string",
				Arg2:       expr, // передаём sk_arr, а не expr.value
				ReturnType: "sk_string",
			})
		} else if exprType == "string" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: result,
				Arg1:   expr,
			})
		} else if exprType == "bool" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_bool_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		} else if exprType == "float" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_float_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		} else if exprType == "double" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_double_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		} else {
			// int
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_int_to_string",
				Arg2:       expr,
				ReturnType: "sk_string",
			})
		}
		return result
	}
	return "SK_NULL_int"
}

func (p *Pipeline) processVarDecl(decl *front.VarDecl, irFn *IRFunction) {
	if decl.IsArray {
		irFn.Locals = append(irFn.Locals, "sk_arr "+decl.Name)

		if irFn.ArrayElemTypes == nil {
			irFn.ArrayElemTypes = make(map[string]string)
		}
		irFn.ArrayElemTypes[decl.Name] = decl.ElemType

		// Регистрация в cleanup ДО инициализации (чтобы при throw в инициализаторе — освободить)
		if p.TryDepth > 0 {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "try_register",
				Result: decl.Name,
				Arg1:   "1",
			})
		}

		if decl.Expr != nil {
			exprType := p.getExprType(decl.Expr, irFn)

			// null (void) → SK_NULL_arr
			if exprType == "void" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: decl.Name,
					Arg1:   "SK_NULL_arr",
				})
				return
			}

			exprResult := p.processExpressionTyped(decl.Expr, decl.ElemType, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   exprResult,
			})
		} else {
			var elemType int = 5
			elemSize := "sizeof(sk_any)"
			if decl.ElemType != "" {
				if isArrayType(decl.ElemType) {
					elemType = 6
					elemSize = "sizeof(sk_arr)"
				} else {
					switch decl.ElemType {
					case "int":
						elemType = 0
						elemSize = "sizeof(sk_int)"
					case "string":
						elemType = 1
						elemSize = "sizeof(sk_string)"
					case "float":
						elemType = 2
						elemSize = "sizeof(sk_float)"
					case "double":
						elemType = 3
						elemSize = "sizeof(sk_double)"
					case "bool":
						elemType = 4
						elemSize = "sizeof(sk_bool)"
					default:
						elemType = 5
						elemSize = "sizeof(sk_any)"
					}
				}
			}
			tmpVar := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_array* "+tmpVar)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmpVar,
				Arg1:       "sk_array_new",
				Arg2:       fmt.Sprintf("%s, %d", elemSize, elemType),
				ReturnType: "sk_array*",
			})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     decl.Name,
				Arg1:       "sk_arr_new",
				Arg2:       tmpVar,
				ReturnType: "sk_arr",
			})
		}
		return
	}

	cType := p.typeToC(decl.Type)
	irFn.Locals = append(irFn.Locals, cType+" "+decl.Name)

	// Регистрация строк
	if p.TryDepth > 0 && decl.Type == "string" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "try_register",
			Result: decl.Name + ".value",
			Arg1:   "0",
		})
	}

	// Регистрация any
	if p.TryDepth > 0 && decl.Type == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "try_register",
			Result: decl.Name,
			Arg1:   "2",
		})
	}

	if decl.Expr != nil {
		exprType := p.getExprType(decl.Expr, irFn)

		// null (void) → SK_NULL_<decl.Type>
		if exprType == "void" {
			nullMacro := "SK_NULL_" + strings.TrimPrefix(cType, "sk_")
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   nullMacro,
			})
			return
		}

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
				irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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

func (p *Pipeline) processAssign(assign *front.Assign, irFn *IRFunction) {
	if assign.Expr == nil {
		return
	}

	exprType := p.getExprType(assign.Expr, irFn)

	// Определяем тип переменной
	varType := ""
	for _, local := range irFn.Locals {
		parts := strings.Fields(local)
		if len(parts) >= 2 && parts[len(parts)-1] == assign.Name {
			switch parts[0] {
			case "sk_int":
				varType = "int"
			case "sk_string":
				varType = "string"
			case "sk_float":
				varType = "float"
			case "sk_double":
				varType = "double"
			case "sk_bool":
				varType = "bool"
			case "sk_arr":
				varType = "arr"
			case "sk_any":
				varType = "any"
			}
			break
		}
	}

	// null (void) → SK_NULL_<varType>
	if exprType == "void" {
		cType := p.typeToC(varType)
		nullMacro := "SK_NULL_" + strings.TrimPrefix(cType, "sk_")
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: assign.Name,
			Arg1:   nullMacro,
		})
		return
	}

	exprResult := p.processExpression(assign.Expr, irFn)

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
			irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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

	if bin.Op == "+" && isArrayType(leftType) {
		// Проверяем, что слева идентификатор (имя переменной)
		if ident, ok := bin.Left.(*front.Ident); ok {
			return p.processArrayAddName(ident.Name, bin.Right, irFn)
		}
	}

	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	// Логические операции
	if bin.Op == "&&" || bin.Op == "||" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		leftVal := left
		rightVal := right
		if leftType == "any" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_bool "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "any_to_bool",
				Arg2:       left,
				ReturnType: "sk_bool",
			})
			leftVal = tmp
		}
		if rightType == "any" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_bool "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "any_to_bool",
				Arg2:       right,
				ReturnType: "sk_bool",
			})
			rightVal = tmp
		}
		fnName := "sk_bool_and"
		if bin.Op == "||" {
			fnName = "sk_bool_or"
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       leftVal + ", " + rightVal,
			ReturnType: "sk_bool",
		})
		return result
	}

	// Конкатенация строк
	if bin.Op == "+" && (leftType == "string" || rightType == "string" || leftType == "any" || rightType == "any") {
		leftVal := left
		leftStrType := leftType
		if leftType == "any" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "any_to_string",
				Arg2:       left,
				ReturnType: "sk_string",
			})
			leftVal = tmp
			leftStrType = "string"
		} else if leftType != "string" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			fnName := ""
			switch leftType {
			case "int":
				fnName = "sk_int_to_string"
			case "bool":
				fnName = "sk_bool_to_string"
			case "float":
				fnName = "sk_float_to_string"
			case "double":
				fnName = "sk_double_to_string"
			}
			if fnName != "" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tmp,
					Arg1:       fnName,
					Arg2:       left,
					ReturnType: "sk_string",
				})
				leftVal = tmp
				leftStrType = "string"
			}
		}
		_ = leftStrType

		rightVal := right
		if rightType == "any" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "any_to_string",
				Arg2:       right,
				ReturnType: "sk_string",
			})
			rightVal = tmp
		} else if rightType != "string" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			fnName := ""
			switch rightType {
			case "int":
				fnName = "sk_int_to_string"
			case "bool":
				fnName = "sk_bool_to_string"
			case "float":
				fnName = "sk_float_to_string"
			case "double":
				fnName = "sk_double_to_string"
			}
			if fnName != "" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tmp,
					Arg1:       fnName,
					Arg2:       right,
					ReturnType: "sk_string",
				})
				rightVal = tmp
			}
		}

		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_string "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_concat",
			Arg2:       leftVal + ", " + rightVal,
			ReturnType: "sk_string",
		})
		return result
	}

	// Арифметика/сравнения — распаковываем any если надо
	leftVal := left
	leftT := leftType
	if leftType == "any" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "any_to_int",
			Arg2:       left,
			ReturnType: "sk_int",
		})
		leftVal = tmp
		leftT = "int"
	}
	rightVal := right
	rightT := rightType
	if rightType == "any" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "any_to_int",
			Arg2:       right,
			ReturnType: "sk_int",
		})
		rightVal = tmp
		rightT = "int"
	}

	// Определяем тип результата
	resultT := leftT
	if resultT != rightT {
		if leftT == "double" || rightT == "double" {
			resultT = "double"
		} else if leftT == "float" || rightT == "float" {
			resultT = "float"
		} else {
			resultT = "int"
		}
	}

	// Сравнения
	if bin.Op == "<" || bin.Op == ">" || bin.Op == "==" || bin.Op == "!=" || bin.Op == "<=" || bin.Op == ">=" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		prefix := "sk_" + resultT
		suffix := ""
		switch bin.Op {
		case "<":
			suffix = "_lt"
		case ">":
			suffix = "_gt"
		case "==":
			suffix = "_eq"
		case "!=":
			suffix = "_ne"
		case "<=":
			suffix = "_le"
		case ">=":
			suffix = "_ge"
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       prefix + suffix,
			Arg2:       leftVal + ", " + rightVal,
			ReturnType: "sk_bool",
		})
		return result
	}

	// Арифметика
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_"+resultT+" "+result)
	fnName := ""
	if resultT == "int" {
		switch bin.Op {
		case "+":
			fnName = "sk_int_add"
		case "-":
			fnName = "sk_int_sub"
		case "*":
			fnName = "sk_int_mul"
		case "/":
			fnName = "sk_int_div"
		case "%":
			fnName = "sk_int_mod"
		}
	} else if resultT == "float" {
		switch bin.Op {
		case "+":
			fnName = "sk_float_add"
		case "-":
			fnName = "sk_float_sub"
		case "*":
			fnName = "sk_float_mul"
		case "/":
			fnName = "sk_float_div"
		}
	} else if resultT == "double" {
		switch bin.Op {
		case "+":
			fnName = "sk_double_add"
		case "-":
			fnName = "sk_double_sub"
		case "*":
			fnName = "sk_double_mul"
		case "/":
			fnName = "sk_double_div"
		}
	}
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       fnName,
		Arg2:       leftVal + ", " + rightVal,
		ReturnType: "sk_" + resultT,
	})

	return result
}

func (p *Pipeline) processExpression(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		if strings.Contains(n.Value, ".") {
			return fmt.Sprintf("sk_double_new(%s)", n.Value)
		}
		return fmt.Sprintf("sk_int_new(%s)", n.Value)
	case *front.NullLiteral:
		return "SK_NULL_int"
	case *front.String:
		return fmt.Sprintf("sk_string_new(%q)", n.Value)
	case *front.Ident:
		if n.Name == "true" {
			return "sk_bool_new(1)"
		}
		if n.Name == "false" {
			return "sk_bool_new(0)"
		}
		return n.Name
	case *front.ErrorInstance:
		return p.processErrorInstance(n, irFn)
	case *front.FieldAccess:
		return p.processFieldAccess(n, irFn)
	case *front.BinaryExpr:
		return p.processBinary(n, irFn)
	case *front.TernaryExpr:
		return p.processTernary(n, irFn)
	case *front.TypeOf:
		return p.processTypeOf(n, irFn)
	case *front.CallExpr:
		return p.processCallExpr(n, irFn)
	case *front.UnaryExpr:
		return p.processUnary(n, irFn)
	case *front.ArrayLiteral:
		return p.processArrayLiteralTyped(n, "", irFn)
	case *front.ArrayIndex:
		return p.processArrayIndex(n, irFn)
	case *front.ArrayLength:
		return p.processArrayLength(n, irFn)
	case *front.RangeExpr:
		startVal := p.getConstantInt(n.Start, irFn)
		endVal := p.getConstantInt(n.End, irFn)
		if startVal >= 0 && endVal >= 0 {
			lit := &front.ArrayLiteral{Elements: []front.Node{}}
			if startVal <= endVal {
				for i := startVal; i <= endVal; i++ {
					lit.Elements = append(lit.Elements, &front.Number{Value: strconv.Itoa(i)})
				}
			} else {
				for i := startVal; i >= endVal; i-- {
					lit.Elements = append(lit.Elements, &front.Number{Value: strconv.Itoa(i)})
				}
			}
			return p.processArrayLiteralTyped(lit, "", irFn)
		}
		return p.processRange(n, irFn)
	case *front.CallRangeExpr:
		return p.processCallRange(n, irFn)
	case *front.ArrayAdd:
		return p.processArrayAdd(n, irFn)
	default:
		return "SK_NULL_int"
	}
}

func (p *Pipeline) processReturn(ret *front.ReturnStmt, irFn *IRFunction) {
	exprResult := ""
	if ret.Expr != nil {
		exprType := p.getExprType(ret.Expr, irFn)

		// null (void) → SK_NULL_<returnType>
		if exprType == "void" {
			cType := p.typeToC(irFn.ReturnType)
			exprResult = "SK_NULL_" + strings.TrimPrefix(cType, "sk_")
		} else {
			exprResult = p.processExpression(ret.Expr, irFn)
		}
	}

	for i := len(p.TryFrames) - 1; i >= 0; i-- {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "try_pop",
			Arg1: p.TryFrames[i],
		})
	}

	if exprResult != "" {
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
				args = append(args, "SK_NULL_int")
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
	// Если Receiver задан — добавляем его в Args
	args := call.Args
	if call.Receiver != "" {
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

	// Builtins
	switch simpleName {
	case "to_int":
		return p.processToInt(&front.CallExpr{Name: "to_int", Args: args}, irFn)
	case "to_float":
		return p.processToFloat(&front.CallExpr{Name: "to_float", Args: args}, irFn)
	case "to_double":
		return p.processToDouble(&front.CallExpr{Name: "to_double", Args: args}, irFn)
	case "to_string":
		return p.processToString(&front.CallExpr{Name: "to_string", Args: args}, irFn)
	case "to_bool":
		return p.processToBool(&front.CallExpr{Name: "to_bool", Args: args}, irFn)
	case "to_arr":
		return p.processToArr(&front.CallExpr{Name: "to_arr", Args: args}, irFn)
	}

	// Ищем целевую функцию
	targetFunc := p.findFunction(simpleName)

	argsStr := []string{}

	if targetFunc != nil {
		argIndex := 0
		for _, param := range targetFunc.Params {
			var argExpr front.Node

			if argIndex < len(args) {
				argExpr = args[argIndex]
				argIndex++
			} else if param.DefaultValue != nil {
				argExpr = param.DefaultValue
			} else {
				argsStr = append(argsStr, "SK_NULL_int")
				continue
			}

			argsStr = append(argsStr, p.prepareArg(argExpr, param.Type, irFn))
		}
	} else {
		for _, arg := range args {
			argsStr = append(argsStr, p.processExpression(arg, irFn))
		}
	}

	argsJoined := strings.Join(argsStr, ", ")

	// Определяем тип возврата
	returnType := "sk_string"
	for _, fn := range p.Program.Functions {
		if fn.Name == simpleName {
			returnType = p.typeToC(fn.ReturnType)
			break
		}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, returnType+" "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       simpleName,
		Arg2:       argsJoined,
		ReturnType: returnType,
	})

	return result
}

// prepareArg подготавливает аргумент: оборачивает в any или распаковывает из any
func (p *Pipeline) prepareArg(argExpr front.Node, paramType string, irFn *IRFunction) string {
	argType := p.getExprType(argExpr, irFn)
	argValue := p.processExpression(argExpr, irFn)

	if argType == "void" && paramType == "any" {
		// Это должно отлавливаться семантикой, но на всякий случай
		return "any_null()"
	}

	if paramType == "any" {
		if argType == "any" {
			return argValue
		}
		wrapper := p.getAnyWrapperByType(argType)
		tempVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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
		irFn.Locals = append(irFn.Locals, p.typeToC(paramType)+" "+tempVar)
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
		irFn.Locals = append(irFn.Locals, "sk_int "+tempVar)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tempVar,
			Arg1:       "any_to_int",
			Arg2:       value,
			ReturnType: "sk_int",
		})
		value = tempVar
		valueType = "int"
	}

	endLabel := p.newLabel()

	for _, branch := range caseStmt.Branches {
		patternType := p.getExprType(branch.Pattern, irFn)
		pattern := p.processExpression(branch.Pattern, irFn)

		if patternType == "any" {
			tempVar := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_int "+tempVar)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempVar,
				Arg1:       "any_to_int",
				Arg2:       pattern,
				ReturnType: "sk_int",
			})
			pattern = tempVar
		}

		branchLabel := p.newLabel()
		nextLabel := p.newLabel()

		// Сравнение через функции
		cmpVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+cmpVar)
		eqFn := "sk_" + valueType + "_eq"
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     cmpVar,
			Arg1:       eqFn,
			Arg2:       value + ", " + pattern,
			ReturnType: "sk_bool",
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "if",
			Result: cmpVar,
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
		irFn.Locals = append(irFn.Locals, "sk_any "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "array_get_any",
			Result:     result,
			Arg1:       idx.Name,
			Arg2:       index,
			ReturnType: "sk_any",
		})
	} else if isArrayType(elemType) {
		irFn.Locals = append(irFn.Locals, "sk_arr "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "array_get_typed",
			Result:     result,
			Arg1:       idx.Name,
			Arg2:       index,
			ReturnType: "sk_arr",
		})
	} else {
		cType := p.typeToC(elemType)
		irFn.Locals = append(irFn.Locals, cType+" "+result)
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
	irFn.Locals = append(irFn.Locals, "sk_int "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "array_len",
		Result:     result,
		Arg1:       length.Name,
		ReturnType: "sk_int",
	})
	return result
}

func (p *Pipeline) processArrayAdd(add *front.ArrayAdd, irFn *IRFunction) string {
	elemType := p.getExprType(add.Elem, irFn)
	elemVal := p.processExpression(add.Elem, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	copyTmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_array* "+copyTmp)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     copyTmp,
		Arg1:       "sk_array_copy",
		Arg2:       add.Name + ".value",
		ReturnType: "sk_array*",
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       copyTmp,
		ReturnType: "sk_arr",
	})

	if elemType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "call",
			Arg1: "sk_array_push",
			Arg2: result + ".value, &" + elemVal,
		})
	} else {
		wrapper := p.getAnyWrapperByType(elemType)
		tempVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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
			Arg2: result + ".value, &" + tempVar,
		})
	}

	return result
}

func (p *Pipeline) processArrayAddName(name string, elemExpr front.Node, irFn *IRFunction) string {
	elemType := p.getExprType(elemExpr, irFn)
	elemVal := p.processExpression(elemExpr, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	copyTmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_array* "+copyTmp)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     copyTmp,
		Arg1:       "sk_array_copy",
		Arg2:       name + ".value",
		ReturnType: "sk_array*",
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       copyTmp,
		ReturnType: "sk_arr",
	})

	if elemType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "call",
			Arg1: "sk_array_push",
			Arg2: result + ".value, &" + elemVal,
		})
	} else {
		wrapper := p.getAnyWrapperByType(elemType)
		tempVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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
			Arg2: result + ".value, &" + tempVar,
		})
	}

	return result
}

// ============ Встроенные функции ============

func (p *Pipeline) processToInt(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_int",
		})
		return result
	}

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_int "+result)

	switch argType {
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_bool_to_int",
			Arg2:       arg,
			ReturnType: "sk_int",
		})
	case "float":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_float_to_int",
			Arg2:       arg,
			ReturnType: "sk_int",
		})
	case "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_double_to_int",
			Arg2:       arg,
			ReturnType: "sk_int",
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_to_int",
			Arg2:       arg,
			ReturnType: "sk_int",
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_int",
			Arg2:       arg,
			ReturnType: "sk_int",
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
		irFn.Locals = append(irFn.Locals, "sk_float "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_float",
		})
		return result
	}

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_float "+result)

	switch argType {
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_float_new(%s.value ? 1.0f : 0.0f)", arg),
		})
	case "int":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_float_new((float)%s.value)", arg),
		})
	case "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_float_new((float)%s.value)", arg),
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_to_float",
			Arg2:       arg,
			ReturnType: "sk_float",
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_float",
			Arg2:       arg,
			ReturnType: "sk_float",
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
		irFn.Locals = append(irFn.Locals, "sk_double "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_double",
		})
		return result
	}

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_double "+result)

	switch argType {
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_double_new(%s.value ? 1.0 : 0.0)", arg),
		})
	case "int":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_double_new((double)%s.value)", arg),
		})
	case "float":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_double_new((double)%s.value)", arg),
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_to_double",
			Arg2:       arg,
			ReturnType: "sk_double",
		})
	case "any":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "any_to_double",
			Arg2:       arg,
			ReturnType: "sk_double",
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
		irFn.Locals = append(irFn.Locals, "sk_string "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   `sk_string_new("")`,
		})
		return result
	}

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_string "+result)

	switch argType {
	case "int":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_to_string",
			Arg2:       arg,
			ReturnType: "sk_string",
		})
	case "bool":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_bool_to_string",
			Arg2:       arg,
			ReturnType: "sk_string",
		})
	case "float":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_float_to_string",
			Arg2:       arg,
			ReturnType: "sk_string",
		})
	case "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_double_to_string",
			Arg2:       arg,
			ReturnType: "sk_string",
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
			Arg2:       arg + ".value",
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
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_to_string",
			Arg2:       arg,
			ReturnType: "sk_string",
		})
	}

	return result
}

func (p *Pipeline) processToBool(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_bool",
		})
		return result
	}

	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_bool "+result)

	switch argType {
	case "int":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_to_bool",
			Arg2:       arg,
			ReturnType: "sk_bool",
		})
	case "float":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_float_to_bool",
			Arg2:       arg,
			ReturnType: "sk_bool",
		})
	case "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_double_to_bool",
			Arg2:       arg,
			ReturnType: "sk_bool",
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_to_bool",
			Arg2:       arg,
			ReturnType: "sk_bool",
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
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	tmpArr := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_array* "+tmpArr)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     tmpArr,
		Arg1:       "sk_array_new",
		Arg2:       "sizeof(sk_any), 5",
		ReturnType: "sk_array*",
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       tmpArr,
		ReturnType: "sk_arr",
	})

	for _, arg := range call.Args {
		argType := p.getExprType(arg, irFn)
		val := p.processExpression(arg, irFn)

		if argType == "any" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:   "call",
				Arg1: "sk_array_push_any",
				Arg2: result + ".value, " + val,
			})
		} else {
			wrapper := p.getAnyWrapperByType(argType)
			tempVar := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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
				Arg2: result + ".value, " + tempVar,
			})
		}
	}

	return result
}

// ============ Система типов ============

func (p *Pipeline) getExprType(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		if strings.Contains(n.Value, ".") {
			return "double"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.NullLiteral:
		return "void"
	case *front.ErrorInstance:
		return n.TypeName
	case *front.FieldAccess:
		if n.Field == "msg" {
			return "string"
		}
		return "int"
	case *front.Ident:
		if n.Name == "true" || n.Name == "false" {
			return "bool"
		}
		for _, param := range irFn.Params {
			if param.Name == n.Name {
				return param.Type
			}
		}
		for _, local := range irFn.Locals {
			parts := strings.Fields(local)
			if len(parts) >= 2 {
				name := parts[len(parts)-1]
				if name == n.Name {
					switch parts[0] {
					case "sk_int":
						return "int"
					case "sk_string":
						return "string"
					case "sk_float":
						return "float"
					case "sk_double":
						return "double"
					case "sk_bool":
						return "bool"
					case "sk_arr":
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
		if n.Op == "+" && (leftType == "string" || rightType == "string" || leftType == "any" || rightType == "any") {
			return "string"
		}
		if n.Op == "<" || n.Op == ">" || n.Op == "==" || n.Op == "!=" || n.Op == "<=" || n.Op == ">=" {
			return "bool"
		}
		if n.Op == "&&" || n.Op == "||" {
			return "bool"
		}
		if leftType == "double" || rightType == "double" {
			return "double"
		}
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		return "int"
	case *front.UnaryExpr:
		if n.Op == "$" {
			return "string"
		}
		if n.Op == "!" {
			return "bool"
		}
		return p.getExprType(n.Expr, irFn)
	case *front.CallExpr:
		simpleName := n.Name
		if strings.Contains(simpleName, ".") {
			parts := strings.Split(simpleName, ".")
			simpleName = parts[len(parts)-1]
		}

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
			return "arr"
		}

		for _, fn := range p.Program.Functions {
			if fn.Name == simpleName {
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
				if isArrayType(elemType) {
					return elemType
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
	if isArrayType(t) {
		return "any_arr"
	}
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
	default:
		return "any_int"
	}
}

func (p *Pipeline) getAnyGetterByType(t string) string {
	if isArrayType(t) {
		return "any_to_arr"
	}
	switch t {
	case "int":
		return "any_to_int"
	case "string":
		return "any_to_string"
	case "float":
		return "any_to_float"
	case "double":
		return "any_to_double"
	case "bool":
		return "any_to_bool"
	default:
		return "any_to_int"
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

func parseArrayElemType(elemType string) string {
	if elemType == "" {
		return ""
	}
	if !strings.HasPrefix(elemType, "arr[") {
		return elemType
	}
	inner := elemType[4 : len(elemType)-1]
	return inner
}

func isArrayType(t string) bool {
	return t == "arr" || strings.HasPrefix(t, "arr[")
}

func (p *Pipeline) processExpressionTyped(expr front.Node, expectedType string, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.ArrayLiteral:
		return p.processArrayLiteralTyped(n, expectedType, irFn)
	case *front.RangeExpr:
		startVal := p.getConstantInt(n.Start, irFn)
		endVal := p.getConstantInt(n.End, irFn)
		if startVal >= 0 && endVal >= 0 {
			lit := &front.ArrayLiteral{Elements: []front.Node{}}
			if startVal <= endVal {
				for i := startVal; i <= endVal; i++ {
					lit.Elements = append(lit.Elements, &front.Number{Value: strconv.Itoa(i)})
				}
			} else {
				for i := startVal; i >= endVal; i-- {
					lit.Elements = append(lit.Elements, &front.Number{Value: strconv.Itoa(i)})
				}
			}
			return p.processArrayLiteralTyped(lit, expectedType, irFn)
		}
		return p.processRange(n, irFn)
	default:
		return p.processExpression(expr, irFn)
	}
}

func (p *Pipeline) processArrayLiteralTyped(lit *front.ArrayLiteral, expectedElemType string, irFn *IRFunction) string {
	expandedElements := []front.Node{}
	for _, elem := range lit.Elements {
		if rangeExpr, ok := elem.(*front.RangeExpr); ok {
			startVal := p.getConstantInt(rangeExpr.Start, irFn)
			endVal := p.getConstantInt(rangeExpr.End, irFn)
			if startVal >= 0 && endVal >= 0 {
				if startVal <= endVal {
					for i := startVal; i <= endVal; i++ {
						expandedElements = append(expandedElements, &front.Number{Value: strconv.Itoa(i)})
					}
				} else {
					for i := startVal; i >= endVal; i-- {
						expandedElements = append(expandedElements, &front.Number{Value: strconv.Itoa(i)})
					}
				}
			} else {
				expandedElements = append(expandedElements, elem)
			}
		} else {
			expandedElements = append(expandedElements, elem)
		}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	tmpArr := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_array* "+tmpArr)

	var elemType int
	var elemSize string

	if isArrayType(expectedElemType) {
		elemType = 6
		elemSize = "sizeof(sk_arr)"
	} else {
		switch expectedElemType {
		case "int":
			elemType = 0
			elemSize = "sizeof(sk_int)"
		case "string":
			elemType = 1
			elemSize = "sizeof(sk_string)"
		case "float":
			elemType = 2
			elemSize = "sizeof(sk_float)"
		case "double":
			elemType = 3
			elemSize = "sizeof(sk_double)"
		case "bool":
			elemType = 4
			elemSize = "sizeof(sk_bool)"
		default:
			elemType = 5
			elemSize = "sizeof(sk_any)"
		}
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     tmpArr,
		Arg1:       "sk_array_new",
		Arg2:       fmt.Sprintf("%s, %d", elemSize, elemType),
		ReturnType: "sk_array*",
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       tmpArr,
		ReturnType: "sk_arr",
	})

	innerElemType := parseArrayElemType(expectedElemType)

	for _, elem := range expandedElements {
		val := p.processExpressionTyped(elem, innerElemType, irFn)

		if isArrayType(expectedElemType) {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:   "call",
				Arg1: "sk_array_push",
				Arg2: result + ".value, &" + val,
			})
		} else if expectedElemType == "" || expectedElemType == "any" {
			elemTypeName := p.getExprType(elem, irFn)
			if elemTypeName == "any" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   "call",
					Arg1: "sk_array_push_any",
					Arg2: result + ".value, " + val,
				})
			} else {
				wrapper := p.getAnyWrapperByType(elemTypeName)
				tempVar := p.newTemp()
				irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
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
					Arg2: result + ".value, " + tempVar,
				})
			}
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:   "call",
				Arg1: "sk_array_push",
				Arg2: result + ".value, &" + val,
			})
		}
	}

	return result
}

func (p *Pipeline) processRange(r *front.RangeExpr, irFn *IRFunction) string {
	start := p.processExpression(r.Start, irFn)
	end := p.processExpression(r.End, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	tmpArr := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_array* "+tmpArr)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     tmpArr,
		Arg1:       "sk_range_new",
		Arg2:       fmt.Sprintf("%s.value, %s.value", start, end),
		ReturnType: "sk_array*",
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       tmpArr,
		ReturnType: "sk_arr",
	})

	return result
}

func (p *Pipeline) processCallRange(call *front.CallRangeExpr, irFn *IRFunction) string {
	targetFunc := p.findFunction(call.Name)
	if targetFunc == nil {
		return "SK_NULL_int"
	}

	startVal := p.getConstantInt(call.Range.Start, irFn)
	endVal := p.getConstantInt(call.Range.End, irFn)

	if startVal < 0 || endVal < 0 {
		return "SK_NULL_int"
	}

	args := []string{}
	for i := startVal; i <= endVal; i++ {
		args = append(args, fmt.Sprintf("sk_int_new(%d)", i))
	}
	for _, extra := range call.Extra {
		args = append(args, p.processExpression(extra, irFn))
	}

	argsStr := strings.Join(args, ", ")

	funcName := call.Name
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		funcName = parts[len(parts)-1]
	}

	result := p.newTemp()
	returnType := p.typeToC(targetFunc.ReturnType)
	irFn.Locals = append(irFn.Locals, returnType+" "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       funcName,
		Arg2:       argsStr,
		ReturnType: returnType,
	})

	return result
}

func (p *Pipeline) getConstantInt(expr front.Node, irFn *IRFunction) int {
	switch n := expr.(type) {
	case *front.Number:
		if v, err := strconv.Atoi(n.Value); err == nil {
			return v
		}
	case *front.UnaryExpr:
		if n.Op == "-" {
			inner := p.getConstantInt(n.Expr, irFn)
			if inner >= 0 {
				return -inner
			}
		}
	}
	return -1
}

func (p *Pipeline) processTernary(t *front.TernaryExpr, irFn *IRFunction) string {
	cond := p.processExpression(t.Condition, irFn)

	thenType := p.getExprType(t.Then, irFn)
	elseType := p.getExprType(t.Else, irFn)

	resultType := thenType
	if resultType == "" {
		resultType = elseType
	}
	if resultType == "" {
		resultType = "int"
	}

	thenVal := p.processExpression(t.Then, irFn)
	elseVal := p.processExpression(t.Else, irFn)

	result := p.newTemp()
	cType := p.typeToC(resultType)
	irFn.Locals = append(irFn.Locals, cType+" "+result)

	// Для cond используем .value если это bool
	condExpr := cond
	if p.getExprType(t.Condition, irFn) == "bool" {
		condExpr = cond + ".value"
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "ternary",
		Result:     result,
		Arg1:       condExpr,
		Arg2:       thenVal,
		Arg3:       elseVal,
		ReturnType: cType,
	})

	return result
}
