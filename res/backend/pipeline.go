package backend

import (
	"fmt"
	"skrp/res/debug"
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
	// Global includeC
	for _, code := range p.Program.GlobalIncludeC {
		p.IR.InlineC += code + "\n\n"
	}

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
			Module: decl.Module,
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
		CName:          computeCName(fn.Name),
		ReturnType:     fn.ReturnType,
		IsExport:       fn.IsExport,
		File:           fn.File,
		Params:         []IRParam{},
		Locals:         []string{},
		Instructions:   []IRInstruction{},
		ArrayElemTypes: make(map[string]string),
		VarTypes:       make(map[string]string),
		DictKeyTypes:   make(map[string]map[string]string),
	}

	for _, param := range fn.Params {
		irFn.Params = append(irFn.Params, IRParam{
			Name: param.Name,
			Type: param.Type,
		})
		irFn.VarTypes[param.Name] = param.Type

		if isArrayType(param.Type) {
			elemType := parseArrayElemType(param.Type)
			if elemType == "" {
				elemType = "any"
			}
			irFn.ArrayElemTypes[param.Name] = elemType
		}
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
	case *front.IknowIdoBlock:
		if n.Body != nil {
			p.processBlock(n.Body, irFn)
		}
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
	case *front.ForInStmt:
		p.processForIn(n, irFn)
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
	if isUnionTypeP(typ) {
		return "sk_any"
	}
	if isArrayType(typ) {
		return "sk_arr"
	}
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
		return "sk_char"
	case "void":
		return "void"
	case "arr":
		return "sk_arr"
	case "dict":
		return "sk_dict_ref"
	case "any":
		return "sk_any"
	case "null":
		return "void*"
	default:
		return "sk_int"
	}
}

// isUnionTypeP — то же, что isUnionType в семантике, но локально
func isUnionTypeP(t string) bool {
	return strings.HasPrefix(t, "T<") && strings.HasSuffix(t, ">")
}

func (p *Pipeline) processFieldAccess(fa *front.FieldAccess, irFn *IRFunction) string {
	// === dict ===
	if irFn.VarTypes != nil {
		if objType, ok := irFn.VarTypes[fa.Object]; ok && objType == "dict" {
			return p.processDictAccess(fa.Object, fa.Field, irFn)
		}
	}

	fieldType := "int"

	if irFn.VarTypes != nil {
		if objType, ok := irFn.VarTypes[fa.Object]; ok {
			if objType == "Error" {
				if fa.Field == "msg" {
					fieldType = "string"
				}
			} else {
				allFields := p.collectErrorFields(objType)
				if ft, exists := allFields[fa.Field]; exists {
					fieldType = ft
				}
			}
		}
	}

	if fieldType == "int" && fa.Field == "msg" {
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

func (p *Pipeline) collectErrorFields(typeName string) map[string]string {
	fields := make(map[string]string)

	// Разрезаем возможный "strings.Foo" или "strings.Parent.Child"
	modulePrefix := ""
	simpleName := typeName
	if idx := strings.Index(typeName, "."); idx >= 0 {
		parts := strings.SplitN(typeName, ".", 2)
		modulePrefix = parts[0]
		simpleName = parts[1]
		if idx2 := strings.LastIndex(simpleName, "."); idx2 >= 0 {
			simpleName = simpleName[idx2+1:]
		}
	}

	var decl *front.ErrorDecl
	for _, d := range p.Program.ErrorDecls {
		if d.Name == simpleName {
			if modulePrefix != "" && d.Module != modulePrefix {
				continue
			}
			decl = d
			break
		}
	}
	if decl == nil {
		return fields
	}

	if decl.Parent != "" && decl.Parent != "Error" {
		parentFields := p.collectErrorFields(decl.Parent)
		for k, v := range parentFields {
			fields[k] = v
		}
	}
	if decl.Parent == "Error" || decl.Parent == "" {
		fields["msg"] = "string"
	}

	for _, f := range decl.Fields {
		fields[f.Name] = f.Type
	}

	return fields
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
			irFn.Locals = append(irFn.Locals, "sk_bool "+typeCheckVar) // было "int"

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "error_type_match",
				Result:     typeCheckVar,
				Arg1:       errorVar,
				Arg2:       fmt.Sprintf(`"%s"`, p.fullErrorTypePath(clause.TypeName)),
				ReturnType: "sk_bool", // было "int"
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
			cType := typeName + "*"
			if typeName == "Error" {
				cType = "SkError*"
			}
			irFn.Locals = append(irFn.Locals, cType+" "+clause.VarName)
			if irFn.VarTypes != nil {
				irFn.VarTypes[clause.VarName] = typeName
			}
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
			Arg2:   fmt.Sprintf("%q", irFn.File),
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

	// Разбираем возможный "strings.FooError" или "strings.Parent.Child"
	modulePrefix := ""
	simpleName := typeName
	if idx := strings.Index(typeName, "."); idx >= 0 {
		parts := strings.SplitN(typeName, ".", 2)
		modulePrefix = parts[0]
		simpleName = parts[1]
		// Если после модуля ещё цепочка "Parent.Child" — берём последний
		if idx2 := strings.LastIndex(simpleName, "."); idx2 >= 0 {
			simpleName = simpleName[idx2+1:]
		}
	}

	// Ищем decl
	var decl *front.ErrorDecl
	for _, d := range p.Program.ErrorDecls {
		if d.Name == simpleName {
			if modulePrefix != "" && d.Module != modulePrefix {
				continue
			}
			decl = d
			break
		}
	}
	if decl == nil {
		// Не нашли — конкатенируем как есть
		if modulePrefix != "" {
			return modulePrefix + ".Error." + simpleName
		}
		return "Error." + simpleName
	}

	// Собираем цепочку от корня
	path := []string{simpleName}
	parent := decl.Parent
	for parent != "" && parent != "Error" {
		path = append([]string{parent}, path...)

		var parentDecl *front.ErrorDecl
		for _, d := range p.Program.ErrorDecls {
			if d.Name == parent && d.Module == decl.Module {
				parentDecl = d
				break
			}
		}
		if parentDecl == nil {
			break
		}
		parent = parentDecl.Parent
	}

	// Формат: <module>.<ErrorChain>
	chain := "Error." + strings.Join(path, ".")
	if decl.Module != "" {
		return decl.Module + "." + chain
	}
	return chain
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

		// Union — используем sk_any_neg
		if isUnionTypeP(exprType) || exprType == "any" {
			irFn.Locals = append(irFn.Locals, "sk_any "+result)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_any_neg",
				Arg2:       expr,
				ReturnType: "sk_any",
			})
			return result
		}

		cType := p.typeToC(exprType)
		// char → sk_int (потому что char_neg возвращает sk_int)
		if exprType == "char" {
			cType = "sk_int"
		}
		irFn.Locals = append(irFn.Locals, cType+" "+result)

		fnName := ""
		switch exprType {
		case "int":
			fnName = "sk_int_neg"
		case "float":
			fnName = "sk_float_neg"
		case "double":
			fnName = "sk_double_neg"
		case "char":
			fnName = "sk_char_neg"
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
		} else if exprType == "char" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_char_to_string",
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
	// === Union T<...> ===
	if isUnionTypeP(decl.Type) {
		irFn.Locals = append(irFn.Locals, "sk_any "+decl.Name)
		if irFn.VarTypes != nil {
			irFn.VarTypes[decl.Name] = decl.Type
		}

		if decl.Expr != nil {
			exprType := p.getExprType(decl.Expr, irFn)
			exprResult := p.processExpression(decl.Expr, irFn)

			if exprType == "any" || isUnionTypeP(exprType) {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: decl.Name,
					Arg1:   exprResult,
				})
			} else {
				wrapper := p.getAnyWrapperByType(exprType)
				if wrapper == "" {
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:     "=",
						Result: decl.Name,
						Arg1:   exprResult,
					})
				} else {
					tempVar := p.newTemp()
					irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:         "call",
						Result:     tempVar,
						Arg1:       wrapper,
						Arg2:       exprResult,
						ReturnType: "sk_any",
					})
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:     "=",
						Result: decl.Name,
						Arg1:   tempVar,
					})
				}
			}
		}
		return
	}

	// === dict ===
	if decl.Type == "dict" {
		irFn.Locals = append(irFn.Locals, "sk_dict_ref "+decl.Name)
		if irFn.VarTypes != nil {
			irFn.VarTypes[decl.Name] = "dict"
		}
		if irFn.DictKeyTypes == nil {
			irFn.DictKeyTypes = make(map[string]map[string]string)
		}
		irFn.DictKeyTypes[decl.Name] = make(map[string]string)

		// Заполняем типы ключей из литерала
		if decl.Expr != nil {
			if lit, ok := decl.Expr.(*front.DictLiteral); ok {
				for _, elem := range lit.Elements {
					keyType := p.getExprType(elem.Value, irFn)
					if keyType == "" {
						keyType = "any"
					}
					irFn.DictKeyTypes[decl.Name][elem.Key] = keyType
				}
			}
		}

		if decl.Expr != nil {
			exprType := p.getExprType(decl.Expr, irFn)
			if exprType == "void" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: decl.Name,
					Arg1:   "SK_NULL_dict",
				})
				return
			}
			exprResult := p.processExpression(decl.Expr, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   exprResult,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   "SK_NULL_dict",
			})
		}
		return
	}

	if decl.IsArray {
		irFn.Locals = append(irFn.Locals, "sk_arr "+decl.Name)

		if irFn.VarTypes != nil {
			elemType := decl.ElemType
			if elemType == "" {
				elemType = "any"
			}
			irFn.VarTypes[decl.Name] = "arr[" + elemType + "]"
		}

		if irFn.ArrayElemTypes == nil {
			irFn.ArrayElemTypes = make(map[string]string)
		}
		irFn.ArrayElemTypes[decl.Name] = decl.ElemType

		if p.TryDepth > 0 {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "try_register",
				Result: decl.Name,
				Arg1:   "1",
			})
		}

		if decl.Expr != nil {
			exprType := p.getExprType(decl.Expr, irFn)

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
					case "char":
						elemType = 7
						elemSize = "sizeof(sk_char)"
					default:
						elemType = 5
						elemSize = "sizeof(sk_any)"
					}
				}
			}
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     decl.Name,
				Arg1:       "sk_arr_new",
				Arg2:       fmt.Sprintf("sk_array_new(%s, %d)", elemSize, elemType),
				ReturnType: "sk_arr",
			})
		}
		return
	}

	cType := p.typeToC(decl.Type)
	irFn.Locals = append(irFn.Locals, cType+" "+decl.Name)

	if irFn.VarTypes != nil {
		irFn.VarTypes[decl.Name] = decl.Type
	}

	if p.TryDepth > 0 && decl.Type == "string" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "try_register",
			Result: decl.Name + ".value",
			Arg1:   "0",
		})
	}

	if p.TryDepth > 0 && decl.Type == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "try_register",
			Result: decl.Name,
			Arg1:   "2",
		})
	}

	if decl.Expr != nil {
		exprType := p.getExprType(decl.Expr, irFn)

		if exprType == "void" {
			if decl.Type == "void" || decl.Type == "any" {
				return
			}
			nullMacro := "SK_NULL_" + strings.TrimPrefix(cType, "sk_")
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     "=",
				Result: decl.Name,
				Arg1:   nullMacro,
			})
			return
		}

		exprResult := p.processExpression(decl.Expr, irFn)

		if decl.Type == "int" && exprType == "char" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_char_to_int",
				Arg2:       exprResult,
				ReturnType: "sk_int",
			})
			exprResult = tmp
		} else if decl.Type == "char" && exprType == "int" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_char "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_int_to_char",
				Arg2:       exprResult,
				ReturnType: "sk_char",
			})
			exprResult = tmp
		}

		if decl.Type == "any" {
			if exprType == "any" || isUnionTypeP(exprType) {
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

	// === dict: d.key = value ===
	if assign.Field != "" {
		if irFn.VarTypes != nil {
			if varType, ok := irFn.VarTypes[assign.Name]; ok && varType == "dict" {
				valType := p.getExprType(assign.Expr, irFn)
				val := p.processExpression(assign.Expr, irFn)
				valAny := p.wrapToAny(valType, val, irFn)

				keyStr := p.newTemp()
				irFn.Locals = append(irFn.Locals, "sk_string "+keyStr)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: keyStr,
					Arg1:   fmt.Sprintf("sk_string_new(%q)", assign.Field),
				})
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   "call",
					Arg1: "sk_dict_set",
					Arg2: assign.Name + ".value, " + keyStr + ", " + valAny,
				})
				return
			}
		}
	}

	// === dict: d["key"] = value; arr: x[i] = value ===
	if assign.Index != nil {
		if irFn.VarTypes != nil {
			if varType, ok := irFn.VarTypes[assign.Name]; ok && varType == "dict" {
				key := p.processExpression(assign.Index, irFn)
				valType := p.getExprType(assign.Expr, irFn)
				val := p.processExpression(assign.Expr, irFn)
				valAny := p.wrapToAny(valType, val, irFn)

				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   "call",
					Arg1: "sk_dict_set",
					Arg2: assign.Name + ".value, " + key + ", " + valAny,
				})
				return
			}
		}
		p.processArrayAssign(assign, irFn)
		return
	}

	// === Union T<...> ===
	if irFn.VarTypes != nil {
		if varType, ok := irFn.VarTypes[assign.Name]; ok && isUnionTypeP(varType) {
			exprType := p.getExprType(assign.Expr, irFn)
			exprResult := p.processExpression(assign.Expr, irFn)

			if exprType == "any" || isUnionTypeP(exprType) {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: assign.Name,
					Arg1:   exprResult,
				})
			} else {
				wrapper := p.getAnyWrapperByType(exprType)
				if wrapper == "" {
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:     "=",
						Result: assign.Name,
						Arg1:   exprResult,
					})
				} else {
					tempVar := p.newTemp()
					irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:         "call",
						Result:     tempVar,
						Arg1:       wrapper,
						Arg2:       exprResult,
						ReturnType: "sk_any",
					})
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:     "=",
						Result: assign.Name,
						Arg1:   tempVar,
					})
				}
			}
			return
		}
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
			case "sk_char":
				varType = "char"
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
			case "sk_dict_ref":
				varType = "dict"
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

	// Неявные конверсии char ↔ int
	if varType == "int" && exprType == "char" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "sk_char_to_int",
			Arg2:       exprResult,
			ReturnType: "sk_int",
		})
		exprResult = tmp
	} else if varType == "char" && exprType == "int" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_char "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "sk_int_to_char",
			Arg2:       exprResult,
			ReturnType: "sk_char",
		})
		exprResult = tmp
	}

	if varType == "any" {
		if exprType == "any" || isUnionTypeP(exprType) {
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

// processArrayAssign — x[index] = value
func (p *Pipeline) processArrayAssign(assign *front.Assign, irFn *IRFunction) {
	elemType := ""
	if irFn.ArrayElemTypes != nil {
		elemType = irFn.ArrayElemTypes[assign.Name]
	}
	if elemType == "" {
		elemType = "any"
	}

	// Индекс
	index := p.processExpression(assign.Index, irFn)

	// Значение
	valueType := p.getExprType(assign.Expr, irFn)
	value := p.processExpression(assign.Expr, irFn)

	// Приводим к elemType
	if elemType == "any" {
		if valueType != "any" {
			wrapper := p.getAnyWrapperByType(valueType)
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_any "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       wrapper,
				Arg2:       value,
				ReturnType: "sk_any",
			})
			value = tmp
		}
	}

	// Определяем C-тип слота
	var cType string
	if elemType == "any" {
		cType = "sk_any"
	} else if isArrayType(elemType) {
		cType = "sk_arr"
	} else {
		cType = p.typeToC(elemType)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "array_set",
		Result:     assign.Name,
		Arg1:       index,
		Arg2:       value,
		ReturnType: cType,
	})
}

// processNullComparison обрабатывает сравнение с null (void).
// Генерирует: (x.__is_null == 1) или (x.__is_null == 0).
func (p *Pipeline) processNullComparison(bin *front.BinaryExpr, leftType, rightType string, irFn *IRFunction) string {
	// Определяем, какая сторона — null, а какая — значение
	var valueNode front.Node
	var valueType string
	var isLeftNull bool

	if leftType == "void" {
		valueNode = bin.Right
		valueType = rightType
		isLeftNull = true
	} else {
		valueNode = bin.Left
		valueType = leftType
		isLeftNull = false
	}
	_ = isLeftNull

	// Допустимы только == и !=
	if bin.Op != "==" && bin.Op != "!=" {
		// Для других операций с null — генерируем null
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_bool",
		})
		return result
	}

	valueVal := p.processExpression(valueNode, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_bool "+result)

	// Для any — используем any-проверку
	if valueType == "any" || isUnionTypeP(valueType) {
		// Сравниваем a.type == 6 (null)
		eqOp := "=="
		if bin.Op == "!=" {
			eqOp = "!="
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   fmt.Sprintf("sk_bool_new(%s.type %s 6)", valueVal, eqOp),
		})
		return result
	}

	// Для всех остальных типов — проверяем .__is_null
	var nullCheck string
	if bin.Op == "==" {
		nullCheck = fmt.Sprintf("sk_bool_new(%s.__is_null)", valueVal)
	} else {
		nullCheck = fmt.Sprintf("sk_bool_new(!%s.__is_null)", valueVal)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "=",
		Result: result,
		Arg1:   nullCheck,
	})

	return result
}

func (p *Pipeline) processBinary(bin *front.BinaryExpr, irFn *IRFunction) string {
	leftType := p.getExprType(bin.Left, irFn)
	rightType := p.getExprType(bin.Right, irFn)

	// === Обработка null (void) в сравнениях ===
	// string == null, int == null, и т.д. → проверка .__is_null
	if leftType == "void" || rightType == "void" {
		return p.processNullComparison(bin, leftType, rightType, irFn)
	}

	// === char vs char/string/int ===
	if leftType == "char" || rightType == "char" {
		return p.processCharBinary(bin, leftType, rightType, irFn)
	}

	// === Union ===
	if isUnionTypeP(leftType) || isUnionTypeP(rightType) {
		leftVal := p.processExpression(bin.Left, irFn)
		rightVal := p.processExpression(bin.Right, irFn)

		if !isUnionTypeP(leftType) {
			wrapper := p.getAnyWrapperByType(leftType)
			tempL := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_any "+tempL)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempL,
				Arg1:       wrapper,
				Arg2:       leftVal,
				ReturnType: "sk_any",
			})
			leftVal = tempL
		}

		if !isUnionTypeP(rightType) {
			wrapper := p.getAnyWrapperByType(rightType)
			tempR := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_any "+tempR)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tempR,
				Arg1:       wrapper,
				Arg2:       rightVal,
				ReturnType: "sk_any",
			})
			rightVal = tempR
		}

		// === СНАЧАЛА — сравнения ===
		if bin.Op == "<" || bin.Op == ">" || bin.Op == "==" ||
			bin.Op == "!=" || bin.Op == "<=" || bin.Op == ">=" {

			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_bool "+result)

			var fnName string
			switch bin.Op {
			case "<":
				fnName = "sk_any_lt"
			case ">":
				fnName = "sk_any_gt"
			case "==":
				fnName = "sk_any_eq"
			case "!=":
				fnName = "sk_any_ne"
			case "<=":
				fnName = "sk_any_le"
			case ">=":
				fnName = "sk_any_ge"
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

		// === Арифметика ===
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_any "+result)

		var fnName string
		switch bin.Op {
		case "+":
			fnName = "sk_any_add"
		case "-":
			fnName = "sk_any_sub"
		case "*":
			fnName = "sk_any_mul"
		case "/":
			fnName = "sk_any_div"
		case "%":
			fnName = "sk_any_mod"
		case "**":
			fnName = "sk_any_pow"
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       leftVal + ", " + rightVal,
			ReturnType: "sk_any",
		})
		return result
	}

	if bin.Op == "+" && isArrayType(leftType) {
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

	// Конкатенация строк — только string + string, char + string, string + char, char + char
	if bin.Op == "+" &&
		(leftType == "string" || leftType == "char") &&
		(rightType == "string" || rightType == "char") {
		leftVal := left
		if leftType == "char" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_char_to_string",
				Arg2:       left,
				ReturnType: "sk_string",
			})
			leftVal = tmp
		}

		rightVal := right
		if rightType == "char" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_char_to_string",
				Arg2:       right,
				ReturnType: "sk_string",
			})
			rightVal = tmp
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

	// === any в операнде — runtime dispatch через sk_any_* ===
	if leftType == "any" || rightType == "any" {
		return p.processAnyBinary(bin, left, right, leftType, rightType, irFn)
	}

	// Арифметика/сравнения — распаковываем any если надо
	leftVal := left
	leftT := leftType
	rightVal := right
	rightT := rightType

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

	// Приводим операнды к resultT
	if leftT != resultT {
		newLeft := p.newTemp()
		switch resultT {
		case "double":
			irFn.Locals = append(irFn.Locals, "sk_double "+newLeft)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     newLeft,
				Arg1:       "sk_double_new",
				Arg2:       fmt.Sprintf("(double)%s.value", leftVal),
				ReturnType: "sk_double",
			})
		case "float":
			irFn.Locals = append(irFn.Locals, "sk_float "+newLeft)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     newLeft,
				Arg1:       "sk_float_new",
				Arg2:       fmt.Sprintf("(float)%s.value", leftVal),
				ReturnType: "sk_float",
			})
		case "int":
			irFn.Locals = append(irFn.Locals, "sk_int "+newLeft)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     newLeft,
				Arg1:       "sk_int_new",
				Arg2:       fmt.Sprintf("(int)%s.value", leftVal),
				ReturnType: "sk_int",
			})
		}
		leftVal = newLeft
		leftT = resultT
	}

	if rightT != resultT {
		newRight := p.newTemp()
		switch resultT {
		case "double":
			irFn.Locals = append(irFn.Locals, "sk_double "+newRight)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     newRight,
				Arg1:       "sk_double_new",
				Arg2:       fmt.Sprintf("(double)%s.value", rightVal),
				ReturnType: "sk_double",
			})
		case "float":
			irFn.Locals = append(irFn.Locals, "sk_float "+newRight)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     newRight,
				Arg1:       "sk_float_new",
				Arg2:       fmt.Sprintf("(float)%s.value", rightVal),
				ReturnType: "sk_float",
			})
		case "int":
			irFn.Locals = append(irFn.Locals, "sk_int "+newRight)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     newRight,
				Arg1:       "sk_int_new",
				Arg2:       fmt.Sprintf("(int)%s.value", rightVal),
				ReturnType: "sk_int",
			})
		}
		rightVal = newRight
		rightT = resultT
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
		case "**":
			fnName = "sk_int_pow"
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
		case "**":
			fnName = "sk_float_pow"
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
		case "**":
			fnName = "sk_double_pow"
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

// processAnyBinary обрабатывает арифметику/сравнения, когда хотя бы один
// операнд — any (или union). Использует рантайм-функции sk_any_*.
func (p *Pipeline) processAnyBinary(
	bin *front.BinaryExpr,
	left, right string,
	leftType, rightType string,
	irFn *IRFunction,
) string {
	// Оборачиваем не-any операнды в sk_any, чтобы передать в sk_any_*
	leftAny := p.wrapToAny(leftType, left, irFn)
	rightAny := p.wrapToAny(rightType, right, irFn)

	// === Сравнения → sk_bool ===
	if bin.Op == "<" || bin.Op == ">" || bin.Op == "<=" ||
		bin.Op == ">=" || bin.Op == "==" || bin.Op == "!=" {

		var fnName string
		switch bin.Op {
		case "<":
			fnName = "sk_any_lt"
		case ">":
			fnName = "sk_any_gt"
		case "<=":
			fnName = "sk_any_le"
		case ">=":
			fnName = "sk_any_ge"
		case "==":
			fnName = "sk_any_eq"
		case "!=":
			fnName = "sk_any_ne"
		}

		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       leftAny + ", " + rightAny,
			ReturnType: "sk_bool",
		})
		return result
	}

	// === Логические операции → sk_bool ===
	if bin.Op == "&&" || bin.Op == "||" {
		leftBool := p.anyToBool(leftAny, irFn)
		rightBool := p.anyToBool(rightAny, irFn)

		fnName := "sk_bool_and"
		if bin.Op == "||" {
			fnName = "sk_bool_or"
		}

		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       leftBool + ", " + rightBool,
			ReturnType: "sk_bool",
		})
		return result
	}

	// === Конкатенация строк ===
	if bin.Op == "+" &&
		(leftType == "string" || rightType == "string" ||
			leftType == "char" || rightType == "char") {
		leftStr := p.anyToString(leftAny, irFn)
		rightStr := p.anyToString(rightAny, irFn)

		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_string "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_concat",
			Arg2:       leftStr + ", " + rightStr,
			ReturnType: "sk_string",
		})
		return result
	}

	// === Арифметика → sk_any ===
	var fnName string
	switch bin.Op {
	case "+":
		fnName = "sk_any_add"
	case "-":
		fnName = "sk_any_sub"
	case "*":
		fnName = "sk_any_mul"
	case "/":
		fnName = "sk_any_div"
	case "%":
		fnName = "sk_any_mod"
	case "**":
		fnName = "sk_any_pow"
	default:
		// Неизвестная операция — возвращаем null
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_any "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "any_null()",
		})
		return result
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_any "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       fnName,
		Arg2:       leftAny + ", " + rightAny,
		ReturnType: "sk_any",
	})
	return result
}

// anyToBool конвертирует sk_any в sk_bool через any_to_bool.
func (p *Pipeline) anyToBool(anyVal string, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_bool "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "any_to_bool",
		Arg2:       anyVal,
		ReturnType: "sk_bool",
	})
	return result
}

// anyToString конвертирует sk_any в sk_string через any_to_string.
func (p *Pipeline) anyToString(anyVal string, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_string "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "any_to_string",
		Arg2:       anyVal,
		ReturnType: "sk_string",
	})
	return result
}

func (p *Pipeline) processExpression(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		if strings.HasSuffix(n.Value, "f") || strings.HasSuffix(n.Value, "F") {
			val := n.Value[:len(n.Value)-1]
			return fmt.Sprintf("sk_float_new(%sf)", val)
		}
		if strings.Contains(n.Value, ".") {
			return fmt.Sprintf("sk_double_new(%s)", n.Value)
		}
		return fmt.Sprintf("sk_int_new(%s)", n.Value)
	case *front.NullLiteral:
		return "SK_NULL_int"
	case *front.DictLiteral:
		return p.processDictLiteral(n, irFn)
	case *front.UnicodeLiteral:
		return fmt.Sprintf("sk_char_new(%d)", n.Codepoint)
	case *front.String:
		return fmt.Sprintf("sk_string_new(%q)", n.Value)
	case *front.CharLiteral:
		return fmt.Sprintf("sk_char_new(%d)", n.Value)
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
	case *front.FormatExpr:
		return p.processFormatExpr(n, irFn)
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

func (p *Pipeline) processDictAccess(objName, key string, irFn *IRFunction) string {
	// Определяем реальный тип ключа (если известен)
	keyType := "any"
	if irFn.DictKeyTypes != nil {
		if keyTypes, ok := irFn.DictKeyTypes[objName]; ok {
			if t, ok := keyTypes[key]; ok {
				keyType = t
			}
		}
	}

	// Получаем sk_any из словаря
	rawAny := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_any "+rawAny)
	keyStr := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_string "+keyStr)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "=",
		Result: keyStr,
		Arg1:   fmt.Sprintf("sk_string_new(%q)", key),
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     rawAny,
		Arg1:       "sk_dict_get",
		Arg2:       objName + ".value, " + keyStr,
		ReturnType: "sk_any",
	})

	// Если тип any — возвращаем sk_any как есть
	if keyType == "any" || keyType == "" {
		return rawAny
	}

	// Иначе распаковываем через any_to_*
	getter := p.getAnyGetterByType(keyType)
	if getter == "" {
		return rawAny
	}

	result := p.newTemp()
	cType := p.typeToC(keyType)
	irFn.Locals = append(irFn.Locals, cType+" "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       getter,
		Arg2:       rawAny,
		ReturnType: cType,
	})
	return result
}

func (p *Pipeline) processDictLiteral(d *front.DictLiteral, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_dict_ref "+result)
	tmpDict := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_dict* "+tmpDict)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "=",
		Result: tmpDict,
		Arg1:   "sk_dict_new()",
	})

	for _, elem := range d.Elements {
		valType := p.getExprType(elem.Value, irFn)
		val := p.processExpression(elem.Value, irFn)
		valAny := p.wrapToAny(valType, val, irFn)

		keyStr := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_string "+keyStr)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: keyStr,
			Arg1:   fmt.Sprintf("sk_string_new(%q)", elem.Key),
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   "call",
			Arg1: "sk_dict_set",
			Arg2: tmpDict + ", " + keyStr + ", " + valAny,
		})
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_dict_ref_new",
		Arg2:       tmpDict,
		ReturnType: "sk_dict_ref",
	})
	return result
}

func (p *Pipeline) wrapToAny(t, val string, irFn *IRFunction) string {
	if t == "any" || isUnionTypeP(t) {
		return val
	}
	wrapper := p.getAnyWrapperByType(t)
	if wrapper == "" {
		return val
	}
	tmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_any "+tmp)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     tmp,
		Arg1:       wrapper,
		Arg2:       val,
		ReturnType: "sk_any",
	})
	return tmp
}

func (p *Pipeline) processReturn(ret *front.ReturnStmt, irFn *IRFunction) {
	exprResult := ""
	if ret.Expr != nil {
		exprType := p.getExprType(ret.Expr, irFn)

		if isUnionTypeP(irFn.ReturnType) {
			expr := p.processExpression(ret.Expr, irFn)

			// Если expr уже sk_any (any или union) — не оборачивать
			if exprType == "any" || isUnionTypeP(exprType) {
				exprResult = expr
			} else {
				wrapper := p.getAnyWrapperByType(exprType)
				if wrapper == "" {
					exprResult = expr
				} else {
					tempVar := p.newTemp()
					irFn.Locals = append(irFn.Locals, "sk_any "+tempVar)
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:         "call",
						Result:     tempVar,
						Arg1:       wrapper,
						Arg2:       expr,
						ReturnType: "sk_any",
					})
					exprResult = tempVar
				}
			}
		} else if exprType == "void" {
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
	debug.Debug("processCall: targetFunc=%s, params=%d", targetFunc.Name, len(targetFunc.Params))

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
		Op:     "call",
		Arg1:   p.cNameForFunc(funcName),
		Arg2:   argsStr,
		Line:   call.GetLine(),
		Column: call.GetColumn(),
		File:   irFn.File,
	})
}

func (p *Pipeline) processCallExpr(call *front.CallExpr, irFn *IRFunction) string {
	// Определяем, является ли Receiver именем модуля
	isModuleCall := false
	if call.Receiver != "" {
		for _, imp := range p.Program.Imports {
			moduleName := front.GetModuleName(imp.Path)
			if moduleName == call.Receiver || imp.Alias == call.Receiver {
				isModuleCall = true
				break
			}
		}
	}

	// Аргументы. Если Receiver — не модуль, добавляем receiver первым аргументом.
	args := call.Args
	if !isModuleCall {
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
	case "to_char":
		return p.processToChar(&front.CallExpr{Name: "to_char", Args: args}, irFn)
	case "to_double":
		return p.processToDouble(&front.CallExpr{Name: "to_double", Args: args}, irFn)
	case "to_string":
		return p.processToString(&front.CallExpr{Name: "to_string", Args: args}, irFn)
	case "to_bool":
		return p.processToBool(&front.CallExpr{Name: "to_bool", Args: args}, irFn)
	case "to_arr":
		// Если один аргумент и он any — распаковываем через any_to_arr
		if len(args) == 1 {
			argType := p.getExprType(args[0], irFn)
			if argType == "any" || isUnionTypeP(argType) {
				arg := p.processExpression(args[0], irFn)
				result := p.newTemp()
				irFn.Locals = append(irFn.Locals, "sk_arr "+result)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     result,
					Arg1:       "any_to_arr",
					Arg2:       arg,
					ReturnType: "sk_arr",
				})
				return result
			}
		}
		return p.processToArr(&front.CallExpr{Name: "to_arr", Args: args}, irFn)
	case "detruncate":
		return p.processDetruncate(&front.CallExpr{Name: "detruncate", Args: args}, irFn)
	}

	// Ищем целевую функцию
	targetFunc := p.findFunction(simpleName)
	if targetFunc == nil {
		debug.Debug("processCall: targetFunc is nil for %s — using fallback (NO wrapping)", simpleName)
	} else {
		debug.Debug("processCall: targetFunc=%s, params=%d", targetFunc.Name, len(targetFunc.Params))
	}

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
	returnType := "sk_int"
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
		Arg1:       p.cNameForFunc(simpleName),
		Arg2:       argsJoined,
		ReturnType: returnType,
		Line:       call.GetLine(),
		Column:     call.GetColumn(),
		File:       irFn.File,
	})

	return result
}

// processDetruncate — возвращает declared тип переменной как строку.
// Compile-time: известен из VarTypes / Params / AST.
func (p *Pipeline) processDetruncate(call *front.CallExpr, irFn *IRFunction) string {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_string "+result)

	if len(call.Args) == 0 {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   `sk_string_new("void")`,
		})
		return result
	}

	arg := call.Args[0]

	// Определяем declared тип аргумента
	declType := p.getDeclaredType(arg, irFn)

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_string_new",
		Arg2:       fmt.Sprintf(`"%s"`, declType),
		ReturnType: "sk_string",
	})

	return result
}

// getDeclaredType возвращает объявленный (статический) тип выражения.
// Для Ident — из VarTypes / Params.
// Для литералов — их собственный тип.
// Для остального — "unknown".
func (p *Pipeline) getDeclaredType(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Ident:
		// Параметры
		for _, param := range irFn.Params {
			if param.Name == n.Name {
				return param.Type
			}
		}
		// Локальные с известным Skorpion-типом
		if irFn.VarTypes != nil {
			if t, ok := irFn.VarTypes[n.Name]; ok {
				return t
			}
		}
		return "unknown"
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
	case *front.NullLiteral:
		return "void"
	case *front.ArrayLiteral:
		return "arr[any]"
	case *front.ErrorInstance:
		return n.TypeName
	case *front.FieldAccess:
		if n.Field == "msg" {
			return "string"
		}
		return "int"
	default:
		return "unknown"
	}
}

func (p *Pipeline) prepareArg(argExpr front.Node, paramType string, irFn *IRFunction) string {
	argType := p.getExprType(argExpr, irFn)
	argValue := p.processExpression(argExpr, irFn)

	// Union или any — если arg уже sk_any, не оборачивать
	if isUnionTypeP(paramType) || paramType == "any" {
		if argType == "any" || isUnionTypeP(argType) {
			return argValue
		}
		if argType == "void" {
			return "any_null()"
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

	// char → int (неявно)
	if argType == "char" && paramType == "int" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "sk_char_to_int",
			Arg2:       argValue,
			ReturnType: "sk_int",
		})
		return tmp
	}
	// int → char (неявно)
	if argType == "int" && paramType == "char" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_char "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "sk_int_to_char",
			Arg2:       argValue,
			ReturnType: "sk_char",
		})
		return tmp
	}

	if argType == "void" {
		cType := p.typeToC(paramType)
		return "SK_NULL_" + strings.TrimPrefix(cType, "sk_")
	}

	if argType == "any" || isUnionTypeP(argType) {
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
	debug.Debug("Pipeline.findFunction: %s (Functions=%d, AllFunctions=%d)",
		name, len(p.Program.Functions), len(p.Program.AllFunctions))
	for _, fn := range p.Program.Functions {
		if fn.Name == name {
			return fn
		}
	}
	// NEW: поиск среди импортированных / всех функций
	for _, fn := range p.Program.AllFunctions {
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
		// NEW
		for _, fn := range p.Program.AllFunctions {
			if fn.Name == simpleName {
				return fn
			}
		}
	}
	debug.Debug("Pipeline.findFunction: %s NOT FOUND", name)
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
		branchLabel := p.newLabel()
		nextLabel := p.newLabel()

		// Собираем OR всех паттернов
		var combinedCheck string

		for i, patNode := range branch.Patterns {
			pattern := p.processExpression(patNode, irFn)
			patternType := p.getExprType(patNode, irFn)

			if patternType == "any" {
				tmp := p.newTemp()
				irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     tmp,
					Arg1:       "any_to_int",
					Arg2:       pattern,
					ReturnType: "sk_int",
				})
				pattern = tmp
			}

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

			if i == 0 {
				combinedCheck = cmpVar
			} else {
				// OR с предыдущим
				orVar := p.newTemp()
				irFn.Locals = append(irFn.Locals, "sk_bool "+orVar)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         "call",
					Result:     orVar,
					Arg1:       "sk_bool_or",
					Arg2:       combinedCheck + ", " + cmpVar,
					ReturnType: "sk_bool",
				})
				combinedCheck = orVar
			}
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "if",
			Result: combinedCheck,
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

func (p *Pipeline) processForIn(forIn *front.ForInStmt, irFn *IRFunction) {
	iterType := p.getExprType(forIn.Iterable, irFn)

	// 1. Получаем iterable
	iterVal := p.processExpression(forIn.Iterable, irFn)

	// 2. Счётчик
	idxVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_int "+idxVar)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "=",
		Result: idxVar,
		Arg1:   "sk_int_new(0)",
	})

	// 3. Переменная элемента
	cElemType := p.typeToC(forIn.VarType)
	irFn.Locals = append(irFn.Locals, cElemType+" "+forIn.VarName)
	if irFn.VarTypes == nil {
		irFn.VarTypes = make(map[string]string)
	}
	irFn.VarTypes[forIn.VarName] = forIn.VarType

	startLabel := p.newLabel()
	bodyLabel := p.newLabel()
	endLabel := p.newLabel()

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: startLabel,
	})

	// 4. Условие idx < length
	lenVar := p.newTemp()

	if iterType == "string" {
		irFn.Locals = append(irFn.Locals, "sk_int "+lenVar)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     lenVar,
			Arg1:       "sk_string_utf8_len",
			Arg2:       iterVal,
			ReturnType: "sk_int",
		})
	} else {
		// arr[T]
		irFn.Locals = append(irFn.Locals, "sk_int "+lenVar)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     lenVar,
			Arg1:       "sk_int_new",
			Arg2:       fmt.Sprintf("sk_array_len(%s.value)", iterVal),
			ReturnType: "sk_int",
		})
	}

	condVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_bool "+condVar)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     condVar,
		Arg1:       "sk_int_lt",
		Arg2:       idxVar + ", " + lenVar,
		ReturnType: "sk_bool",
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "if",
		Result: condVar,
		Arg1:   bodyLabel,
		Arg2:   endLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: bodyLabel,
	})

	// 5. el = iterable[idx]
	if iterType == "string" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     forIn.VarName,
			Arg1:       "sk_string_char_at",
			Arg2:       iterVal + ", " + idxVar,
			ReturnType: "sk_string",
		})
	} else {
		// arr[T] → array_get_typed или any
		if forIn.VarType == "any" || forIn.VarType == "" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "array_get_any",
				Result:     forIn.VarName,
				Arg1:       iterVal,
				Arg2:       idxVar,
				ReturnType: "sk_any",
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "array_get_typed",
				Result:     forIn.VarName,
				Arg1:       iterVal,
				Arg2:       idxVar,
				ReturnType: cElemType,
			})
		}
	}

	// 6. Тело
	if forIn.Body != nil {
		p.processBlock(forIn.Body, irFn)
	}

	// 7. idx = idx + 1
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     idxVar,
		Arg1:       "sk_int_add",
		Arg2:       idxVar + ", sk_int_new(1)",
		ReturnType: "sk_int",
	})

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
	if irFn.VarTypes != nil {
		if objType, ok := irFn.VarTypes[idx.Name]; ok && objType == "dict" {
			index := p.processExpression(idx.Index, irFn)
			rawAny := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_any "+rawAny)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     rawAny,
				Arg1:       "sk_dict_get",
				Arg2:       fmt.Sprintf("%s.value, %s", idx.Name, index),
				ReturnType: "sk_any",
			})
			// Тип ключа неизвестен (индекс — рантайм-выражение), возвращаем sk_any
			return rawAny
		}
	}

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
	varType := ""
	for _, local := range irFn.Locals {
		parts := strings.Fields(local)
		if len(parts) >= 2 && parts[len(parts)-1] == length.Name {
			varType = parts[0]
			break
		}
	}
	if varType == "" {
		for _, param := range irFn.Params {
			if param.Name == length.Name {
				varType = p.typeToC(param.Type)
				break
			}
		}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_int "+result)

	if varType == "sk_string" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_new",
			Arg2:       fmt.Sprintf("(int)__sk__utf8__strlen(%s.value)", length.Name),
			ReturnType: "sk_int",
		})
	} else if varType == "sk_dict_ref" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_new",
			Arg2:       fmt.Sprintf("sk_dict_len(%s.value)", length.Name),
			ReturnType: "sk_int",
		})
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "array_len",
			Result:     result,
			Arg1:       length.Name,
			ReturnType: "sk_int",
		})
	}
	return result
}

func (p *Pipeline) processArrayAdd(add *front.ArrayAdd, irFn *IRFunction) string {
	elemType := p.getExprType(add.Elem, irFn)
	elemVal := p.processExpression(add.Elem, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	// Инлайним sk_array_deep_copy в sk_arr_new,
	// чтобы GC не освободил промежуточный sk_array*
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       fmt.Sprintf("sk_array_deep_copy(%s.value)", add.Name),
		ReturnType: "sk_arr",
	})

	pushArrayElem(irFn, result, elemType, elemVal)

	return result
}

func (p *Pipeline) processArrayAddName(name string, elemExpr front.Node, irFn *IRFunction) string {
	elemType := p.getExprType(elemExpr, irFn)
	elemVal := p.processExpression(elemExpr, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	// Инлайним sk_array_deep_copy в sk_arr_new,
	// чтобы GC не освободил промежуточный sk_array*
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       fmt.Sprintf("sk_array_deep_copy(%s.value)", name),
		ReturnType: "sk_arr",
	})

	pushArrayElem(irFn, result, elemType, elemVal)

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
	case "char":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_char_to_int",
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
	case "void":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_" + strings.TrimPrefix(p.typeToC("int"), "sk_"),
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
	case "void":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_" + strings.TrimPrefix(p.typeToC("float"), "sk_"),
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
	case "void":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_" + strings.TrimPrefix(p.typeToC("double"), "sk_"),
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
	case "char":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_char_to_string",
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
	case "void":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_" + strings.TrimPrefix(p.typeToC("string"), "sk_"),
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

func (p *Pipeline) processToChar(call *front.CallExpr, irFn *IRFunction) string {
	if len(call.Args) == 0 {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_char "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_char",
		})
		return result
	}
	argType := p.getExprType(call.Args[0], irFn)
	arg := p.processExpression(call.Args[0], irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_char "+result)

	switch argType {
	case "int":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_to_char",
			Arg2:       arg,
			ReturnType: "sk_char",
		})
	case "string":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_string_to_char",
			Arg2:       arg,
			ReturnType: "sk_char",
		})
	case "float":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_to_char",
			Arg2:       fmt.Sprintf("sk_int_new((int)%s.value)", arg),
			ReturnType: "sk_char",
		})
	case "double":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       "sk_int_to_char",
			Arg2:       fmt.Sprintf("sk_int_new((int)%s.value)", arg),
			ReturnType: "sk_char",
		})
	default:
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_char",
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
	case "void":
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: result,
			Arg1:   "SK_NULL_" + strings.TrimPrefix(p.typeToC("bool"), "sk_"),
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

	// Инлайним sk_array_new в sk_arr_new,
	// чтобы GC не освободил промежуточный sk_array*
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       "sk_array_new(sizeof(sk_any), 5)",
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
		if strings.HasSuffix(n.Value, "f") || strings.HasSuffix(n.Value, "F") {
			return "float"
		}
		if strings.Contains(n.Value, ".") {
			return "double"
		}
		return "int"
	case *front.String:
		return "string"
	case *front.UnicodeLiteral:
		return "char"
	case *front.CharLiteral:
		return "char"
	case *front.NullLiteral:
		return "void"
	case *front.DictLiteral:
		return "dict"
	case *front.ErrorInstance:
		return n.TypeName
	case *front.FieldAccess:
		if irFn.VarTypes != nil {
			if objType, ok := irFn.VarTypes[n.Object]; ok {
				if objType == "dict" {
					if irFn.DictKeyTypes != nil {
						if keyTypes, ok := irFn.DictKeyTypes[n.Object]; ok {
							if t, ok := keyTypes[n.Field]; ok {
								return t
							}
						}
					}
					return "any"
				}
				if objType == "Error" {
					if n.Field == "msg" {
						return "string"
					}
				} else {
					allFields := p.collectErrorFields(objType)
					if ft, exists := allFields[n.Field]; exists {
						return ft
					}
				}
			}
		}
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
					case "sk_char":
						return "char"
					case "sk_double":
						return "double"
					case "sk_bool":
						return "bool"
					case "sk_arr":
						return "arr"
					case "sk_any":
						return "any"
					case "sk_dict_ref":
						return "dict"
					}
				}
			}
		}
		return ""
	case *front.BinaryExpr:
		leftType := p.getExprType(n.Left, irFn)
		rightType := p.getExprType(n.Right, irFn)

		if n.Op == "<" || n.Op == ">" || n.Op == "==" || n.Op == "!=" || n.Op == "<=" || n.Op == ">=" {
			return "bool"
		}
		if n.Op == "&&" || n.Op == "||" {
			return "bool"
		}

		if isUnionTypeP(leftType) || isUnionTypeP(rightType) {
			return "any"
		}

		// any в операнде → any
		if leftType == "any" || rightType == "any" {
			return "any"
		}

		// string + string / char + char / char + string → string
		if n.Op == "+" &&
			(leftType == "string" || leftType == "char") &&
			(rightType == "string" || rightType == "char") {
			return "string"
		}

		if leftType == "double" || rightType == "double" {
			return "double"
		}
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		return "int"
	case *front.FormatExpr:
		if n.Mode == "Nf" && n.N == 0 {
			return "int"
		}
		return p.getExprType(n.Expr, irFn)
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
		case "to_char":
			return "char"
		case "to_arr":
			return "arr"
		case "detruncate":
			return "string"
		}

		for _, fn := range p.Program.Functions {
			if fn.Name == simpleName {
				return fn.ReturnType
			}
		}
		// NEW
		for _, fn := range p.Program.AllFunctions {
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
		if irFn.VarTypes != nil {
			if objType, ok := irFn.VarTypes[n.Name]; ok && objType == "dict" {
				return "any"
			}
		}
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
	if isUnionTypeP(t) || t == "any" {
		return ""
	}
	if isArrayType(t) {
		return "any_arr"
	}
	switch t {
	case "int":
		return "any_int"
	case "string":
		return "any_string"
	case "char":
		return "any_char"
	case "float":
		return "any_float"
	case "double":
		return "any_double"
	case "bool":
		return "any_bool"
	case "dict":
		return "any_dict"
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
	case "char":
		return "any_to_char"
	case "float":
		return "any_to_float"
	case "double":
		return "any_to_double"
	case "bool":
		return "any_to_bool"
	case "dict":
		return "any_to_dict"
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
		case "char":
			elemType = 7
			elemSize = "sizeof(sk_char)"
		default:
			elemType = 5
			elemSize = "sizeof(sk_any)"
		}
	}

	// Инлайним sk_array_new в sk_arr_new,
	// чтобы GC не освободил промежуточный sk_array*
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       fmt.Sprintf("sk_array_new(%s, %d)", elemSize, elemType),
		ReturnType: "sk_arr",
	})

	innerElemType := parseArrayElemType(expectedElemType)

	for _, elem := range expandedElements {
		val := p.processExpressionTyped(elem, innerElemType, irFn)

		if isArrayType(expectedElemType) {
			pushArrayElem(irFn, result, expectedElemType, val)
		} else if expectedElemType == "" || expectedElemType == "any" {
			elemTypeName := p.getExprType(elem, irFn)
			if elemTypeName == "any" {
				pushArrayElem(irFn, result, "any", val)
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
				pushArrayElem(irFn, result, "any", tempVar)
			}
		} else {
			if p.getExprType(elem, irFn) == "void" {
				cType := p.typeToC(expectedElemType)
				nullMacro := "SK_NULL_" + strings.TrimPrefix(cType, "sk_")
				nullVar := p.newTemp()
				irFn.Locals = append(irFn.Locals, cType+" "+nullVar)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:     "=",
					Result: nullVar,
					Arg1:   nullMacro,
				})
				pushArrayElem(irFn, result, expectedElemType, nullVar)
			} else {
				pushArrayElem(irFn, result, expectedElemType, val)
			}
		}
	}

	return result
}

func (p *Pipeline) processRange(r *front.RangeExpr, irFn *IRFunction) string {
	start := p.processExpression(r.Start, irFn)
	end := p.processExpression(r.End, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_arr "+result)

	// Инлайним sk_range_new в sk_arr_new,
	// чтобы GC не освободил промежуточный sk_array*
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       "sk_arr_new",
		Arg2:       fmt.Sprintf("sk_range_new(%s.value, %s.value)", start, end),
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
		Arg1:       p.cNameForFunc(funcName),
		Arg2:       argsStr,
		ReturnType: returnType,
		Line:       call.GetLine(),
		Column:     call.GetColumn(),
		File:       irFn.File,
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

// processFormatExpr — генерация ^Nf или ^X...
func (p *Pipeline) processFormatExpr(fe *front.FormatExpr, irFn *IRFunction) string {
	exprType := p.getExprType(fe.Expr, irFn)
	expr := p.processExpression(fe.Expr, irFn)

	// int — no-op (возвращаем как есть)
	if exprType == "int" {
		return expr
	}

	cType := p.typeToC(exprType) // sk_float или sk_double
	isFloat := exprType == "float"

	// ^0f — int
	if fe.Mode == "Nf" && fe.N == 0 {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+result)

		var fnName string
		if isFloat {
			fnName = "sk_float_to_int_trunc"
		} else {
			fnName = "sk_double_to_int_trunc"
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       expr,
			ReturnType: "sk_int",
		})
		return result
	}

	// ^Nf (N > 0) — обрезка до N знаков
	if fe.Mode == "Nf" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, cType+" "+result)

		var fnName string
		if isFloat {
			fnName = "sk_float_with_prec"
		} else {
			fnName = "sk_double_with_prec"
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       fmt.Sprintf("%s, %d", expr, fe.N),
			ReturnType: cType,
		})
		return result
	}

	// ^X... — обрезка символов с конца
	if fe.Mode == "X" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, cType+" "+result)

		var fnName string
		if isFloat {
			fnName = "sk_float_trim"
		} else {
			fnName = "sk_double_trim"
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       fmt.Sprintf("%s, \"%s\"", expr, fe.TrimSet),
			ReturnType: cType,
		})
		return result
	}

	return expr
}

func pushArrayElem(irFn *IRFunction, arrVar string, elemType string, elemVal string) {
	var fnName string
	var arg string

	if elemType == "any" || isUnionTypeP(elemType) {
		fnName = "sk_array_push_any"
		arg = arrVar + ".value, " + elemVal
	} else if elemType == "string" {
		fnName = "sk_array_push_string"
		arg = arrVar + ".value, " + elemVal
	} else if elemType == "char" {
		fnName = "sk_array_push_char"
		arg = arrVar + ".value, " + elemVal
	} else if isArrayType(elemType) {
		fnName = "sk_array_push_arr"
		arg = arrVar + ".value, " + elemVal
	} else {
		// int/float/double/bool — через временную переменную,
		// потому что &sk_int_new(1) — не lvalue
		cType := ""
		switch elemType {
		case "int":
			cType = "sk_int"
		case "float":
			cType = "sk_float"
		case "double":
			cType = "sk_double"
		case "bool":
			cType = "sk_bool"
		default:
			cType = "sk_int"
		}
		tmp := fmt.Sprintf("_tmp_push_%d", len(irFn.Locals))
		irFn.Locals = append(irFn.Locals, cType+" "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "=",
			Result: tmp,
			Arg1:   elemVal,
		})
		fnName = "sk_array_push"
		arg = arrVar + ".value, &" + tmp
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   "call",
		Arg1: fnName,
		Arg2: arg,
	})
}

// computeCName возвращает C-имя функции Skorpion.
// main, sk_*, any_*, __sk__* — без изменений.
// Остальные — с префиксом __sk__.
func computeCName(name string) string {
	if name == "main" {
		return "main"
	}
	if strings.HasPrefix(name, "sk_") ||
		strings.HasPrefix(name, "any_") ||
		strings.HasPrefix(name, "__sk__") {
		return name
	}
	return "__sk__" + name
}

// cNameForFunc ищет функцию по имени и возвращает её C-имя.
// Если не найдена — считает как пользовательскую (__sk__<name>).
func (p *Pipeline) cNameForFunc(name string) string {
	for _, fn := range p.Program.Functions {
		if fn.Name == name {
			return computeCName(fn.Name)
		}
	}
	// NEW: импортированные тоже резолвятся
	for _, fn := range p.Program.AllFunctions {
		if fn.Name == name {
			return computeCName(fn.Name)
		}
	}
	return computeCName(name)
}

func (p *Pipeline) processCharBinary(bin *front.BinaryExpr, leftType, rightType string, irFn *IRFunction) string {
	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	// === Конкатенация ===
	if bin.Op == "+" {
		// char + char → string
		if leftType == "char" && rightType == "char" {
			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+result)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_char_add_char",
				Arg2:       left + ", " + right,
				ReturnType: "sk_string",
			})
			return result
		}
		// char + string → string
		if leftType == "char" && rightType == "string" {
			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+result)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_char_add_string",
				Arg2:       left + ", " + right,
				ReturnType: "sk_string",
			})
			return result
		}
		// string + char → string
		if leftType == "string" && rightType == "char" {
			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+result)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_string_add_char",
				Arg2:       left + ", " + right,
				ReturnType: "sk_string",
			})
			return result
		}
		// char + any → string (через any_to_string)
		if leftType == "char" && (rightType == "any" || isUnionTypeP(rightType)) {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_char_to_string",
				Arg2:       left,
				ReturnType: "sk_string",
			})
			tmpR := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmpR)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmpR,
				Arg1:       "any_to_string",
				Arg2:       right,
				ReturnType: "sk_string",
			})
			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+result)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_string_concat",
				Arg2:       tmp + ", " + tmpR,
				ReturnType: "sk_string",
			})
			return result
		}
		if (leftType == "any" || isUnionTypeP(leftType)) && rightType == "char" {
			tmpL := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmpL)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmpL,
				Arg1:       "any_to_string",
				Arg2:       left,
				ReturnType: "sk_string",
			})
			tmpR := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+tmpR)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmpR,
				Arg1:       "sk_char_to_string",
				Arg2:       right,
				ReturnType: "sk_string",
			})
			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_string "+result)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     result,
				Arg1:       "sk_string_concat",
				Arg2:       tmpL + ", " + tmpR,
				ReturnType: "sk_string",
			})
			return result
		}
	}

	// === Сравнения ===
	if bin.Op == "==" || bin.Op == "!=" || bin.Op == "<" || bin.Op == ">" || bin.Op == "<=" || bin.Op == ">=" {
		// Приводим оба к int-функциям: char_to_int → sk_int_*
		leftInt := left
		rightInt := right
		if leftType == "char" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_char_to_int",
				Arg2:       left,
				ReturnType: "sk_int",
			})
			leftInt = tmp
		}
		if rightType == "char" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         "call",
				Result:     tmp,
				Arg1:       "sk_char_to_int",
				Arg2:       right,
				ReturnType: "sk_int",
			})
			rightInt = tmp
		}

		var fnName string
		switch bin.Op {
		case "==":
			fnName = "sk_int_eq"
		case "!=":
			fnName = "sk_int_ne"
		case "<":
			fnName = "sk_int_lt"
		case ">":
			fnName = "sk_int_gt"
		case "<=":
			fnName = "sk_int_le"
		case ">=":
			fnName = "sk_int_ge"
		}
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_bool "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       leftInt + ", " + rightInt,
			ReturnType: "sk_bool",
		})
		return result
	}

	// === Арифметика char <op> int ===
	// (в том числе char - char, char * int, char / int, char ** int, char % int)
	leftInt := left
	rightInt := right
	leftT := leftType
	rightT := rightType

	if leftT == "char" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "sk_char_to_int",
			Arg2:       left,
			ReturnType: "sk_int",
		})
		leftInt = tmp
		leftT = "int"
	}
	if rightT == "char" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+tmp)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     tmp,
			Arg1:       "sk_char_to_int",
			Arg2:       right,
			ReturnType: "sk_int",
		})
		rightInt = tmp
		rightT = "int"
	}

	// Приведение к общему числовому типу — как в обычной арифметике
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

	// Оба int — стандартная арифметика
	if resultT == "int" {
		var fnName string
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
		case "**":
			fnName = "sk_int_pow"
		default:
			fnName = "sk_int_add"
		}
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, "sk_int "+result)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     result,
			Arg1:       fnName,
			Arg2:       leftInt + ", " + rightInt,
			ReturnType: "sk_int",
		})
		return result
	}

	// float/double — конвертируем
	newLeft := p.newTemp()
	newRight := p.newTemp()

	if resultT == "double" {
		irFn.Locals = append(irFn.Locals, "sk_double "+newLeft)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     newLeft,
			Arg1:       "sk_double_new",
			Arg2:       fmt.Sprintf("(double)%s.value", leftInt),
			ReturnType: "sk_double",
		})
		irFn.Locals = append(irFn.Locals, "sk_double "+newRight)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     newRight,
			Arg1:       "sk_double_new",
			Arg2:       fmt.Sprintf("(double)%s.value", rightInt),
			ReturnType: "sk_double",
		})
	} else {
		irFn.Locals = append(irFn.Locals, "sk_float "+newLeft)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     newLeft,
			Arg1:       "sk_float_new",
			Arg2:       fmt.Sprintf("(float)%s.value", leftInt),
			ReturnType: "sk_float",
		})
		irFn.Locals = append(irFn.Locals, "sk_float "+newRight)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         "call",
			Result:     newRight,
			Arg1:       "sk_float_new",
			Arg2:       fmt.Sprintf("(float)%s.value", rightInt),
			ReturnType: "sk_float",
		})
	}

	var fnName string
	if resultT == "double" {
		switch bin.Op {
		case "+":
			fnName = "sk_double_add"
		case "-":
			fnName = "sk_double_sub"
		case "*":
			fnName = "sk_double_mul"
		case "/":
			fnName = "sk_double_div"
		case "**":
			fnName = "sk_double_pow"
		default:
			fnName = "sk_double_add"
		}
	} else {
		switch bin.Op {
		case "+":
			fnName = "sk_float_add"
		case "-":
			fnName = "sk_float_sub"
		case "*":
			fnName = "sk_float_mul"
		case "/":
			fnName = "sk_float_div"
		case "**":
			fnName = "sk_float_pow"
		default:
			fnName = "sk_float_add"
		}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, "sk_"+resultT+" "+result)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "call",
		Result:     result,
		Arg1:       fnName,
		Arg2:       newLeft + ", " + newRight,
		ReturnType: "sk_" + resultT,
	})
	return result
}
