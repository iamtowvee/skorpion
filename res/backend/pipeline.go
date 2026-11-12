package backend

import (
	"fmt"
	"os"
	"skrp/res/errors"
	"skrp/res/front"
	"skrp/res/log"
	"skrp/res/utils"
	"strconv"
	"strings"
)

var version string

type Pipeline struct {
	Program       *front.Program
	IR            *IRProgram
	TempCounter   int
	LabelCounter  int
	TryFrames     []string
	TryDepth      int
	cfg           *front.Config
	LoopStack     []LoopLabels
	PromiseOwners map[string]string
}

type LoopLabels struct {
	Start string
	End   string
}

func NewPipeline(prog *front.Program, cfg *front.Config, ver string) *Pipeline {
	version = ver
	return &Pipeline{
		Program:       prog,
		IR:            &IRProgram{},
		cfg:           cfg,
		PromiseOwners: make(map[string]string),
	}
}

func (p *Pipeline) Process() *IRProgram {
	p.IR.Config = p.cfg

	for _, code := range p.Program.GlobalIncludeC {
		p.IR.InlineC += code + "\n\n"
	}

	for _, g := range p.Program.GlobalVars {
		p.processGlobalVar(g)
	}

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

	allFuncs := p.Program.AllFunctions
	if len(allFuncs) == 0 {
		allFuncs = p.Program.Functions
	}

	processed := make(map[string]bool)
	for _, fn := range allFuncs {
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

// ============================================================================
// Global vars
// ============================================================================

func (p *Pipeline) processGlobalVar(g *front.VarDecl) {
	if g == nil {
		return
	}

	if g.Expr == nil {
		errors.NewError("1573",
			fmt.Sprintf("Global constant '%s' requires an initializer", g.Name),
			g.GetLine(), g.GetColumn(), "")
		return
	}

	varType := g.Type
	elemType := ""
	if g.IsArray {
		elemType = g.ElemType
		if elemType == "" {
			elemType = "any"
		}
		varType = "arr[" + elemType + "]"
	}

	init, errCode, errMsg, errLine, errCol := p.buildGlobalInit(
		g.Name, varType, elemType, g.Expr)

	if errCode != "" {
		errors.NewError(errCode, errMsg, errLine, errCol, "")
		return
	}

	p.IR.Globals = append(p.IR.Globals, IRGlobal{
		Name:    g.Name,
		Type:    varType,
		Value:   init,
		IsConst: g.IsConst,
	})
}

func (p *Pipeline) buildGlobalInit(name, varType, elemType string, expr front.Node) (IRGlobalInit, string, string, int, int) {
	// Скаляры
	switch varType {
	case "int":
		switch e := expr.(type) {
		case *front.Number:
			if strings.HasSuffix(e.Value, "f") || strings.HasSuffix(e.Value, "F") ||
				strings.Contains(e.Value, ".") {
				return IRGlobalInit{}, "1507",
					fmt.Sprintf("Type mismatch: cannot assign '%s' to global constant '%s' of type 'int'",
						numberLiteralType(e.Value), name),
					e.GetLine(), e.GetColumn()
			}
			v, _ := strconv.ParseInt(e.Value, 10, 64)
			return IRGlobalInit{Kind: GlobalInt, IntVal: v}, "", "", 0, 0
		case *front.CharLiteral:
			return IRGlobalInit{Kind: GlobalInt, IntVal: int64(e.Value)}, "", "", 0, 0
		case *front.UnicodeLiteral:
			return IRGlobalInit{Kind: GlobalInt, IntVal: int64(e.Codepoint)}, "", "", 0, 0
		}
		return p.badGlobalInit(name, "int", expr)

	case "char":
		switch e := expr.(type) {
		case *front.CharLiteral:
			return IRGlobalInit{Kind: GlobalChar, CharVal: e.Value}, "", "", 0, 0
		case *front.UnicodeLiteral:
			return IRGlobalInit{Kind: GlobalChar, CharVal: byte(e.Codepoint)}, "", "", 0, 0
		case *front.Number:
			if strings.Contains(e.Value, ".") ||
				strings.HasSuffix(e.Value, "f") ||
				strings.HasSuffix(e.Value, "F") {
				return IRGlobalInit{}, "1507",
					fmt.Sprintf("Type mismatch: cannot assign '%s' to global constant '%s' of type 'char'",
						numberLiteralType(e.Value), name),
					e.GetLine(), e.GetColumn()
			}
			v, _ := strconv.Atoi(e.Value)
			return IRGlobalInit{Kind: GlobalChar, CharVal: byte(v)}, "", "", 0, 0
		}
		return p.badGlobalInit(name, "char", expr)

	case "bool":
		if id, ok := expr.(*front.Ident); ok {
			if id.Name == "true" {
				return IRGlobalInit{Kind: GlobalBool, BoolVal: true}, "", "", 0, 0
			}
			if id.Name == "false" {
				return IRGlobalInit{Kind: GlobalBool, BoolVal: false}, "", "", 0, 0
			}
		}
		return p.badGlobalInit(name, "bool", expr)

	case "float":
		num, ok := expr.(*front.Number)
		if !ok {
			return p.badGlobalInit(name, "float", expr)
		}
		s := strings.TrimSuffix(strings.TrimSuffix(num.Value, "f"), "F")
		return IRGlobalInit{Kind: GlobalFloat, FloatVal: s}, "", "", 0, 0

	case "double":
		num, ok := expr.(*front.Number)
		if !ok {
			return p.badGlobalInit(name, "double", expr)
		}
		return IRGlobalInit{Kind: GlobalDouble, FloatVal: num.Value}, "", "", 0, 0

	case "string":
		str, ok := expr.(*front.String)
		if !ok {
			return p.badGlobalInit(name, "string", expr)
		}
		return IRGlobalInit{Kind: GlobalString, StrVal: str.Value}, "", "", 0, 0

	case "any":
		return p.buildGlobalAny(name, expr)

	case "dict":
		return p.buildGlobalDict(name, expr)
	}

	if utils.IsArrayType(varType) {
		return p.buildGlobalArray(name, elemType, expr)
	}

	return p.badGlobalInit(name, varType, expr)
}

func (p *Pipeline) badGlobalInit(name, varType string, expr front.Node) (IRGlobalInit, string, string, int, int) {
	if lt := p.globalLiteralType(expr); lt != "" {
		return IRGlobalInit{}, "1507",
			fmt.Sprintf("Type mismatch: cannot assign '%s' to global constant '%s' of type '%s'",
				lt, name, varType),
			expr.GetLine(), expr.GetColumn()
	}
	return IRGlobalInit{}, "1574",
		fmt.Sprintf("Global constant '%s' initializer must be a literal (got %T)", name, expr),
		expr.GetLine(), expr.GetColumn()
}

func (p *Pipeline) globalLiteralType(expr front.Node) string {
	switch e := expr.(type) {
	case *front.Number:
		return numberLiteralType(e.Value)
	case *front.String:
		return "string"
	case *front.CharLiteral, *front.UnicodeLiteral:
		return "char"
	case *front.Ident:
		if e.Name == "true" || e.Name == "false" {
			return "bool"
		}
	}
	return ""
}

func numberLiteralType(v string) string {
	if strings.HasSuffix(v, "f") || strings.HasSuffix(v, "F") {
		return "float"
	}
	if strings.Contains(v, ".") {
		return "double"
	}
	return "int"
}

func (p *Pipeline) buildGlobalArray(name, elemType string, expr front.Node) (IRGlobalInit, string, string, int, int) {
	arrLit, ok := expr.(*front.ArrayLiteral)
	if !ok {
		return p.badGlobalInit(name, "arr["+elemType+"]", expr)
	}
	if elemType == "" {
		elemType = "any"
	}
	if _, ok := cElemTypeCodeForArray(elemType); !ok {
		return IRGlobalInit{}, "1575",
			fmt.Sprintf("Global constant '%s': array element type '%s' is not supported",
				name, elemType),
			arrLit.GetLine(), arrLit.GetColumn()
	}

	elements := []IRGlobalInit{}
	for _, el := range arrLit.Elements {
		elInit, code, msg, ln, cl := p.buildGlobalElem(name, elemType, el)
		if code != "" {
			return IRGlobalInit{}, code, msg, ln, cl
		}
		elements = append(elements, elInit)
	}

	return IRGlobalInit{
		Kind:     GlobalArray,
		ElemType: elemType,
		Elements: elements,
	}, "", "", 0, 0
}

func (p *Pipeline) buildGlobalElem(parentName, elemType string, el front.Node) (IRGlobalInit, string, string, int, int) {
	if utils.IsArrayType(elemType) {
		return p.buildGlobalArray(parentName+"_sub", utils.ParseArrayElemType(elemType), el)
	}
	if elemType == "any" {
		return p.buildGlobalAny(parentName, el)
	}
	if elemType == "dict" {
		return p.buildGlobalDict(parentName, el)
	}
	return p.buildGlobalInit(parentName, elemType, "", el)
}

func (p *Pipeline) buildGlobalDict(name string, expr front.Node) (IRGlobalInit, string, string, int, int) {
	dictLit, ok := expr.(*front.DictLiteral)
	if !ok {
		return p.badGlobalInit(name, "dict", expr)
	}
	entries := []IRGlobalEntry{}
	for _, el := range dictLit.Elements {
		v, code, msg, ln, cl := p.buildGlobalAny(name+"_v", el.Value)
		if code != "" {
			return IRGlobalInit{}, code, msg, ln, cl
		}
		entries = append(entries, IRGlobalEntry{Key: el.Key, Value: v})
	}
	return IRGlobalInit{Kind: GlobalDict, Entries: entries}, "", "", 0, 0
}

func (p *Pipeline) buildGlobalAny(name string, expr front.Node) (IRGlobalInit, string, string, int, int) {
	switch e := expr.(type) {
	case *front.Number:
		t := numberLiteralType(e.Value)
		switch t {
		case "int":
			v, _ := strconv.ParseInt(e.Value, 10, 64)
			return IRGlobalInit{Kind: GlobalAny, IntVal: v}, "", "", 0, 0
		case "float":
			v := strings.TrimSuffix(strings.TrimSuffix(e.Value, "f"), "F")
			return IRGlobalInit{Kind: GlobalAny, FloatVal: v, ElemType: "float"}, "", "", 0, 0
		case "double":
			return IRGlobalInit{Kind: GlobalAny, FloatVal: e.Value, ElemType: "double"}, "", "", 0, 0
		}
	case *front.String:
		return IRGlobalInit{Kind: GlobalAny, StrVal: e.Value, ElemType: "string"}, "", "", 0, 0
	case *front.CharLiteral:
		return IRGlobalInit{Kind: GlobalAny, CharVal: e.Value, ElemType: "char"}, "", "", 0, 0
	case *front.UnicodeLiteral:
		return IRGlobalInit{Kind: GlobalAny, CharVal: byte(e.Codepoint), ElemType: "char"}, "", "", 0, 0
	case *front.Ident:
		if e.Name == "true" {
			return IRGlobalInit{Kind: GlobalAny, BoolVal: true, ElemType: "bool"}, "", "", 0, 0
		}
		if e.Name == "false" {
			return IRGlobalInit{Kind: GlobalAny, BoolVal: false, ElemType: "bool"}, "", "", 0, 0
		}
	case *front.NullLiteral:
		return IRGlobalInit{Kind: GlobalNull}, "", "", 0, 0
	case *front.ArrayLiteral:
		sub, code, msg, ln, cl := p.buildGlobalArray(name+"_sub", "", e)
		if code != "" {
			return IRGlobalInit{}, code, msg, ln, cl
		}
		return IRGlobalInit{Kind: GlobalAny, ElemType: "arr", Elements: []IRGlobalInit{sub}}, "", "", 0, 0
	case *front.DictLiteral:
		sub, code, msg, ln, cl := p.buildGlobalDict(name+"_sub", e)
		if code != "" {
			return IRGlobalInit{}, code, msg, ln, cl
		}
		return IRGlobalInit{Kind: GlobalAny, ElemType: "dict", Elements: []IRGlobalInit{sub}}, "", "", 0, 0
	}
	return p.badGlobalInit(name, "any", expr)
}

// pipeline.go — заменить функцию cElemTypeForArray
func cElemTypeCodeForArray(elemType string) (int, bool) {
	if utils.IsArrayType(elemType) {
		return 6, true
	}
	switch elemType {
	case "int":
		return 0, true
	case "string":
		return 1, true
	case "float":
		return 2, true
	case "double":
		return 3, true
	case "bool":
		return 4, true
	case "any":
		return 5, true
	case "char":
		return 7, true
	case "dict":
		return 8, true
	}
	return 0, false
}

func cElemTypeForArray(elemType string) (string, int, bool) {
	if utils.IsArrayType(elemType) {
		return "sk_arr", 6, true
	}
	switch elemType {
	case "int":
		return "sk_int", 0, true
	case "string":
		return "sk_string", 1, true
	case "float":
		return "sk_float", 2, true
	case "double":
		return "sk_double", 3, true
	case "bool":
		return "sk_bool", 4, true
	case "any":
		return "sk_any", 5, true
	case "char":
		return "sk_char", 7, true
	case "dict":
		return "sk_dict_ref", 8, true
	}
	return "", 0, false
}

func cZeroInitForArrayElem(elemType string) string {
	if utils.IsArrayType(elemType) {
		return "{ NULL, 1 }"
	}
	switch elemType {
	case "int":
		return "{ 0, 1 }"
	case "char":
		return "{ 0, 1 }"
	case "bool":
		return "{ 0, 1 }"
	case "float":
		return "{ 0, 1, -1 }"
	case "double":
		return "{ 0, 1, -1 }"
	case "string":
		return "{ NULL, 1 }"
	case "any":
		return "{ 6, -1, { .p = NULL } }"
	case "dict":
		return "{ NULL, 1 }"
	}
	return "0"
}

// ============================================================================
// Functions
// ============================================================================

func (p *Pipeline) processFunction(fn *front.Function) {
	irFn := IRFunction{
		Name:           fn.Name,
		CName:          computeCName(fn.Name),
		ReturnType:     fn.ReturnType,
		IsExport:       fn.IsExport,
		IsAsync:        fn.IsAsync,
		File:           fn.File,
		Params:         []IRParam{},
		Locals:         []IRLocal{},
		ArrayElemTypes: make(map[string]string),
		VarTypes:       make(map[string]string),
		DictKeyTypes:   make(map[string]map[string]string),
	}

	for _, param := range fn.Params {
		paramType := param.Type
		if param.IsVariadic {
			paramType = "arr[any]"
		}

		irFn.Params = append(irFn.Params, IRParam{Name: param.Name, Type: paramType})
		irFn.VarTypes[param.Name] = paramType

		if utils.IsArrayType(paramType) {
			elemType := utils.ParseArrayElemType(paramType)
			if elemType == "" {
				elemType = "any"
			}
			irFn.ArrayElemTypes[param.Name] = elemType
		}
	}

	p.TryFrames = []string{}
	p.TryDepth = 0
	p.LoopStack = []LoopLabels{}
	p.PromiseOwners = make(map[string]string)

	if fn.Body != nil {
		if fn.IsAsync && len(fn.Body.Statements) > 0 {
			startIdx := 0
			if _, ok := fn.Body.Statements[0].(*front.PromiseSet); ok {
				startIdx = 1
			}
			for i := startIdx; i < len(fn.Body.Statements); i++ {
				p.processNode(fn.Body.Statements[i], &irFn)
			}
		} else {
			p.processBlock(fn.Body, &irFn)
		}
	}

	if fn.ReturnType == "void" && len(irFn.Instructions) > 0 && !fn.IsAsync {
		lastIns := irFn.Instructions[len(irFn.Instructions)-1]
		if lastIns.Op != OpRet {
			for i := len(p.TryFrames) - 1; i >= 0; i-- {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   OpTryPop,
					Arg1: p.TryFrames[i],
				})
			}
			irFn.Instructions = append(irFn.Instructions, IRInstruction{Op: OpRet})
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
	case *front.PostfixExpr:
		p.processPostfixStatement(n, irFn)
	case *front.Assign:
		p.processAssign(n, irFn)
	case *front.BreakStmt:
		p.processBreak(irFn)
	case *front.ContinueStmt:
		p.processContinue(irFn)
	case *front.DoWhileStmt:
		p.processDoWhile(n, irFn)
	case *front.PromiseSet:
		p.processPromiseSet(n, irFn)
	case *front.AwaitExpr:
		p.processAwaitExpr(n, irFn)
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
			Op:   OpInlineC,
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

// ============================================================================
// Promise
// ============================================================================

func (p *Pipeline) processPromiseSet(ps *front.PromiseSet, irFn *IRFunction) {
	if !irFn.IsAsync {
		errors.NewError("0662",
			"'Promise[...]' can only be used inside an async function",
			ps.GetLine(), ps.GetColumn(), irFn.File)
		return
	}

	valType := p.getExprType(ps.Value, irFn)
	val := p.processExpression(ps.Value, irFn)

	if valType != irFn.ReturnType && valType != "" {
		val = p.convertToType(val, valType, irFn.ReturnType, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpPromiseSet,
		Arg1:       "__promise",
		Arg2:       val.Var,
		ReturnType: irFn.ReturnType,
	})
}

func (p *Pipeline) processAwaitExpr(aw *front.AwaitExpr, irFn *IRFunction) IRValue {
	switch e := aw.Expr.(type) {
	case *front.CallExpr:
		promiseVar := p.processAsyncCall(e, irFn)
		if promiseVar == "" {
			return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
		}
		return p.emitPromiseAwait(promiseVar, aw, irFn)

	case *front.Ident:
		handle, ok := p.PromiseOwners[e.Name]
		if !ok {
			errors.NewError("0663",
				fmt.Sprintf("'await' applied to '%s', which is not a promise", e.Name),
				aw.GetLine(), aw.GetColumn(), irFn.File)
			return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
		}
		return p.emitPromiseAwait(handle, aw, irFn)
	}

	errors.NewError("0663",
		"'await' requires a function call or a promise variable",
		aw.GetLine(), aw.GetColumn(), irFn.File)
	return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
}

func (p *Pipeline) typeToPromiseSuffix(t string) string {
	if utils.IsUnionType(t) {
		return "any"
	}
	if utils.IsArrayType(t) {
		return "arr"
	}
	switch t {
	case "int", "char", "float", "double", "bool", "string", "dict", "any", "void":
		return t
	}
	return "int"
}

func (p *Pipeline) processAsyncCall(call *front.CallExpr, irFn *IRFunction) string {
	targetFunc := p.findFunction(call.Name)
	if targetFunc == nil {
		errors.NewError("1518",
			fmt.Sprintf("Undefined function '%s'", call.Name),
			call.GetLine(), call.GetColumn(), irFn.File)
		return ""
	}
	if !targetFunc.IsAsync {
		errors.NewError("0664",
			fmt.Sprintf("'%s' is not an async function", call.Name),
			call.GetLine(), call.GetColumn(), irFn.File)
		return ""
	}

	simpleName := call.Name
	if strings.Contains(simpleName, ".") {
		parts := strings.Split(simpleName, ".")
		simpleName = parts[len(parts)-1]
	}

	argsStr := []string{}
	argIndex := 0
	for _, param := range targetFunc.Params {
		if param.IsVariadic {
			variadicArr := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "arr[any]", Name: variadicArr})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpArrayNew,
				Result:     variadicArr,
				Metadata:   "any",
				ReturnType: "arr[any]",
			})
			for argIndex < len(call.Args) {
				argExpr := call.Args[argIndex]
				argIndex++
				if isUnderscoreIdent(argExpr) {
					continue
				}
				argType := p.getExprType(argExpr, irFn)
				argVal := p.processExpression(argExpr, irFn)
				argAny := p.wrapToAny(argType, argVal, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   OpArrayPush,
					Arg1: variadicArr,
					Arg2: argAny.Var,
				})
			}
			argsStr = append(argsStr, variadicArr)
			break
		}

		var argExpr front.Node
		if argIndex < len(call.Args) {
			argExpr = call.Args[argIndex]
			argIndex++
			if isUnderscoreIdent(argExpr) {
				argExpr = param.DefaultValue
			}
		} else {
			argExpr = param.DefaultValue
		}
		if argExpr == nil {
			argsStr = append(argsStr, p.zeroOf(param.Type, irFn))
			continue
		}
		argsStr = append(argsStr, p.prepareArg(argExpr, param.Type, irFn).Var)
	}

	suffix := p.typeToPromiseSuffix(targetFunc.ReturnType)
	promiseVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "sk_promise_" + suffix + "*", Name: promiseVar})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpPromiseNew,
		Result:     promiseVar,
		Metadata:   targetFunc.ReturnType,
		ReturnType: "sk_promise_" + suffix + "*",
	})

	if targetFunc.Body != nil && len(targetFunc.Body.Statements) > 0 {
		if ps, ok := targetFunc.Body.Statements[0].(*front.PromiseSet); ok {
			litType := p.getExprType(ps.Value, irFn)
			if litType == "" {
				litType = targetFunc.ReturnType
			}
			litVal := p.processExpression(ps.Value, irFn)
			if litType != targetFunc.ReturnType {
				litVal = p.convertToType(litVal, litType, targetFunc.ReturnType, irFn)
			}
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpPromiseSet,
				Arg1:       promiseVar,
				Arg2:       litVal.Var,
				ReturnType: targetFunc.ReturnType,
			})
		}
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpAsyncSpawn,
		Result:     promiseVar,
		Arg1:       p.cNameForFunc(simpleName),
		Arg2:       strings.Join(argsStr, ", "),
		ReturnType: targetFunc.ReturnType,
	})

	return promiseVar
}

func (p *Pipeline) emitPromiseAwait(promiseVar string, aw *front.AwaitExpr, irFn *IRFunction) IRValue {
	var resultSkType string
	switch e := aw.Expr.(type) {
	case *front.CallExpr:
		if fn := p.findFunction(e.Name); fn != nil {
			resultSkType = fn.ReturnType
		}
	case *front.Ident:
		resultSkType = irFn.VarTypes[e.Name]
	}
	if resultSkType == "" {
		resultSkType = "int"
	}

	if resultSkType == "void" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpPromiseAwaitVoid,
			Arg1:       promiseVar,
			ReturnType: "void",
		})
		return IRValue{Type: "void", Var: ""}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: resultSkType, Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpPromiseAwait,
		Result:     result,
		Arg1:       promiseVar,
		ReturnType: resultSkType,
	})
	return IRValue{Type: resultSkType, Var: result}
}

// ============================================================================
// Control flow
// ============================================================================

func (p *Pipeline) processBreak(irFn *IRFunction) {
	if len(p.LoopStack) == 0 {
		return
	}
	loop := p.LoopStack[len(p.LoopStack)-1]
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpGoto,
		Arg1: loop.End,
	})
}

func (p *Pipeline) processContinue(irFn *IRFunction) {
	if len(p.LoopStack) == 0 {
		return
	}
	loop := p.LoopStack[len(p.LoopStack)-1]
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpGoto,
		Arg1: loop.Start,
	})
}

// ============================================================================
// Field access
// ============================================================================

func (p *Pipeline) processFieldAccess(fa *front.FieldAccess, irFn *IRFunction) IRValue {
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
	irFn.Locals = append(irFn.Locals, IRLocal{Type: fieldType, Name: result})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpFieldGet,
		Result:     result,
		Arg1:       fa.Object,
		Arg2:       fa.Field,
		ReturnType: fieldType,
	})

	return IRValue{Type: fieldType, Var: result}
}

func (p *Pipeline) collectErrorFields(typeName string) map[string]string {
	fields := make(map[string]string)

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

// ============================================================================
// Try / throw
// ============================================================================

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
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "try_frame", Name: frameVar})
	errorVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "error_ptr", Name: errorVar})

	p.TryFrames = append(p.TryFrames, frameVar)

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpTryPush,
		Result: frameVar,
		Arg1:   catchLabels[0],
	})

	p.processBlock(try.Body, irFn)

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpGoto,
		Arg1: endLabel,
	})

	framePopped := false

	for i, clause := range try.Catches {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: catchLabels[i],
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpTryPop,
			Arg1: frameVar,
		})
		if !framePopped {
			if len(p.TryFrames) > 0 && p.TryFrames[len(p.TryFrames)-1] == frameVar {
				p.TryFrames = p.TryFrames[:len(p.TryFrames)-1]
			}
			framePopped = true
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpTryGetError,
			Result: errorVar,
			Arg1:   frameVar,
		})

		if clause.TypeName != "" && clause.TypeName != "Error" {
			typeCheckVar := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: typeCheckVar})

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpErrorMatch,
				Result:     typeCheckVar,
				Arg1:       errorVar,
				Arg2:       p.fullErrorTypePath(clause.TypeName),
				ReturnType: "bool",
			})

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     OpIf,
				Result: typeCheckVar,
				Arg1:   catchLabels[i] + "_body",
				Arg2:   nextCatchLabels[i],
			})

			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     OpLabel,
				Result: catchLabels[i] + "_body",
			})
		}

		if clause.VarName != "" {
			typeName := clause.TypeName
			if typeName == "" {
				typeName = "Error"
			}
			irFn.Locals = append(irFn.Locals, IRLocal{Type: typeName, Name: clause.VarName})
			if irFn.VarTypes != nil {
				irFn.VarTypes[clause.VarName] = typeName
			}
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     OpErrorCast,
				Result: clause.VarName,
				Arg1:   errorVar,
				Arg2:   typeName,
			})
		}

		p.processBlock(clause.Body, irFn)

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpErrorFree,
			Arg1: errorVar,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpGoto,
			Arg1: endLabel,
		})

		if clause.TypeName != "" && clause.TypeName != "Error" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:     OpLabel,
				Result: nextCatchLabels[i],
			})
		}
	}

	lastIsCatchAll := len(try.Catches) > 0 && (try.Catches[len(try.Catches)-1].TypeName == "" || try.Catches[len(try.Catches)-1].TypeName == "Error")
	if !lastIsCatchAll {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpTryGetError,
			Result: errorVar,
			Arg1:   frameVar,
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpThrow,
			Arg1:   errorVar,
			Line:   try.GetLine(),
			Column: try.GetColumn(),
			File:   irFn.File,
		})
	}

	if !framePopped {
		if len(p.TryFrames) > 0 && p.TryFrames[len(p.TryFrames)-1] == frameVar {
			p.TryFrames = p.TryFrames[:len(p.TryFrames)-1]
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpTryPop,
			Arg1: frameVar,
		})
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: endLabel,
	})
}

// ============================================================================
// typeof
// ============================================================================

func (p *Pipeline) processTypeOf(typeOf *front.TypeOf, irFn *IRFunction) IRValue {
	exprType := p.getExprType(typeOf.Expr, irFn)
	expr := p.processExpression(typeOf.Expr, irFn)
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})

	if exprType == "any" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpTypeOfAny,
			Result:     result,
			Arg1:       expr.Var,
			ReturnType: "string",
		})
	} else {
		varType := exprType
		if varType == "" {
			varType = "unknown"
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpConstString,
			Result:     result,
			Arg1:       varType,
			ReturnType: "string",
		})
	}

	return IRValue{Type: "string", Var: result}
}

// ============================================================================
// Error instance
// ============================================================================

func (p *Pipeline) processErrorInstance(inst *front.ErrorInstance, irFn *IRFunction) IRValue {
	result := p.newTemp()
	cName := inst.TypeName
	if cName == "Error" {
		cName = "SkError"
	}
	irFn.Locals = append(irFn.Locals, IRLocal{Type: inst.TypeName, Name: result})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpErrorNew,
		Result:     result,
		Arg1:       p.fullErrorTypePath(inst.TypeName),
		ReturnType: inst.TypeName,
	})

	for fieldName, value := range inst.Fields {
		val := p.processExpression(value, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpFieldSet,
			Arg1: result,
			Arg2: fieldName,
			Arg3: val.Var,
		})
	}

	return IRValue{Type: inst.TypeName, Var: result}
}

func (p *Pipeline) fullErrorTypePath(typeName string) string {
	if typeName == "Error" {
		return "Error"
	}

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
		if modulePrefix != "" {
			return modulePrefix + ".Error." + simpleName
		}
		return "Error." + simpleName
	}

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

	chain := "Error." + strings.Join(path, ".")
	if decl.Module != "" {
		return decl.Module + "." + chain
	}
	return chain
}

func (p *Pipeline) processThrow(throw *front.ThrowStmt, irFn *IRFunction) {
	expr := p.processExpression(throw.Expr, irFn)

	if irFn.IsAsync {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpPromiseFail,
			Arg1:       "__promise",
			Arg2:       expr.Var,
			ReturnType: irFn.ReturnType,
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{Op: OpRet})
		return
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpThrow,
		Arg1:   expr.Var,
		Line:   throw.GetLine(),
		Column: throw.GetColumn(),
		File:   irFn.File,
	})
}

// ============================================================================
// Unary
// ============================================================================

func (p *Pipeline) processUnary(unary *front.UnaryExpr, irFn *IRFunction) IRValue {
	if unary.Op == "!" {
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})

		if exprType == "any" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpUnwrapAny,
				Result:     result,
				Arg1:       expr.Var,
				ReturnType: "bool",
			})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpNot,
				Result:     result,
				Arg1:       result,
				ReturnType: "bool",
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpNot,
				Result:     result,
				Arg1:       expr.Var,
				ReturnType: "bool",
			})
		}
		return IRValue{Type: "bool", Var: result}
	}

	if unary.Op == "-" {
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()

		if utils.IsUnionType(exprType) || exprType == "any" {
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: result})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpNeg,
				Result:     result,
				Arg1:       expr.Var,
				ReturnType: "any",
			})
			return IRValue{Type: "any", Var: result}
		}

		resultType := exprType
		if exprType == "char" {
			resultType = "int"
		}
		irFn.Locals = append(irFn.Locals, IRLocal{Type: resultType, Name: result})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpNeg,
			Result:     result,
			Arg1:       expr.Var,
			ReturnType: resultType,
		})
		return IRValue{Type: resultType, Var: result}
	}

	if unary.Op == "$" {
		exprType := p.getExprType(unary.Expr, irFn)
		expr := p.processExpression(unary.Expr, irFn)
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})

		switch {
		case exprType == "string":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: result, Arg1: expr.Var,
			})
		case exprType == "void":
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpConstString, Result: result, Arg1: "null", ReturnType: "string",
			})
		case utils.IsArrayType(exprType):
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpCast,
				Result:     result,
				Arg1:       expr.Var,
				ReturnType: "string",
				SrcType:    exprType,
				Metadata:   "array",
			})
		default:
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpCast,
				Result:     result,
				Arg1:       expr.Var,
				ReturnType: "string",
				SrcType:    exprType,
			})
		}
		return IRValue{Type: "string", Var: result}
	}
	return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
}

// ============================================================================
// VarDecl / Assign
// ============================================================================

func (p *Pipeline) processAsyncVarDecl(decl *front.VarDecl, call *front.CallExpr, targetFunc *front.Function, irFn *IRFunction) {
	promiseVar := p.processAsyncCall(call, irFn)
	if promiseVar == "" {
		return
	}

	irFn.VarTypes[decl.Name] = decl.Type

	simpleName := call.Name
	if strings.Contains(simpleName, ".") {
		parts := strings.Split(simpleName, ".")
		simpleName = parts[len(parts)-1]
	}
	p.PromiseOwners[simpleName] = promiseVar
	p.PromiseOwners[decl.Name] = promiseVar
}

func (p *Pipeline) processVarDecl(decl *front.VarDecl, irFn *IRFunction) {
	if call, ok := decl.Expr.(*front.CallExpr); ok {
		if targetFunc := p.findFunction(call.Name); targetFunc != nil && targetFunc.IsAsync {
			p.processAsyncVarDecl(decl, call, targetFunc, irFn)
			return
		}
	}

	if utils.IsUnionType(decl.Type) {
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: decl.Name})
		if irFn.VarTypes != nil {
			irFn.VarTypes[decl.Name] = decl.Type
		}

		if decl.Expr != nil {
			exprType := p.getExprType(decl.Expr, irFn)
			exprResult := p.processExpression(decl.Expr, irFn)

			if exprType == "any" || utils.IsUnionType(exprType) {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpAssign, Result: decl.Name, Arg1: exprResult.Var,
				})
			} else {
				val := p.wrapToAny(exprType, exprResult, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpAssign, Result: decl.Name, Arg1: val.Var,
				})
			}
		}
		return
	}

	if decl.Type == "dict" {
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "dict", Name: decl.Name})
		if irFn.VarTypes != nil {
			irFn.VarTypes[decl.Name] = "dict"
		}
		if irFn.DictKeyTypes == nil {
			irFn.DictKeyTypes = make(map[string]map[string]string)
		}
		irFn.DictKeyTypes[decl.Name] = make(map[string]string)

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
					Op: OpConstNull, Result: decl.Name, ReturnType: "dict",
				})
				return
			}
			if _, isRef := decl.Expr.(*front.Ident); isRef {
				src := p.processExpression(decl.Expr, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpCopy, Result: decl.Name, Arg1: src.Var, ReturnType: "dict",
				})
				return
			}

			exprResult := p.processExpression(decl.Expr, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: decl.Name, Arg1: exprResult.Var,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpConstNull, Result: decl.Name, ReturnType: "dict",
			})
		}
		return
	}

	if decl.IsArray {
		elemType := decl.ElemType
		if elemType == "" {
			elemType = "any"
		}
		arrType := "arr[" + elemType + "]"
		irFn.Locals = append(irFn.Locals, IRLocal{Type: arrType, Name: decl.Name})

		if irFn.VarTypes != nil {
			irFn.VarTypes[decl.Name] = arrType
		}

		if irFn.ArrayElemTypes == nil {
			irFn.ArrayElemTypes = make(map[string]string)
		}
		irFn.ArrayElemTypes[decl.Name] = elemType

		if p.TryDepth > 0 {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:       OpTryRegister,
				Arg1:     decl.Name,
				Metadata: "1",
			})
		}

		if decl.Expr != nil {
			exprType := p.getExprType(decl.Expr, irFn)

			if exprType == "void" {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpConstNull, Result: decl.Name, ReturnType: arrType,
				})
				return
			}

			if _, isRef := decl.Expr.(*front.Ident); isRef {
				src := p.processExpression(decl.Expr, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpCopy, Result: decl.Name, Arg1: src.Var, ReturnType: arrType,
				})
				return
			}

			exprResult := p.processExpressionTyped(decl.Expr, decl.ElemType, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: decl.Name, Arg1: exprResult.Var,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpArrayNew,
				Result:     decl.Name,
				Metadata:   elemType,
				ReturnType: arrType,
			})
		}
		return
	}

	// Скаляр
	irFn.Locals = append(irFn.Locals, IRLocal{Type: decl.Type, Name: decl.Name})
	if irFn.VarTypes != nil {
		irFn.VarTypes[decl.Name] = decl.Type
	}

	if p.TryDepth > 0 && (decl.Type == "string" || decl.Type == "any") {
		kind := "0"
		if decl.Type == "any" {
			kind = "2"
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:       OpTryRegister,
			Arg1:     decl.Name,
			Metadata: kind,
		})
	}

	if decl.Expr != nil {
		exprType := p.getExprType(decl.Expr, irFn)

		if exprType == "void" {
			if decl.Type == "void" || decl.Type == "any" {
				return
			}
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpConstNull, Result: decl.Name, ReturnType: decl.Type,
			})
			return
		}

		exprResult := p.processExpression(decl.Expr, irFn)

		if decl.Type == "any" {
			val := p.wrapToAny(exprType, exprResult, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: decl.Name, Arg1: val.Var,
			})
		} else {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: decl.Name, Arg1: exprResult.Var,
			})
		}
	}
}

func (p *Pipeline) processAssign(assign *front.Assign, irFn *IRFunction) {
	if assign.Expr == nil {
		return
	}

	if assign.Field != "" {
		varType := ""
		if irFn.VarTypes != nil {
			varType = irFn.VarTypes[assign.Name]
		}
		if varType != "dict" {
			p.crash(
				fmt.Sprintf("processAssign: field '%s' assigned on non-dict '%s' (type %q)",
					assign.Field, assign.Name, varType),
				assign.GetLine(), assign.GetColumn(), irFn.File,
			)
			return
		}

		valType := p.getExprType(assign.Expr, irFn)
		val := p.processExpression(assign.Expr, irFn)
		valAny := p.wrapToAny(valType, val, irFn)

		keyVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: keyVar})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstString, Result: keyVar, Arg1: assign.Field, ReturnType: "string",
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpDictSet,
			Arg1: assign.Name,
			Arg2: keyVar,
			Arg3: valAny.Var,
		})
		return
	}

	if assign.Op != "" && assign.Op != "=" {
		p.processCompoundAssign(assign, irFn)
		return
	}

	if assign.Index != nil {
		if irFn.VarTypes != nil {
			if varType, ok := irFn.VarTypes[assign.Name]; ok && varType == "dict" {
				key := p.processExpression(assign.Index, irFn)
				valType := p.getExprType(assign.Expr, irFn)
				val := p.processExpression(assign.Expr, irFn)
				valAny := p.wrapToAny(valType, val, irFn)

				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:   OpDictSet,
					Arg1: assign.Name,
					Arg2: key.Var,
					Arg3: valAny.Var,
				})
				return
			}
		}
		p.processArrayAssign(assign, irFn)
		return
	}

	if irFn.VarTypes != nil {
		if varType, ok := irFn.VarTypes[assign.Name]; ok && utils.IsUnionType(varType) {
			exprType := p.getExprType(assign.Expr, irFn)
			exprResult := p.processExpression(assign.Expr, irFn)

			if exprType == "any" || utils.IsUnionType(exprType) {
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpAssign, Result: assign.Name, Arg1: exprResult.Var,
				})
			} else {
				val := p.wrapToAny(exprType, exprResult, irFn)
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op: OpAssign, Result: assign.Name, Arg1: val.Var,
				})
			}
			return
		}
	}

	exprType := p.getExprType(assign.Expr, irFn)
	varType := p.getVarSkType(assign.Name, irFn)

	if exprType == "void" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: assign.Name, ReturnType: varType,
		})
		return
	}

	exprResult := p.processExpression(assign.Expr, irFn)

	if varType == "any" {
		if exprType == "any" || utils.IsUnionType(exprType) {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: assign.Name, Arg1: exprResult.Var,
			})
		} else {
			val := p.wrapToAny(exprType, exprResult, irFn)
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpAssign, Result: assign.Name, Arg1: val.Var,
			})
		}
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpAssign, Result: assign.Name, Arg1: exprResult.Var,
		})
	}
}

func (p *Pipeline) processCompoundAssign(assign *front.Assign, irFn *IRFunction) {
	op := strings.TrimSuffix(assign.Op, "=")

	skType := p.getVarSkType(assign.Name, irFn)
	if skType == "" {
		skType = "int"
	}

	exprType := p.getExprType(assign.Expr, irFn)
	rightVal := p.processExpression(assign.Expr, irFn)

	if skType == "any" {
		rightAny := p.wrapToAny(exprType, rightVal, irFn)
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: result})
		opcode := opToOpcode(op)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         opcode,
			Result:     result,
			Arg1:       assign.Name,
			Arg2:       rightAny.Var,
			ReturnType: "any",
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpAssign, Result: assign.Name, Arg1: result,
		})
		return
	}

	if skType == "string" && op == "+" {
		rightStr := rightVal
		if exprType == "char" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: tmp})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpCast,
				Result:     tmp,
				Arg1:       rightVal.Var,
				ReturnType: "string",
				SrcType:    "char",
			})
			rightStr = IRValue{Type: "string", Var: tmp}
		}
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpConcat,
			Result:     result,
			Arg1:       assign.Name,
			Arg2:       rightStr.Var,
			ReturnType: "string",
		})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpAssign, Result: assign.Name, Arg1: result,
		})
		return
	}

	if skType == "char" && op == "+" {
		return
	}

	rightTyped := rightVal
	if skType != exprType && exprType != "any" && exprType != "" {
		rightTyped = p.convertToType(rightVal, exprType, skType, irFn)
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: skType, Name: result})
	opcode := opToOpcode(op)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         opcode,
		Result:     result,
		Arg1:       assign.Name,
		Arg2:       rightTyped.Var,
		ReturnType: skType,
	})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op: OpAssign, Result: assign.Name, Arg1: result,
	})
}

func opToOpcode(op string) Opcode {
	switch op {
	case "+":
		return OpAdd
	case "-":
		return OpSub
	case "*":
		return OpMul
	case "/":
		return OpDiv
	case "%":
		return OpMod
	case "**":
		return OpPow
	}
	return OpAdd
}

func (p *Pipeline) getVarSkType(name string, irFn *IRFunction) string {
	if irFn.VarTypes != nil {
		if t, ok := irFn.VarTypes[name]; ok {
			return t
		}
	}
	return ""
}

func (p *Pipeline) convertToType(val IRValue, fromType, toType string, irFn *IRFunction) IRValue {
	if fromType == toType || fromType == "" {
		return val
	}
	tmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: toType, Name: tmp})
	meta := ""
	if fromType == "any" || utils.IsUnionType(fromType) {
		meta = "from_any"
	}
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     tmp,
		Arg1:       val.Var,
		ReturnType: toType,
		Metadata:   meta,
		SrcType:    fromType,
	})
	return IRValue{Type: toType, Var: tmp}
}

// emitConcat — конкатенация строк/символов (string + string → OpConcat,
// char + char → OpConcatCC, char + string → OpConcatCS, string + char → OpConcatSC).
func (p *Pipeline) emitConcat(left, right IRValue, leftType, rightType string, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})

	var opcode Opcode
	switch {
	case leftType == "char" && rightType == "char":
		opcode = OpConcatCC
	case leftType == "char" && rightType == "string":
		opcode = OpConcatCS
	case leftType == "string" && rightType == "char":
		opcode = OpConcatSC
	default:
		opcode = OpConcat
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         opcode,
		Result:     result,
		Arg1:       left.Var,
		Arg2:       right.Var,
		ReturnType: "string",
	})
	return IRValue{Type: "string", Var: result}
}

func (p *Pipeline) processArrayAssign(assign *front.Assign, irFn *IRFunction) {
	elemType := ""
	if irFn.ArrayElemTypes != nil {
		elemType = irFn.ArrayElemTypes[assign.Name]
	}
	if elemType == "" {
		elemType = "any"
	}

	index := p.processExpression(assign.Index, irFn)

	valueType := p.getExprType(assign.Expr, irFn)
	value := p.processExpression(assign.Expr, irFn)

	if elemType == "any" && valueType != "any" {
		value = p.wrapToAny(valueType, value, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpArraySet,
		Arg1: assign.Name,
		Arg2: index.Var,
		Arg3: value.Var,
	})
}

// ============================================================================
// Binary
// ============================================================================

func (p *Pipeline) processNullComparison(bin *front.BinaryExpr, leftType, rightType string, irFn *IRFunction) IRValue {
	var valueNode front.Node
	var valueType string

	if leftType == "void" {
		valueNode = bin.Right
		valueType = rightType
	} else {
		valueNode = bin.Left
		valueType = leftType
	}

	if bin.Op != "==" && bin.Op != "!=" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: result, ReturnType: "bool",
		})
		return IRValue{Type: "bool", Var: result}
	}

	valueVal := p.processExpression(valueNode, irFn)

	if valueType == "char" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		b := bin.Op == "!="
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstBool, Result: result, Arg1: boolLit(b), ReturnType: "bool",
		})
		return IRValue{Type: "bool", Var: result}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})

	if valueType == "any" || utils.IsUnionType(valueType) {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpEq, Result: result, Arg1: valueVal.Var, Arg2: "", ReturnType: "bool",
			Metadata: "any_null",
		})
		return IRValue{Type: "bool", Var: result}
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op: OpEq, Result: result, Arg1: valueVal.Var, Arg2: "", ReturnType: "bool",
		Metadata: "null_check",
	})
	return IRValue{Type: "bool", Var: result}
}

func (p *Pipeline) processBinary(bin *front.BinaryExpr, irFn *IRFunction) IRValue {
	leftType := p.getExprType(bin.Left, irFn)
	rightType := p.getExprType(bin.Right, irFn)

	if leftType == "void" || rightType == "void" {
		return p.processNullComparison(bin, leftType, rightType, irFn)
	}

	if leftType == "char" || rightType == "char" {
		return p.processCharBinary(bin, leftType, rightType, irFn)
	}

	if utils.IsUnionType(leftType) || utils.IsUnionType(rightType) {
		return p.processUnionBinary(bin, leftType, rightType, irFn)
	}

	if bin.Op == "+" && utils.IsArrayType(leftType) {
		if ident, ok := bin.Left.(*front.Ident); ok {
			return p.processArrayAddName(ident.Name, bin.Right, irFn)
		}
	}

	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	if bin.Op == "&&" || bin.Op == "||" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		leftVal := left
		rightVal := right
		if leftType == "any" {
			leftVal = p.unwrapToBool(left, irFn)
		}
		if rightType == "any" {
			rightVal = p.unwrapToBool(right, irFn)
		}
		opcode := OpAnd
		if bin.Op == "||" {
			opcode = OpOr
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: opcode, Result: result, Arg1: leftVal.Var, Arg2: rightVal.Var, ReturnType: "bool",
		})
		return IRValue{Type: "bool", Var: result}
	}

	if bin.Op == "+" &&
		(leftType == "string" || leftType == "char") &&
		(rightType == "string" || rightType == "char") {
		return p.emitConcat(left, right, leftType, rightType, irFn)
	}

	if leftType == "any" || rightType == "any" {
		return p.processAnyBinary(bin, left, right, leftType, rightType, irFn)
	}

	leftT := leftType
	rightT := rightType

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

	if leftT != resultT {
		left = p.convertToType(left, leftT, resultT, irFn)
	}
	if rightT != resultT {
		right = p.convertToType(right, rightT, resultT, irFn)
	}

	if isComparisonOp(bin.Op) {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         cmpOpcode(bin.Op),
			Result:     result,
			Arg1:       left.Var,
			Arg2:       right.Var,
			ReturnType: "bool",
			Metadata:   resultT, // тип операндов
		})
		return IRValue{Type: "bool", Var: result}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: resultT, Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         arithOpcode(bin.Op),
		Result:     result,
		Arg1:       left.Var,
		Arg2:       right.Var,
		ReturnType: resultT,
	})
	return IRValue{Type: resultT, Var: result}
}

func (p *Pipeline) processUnionBinary(bin *front.BinaryExpr, leftType, rightType string, irFn *IRFunction) IRValue {
	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	leftAny := p.wrapToAny(leftType, left, irFn)
	rightAny := p.wrapToAny(rightType, right, irFn)

	if isComparisonOp(bin.Op) {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         cmpOpcode(bin.Op),
			Result:     result,
			Arg1:       leftAny.Var,
			Arg2:       rightAny.Var,
			ReturnType: "bool",
			Metadata:   "any",
		})
		return IRValue{Type: "bool", Var: result}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         arithOpcode(bin.Op),
		Result:     result,
		Arg1:       leftAny.Var,
		Arg2:       rightAny.Var,
		ReturnType: "any",
	})
	return IRValue{Type: "any", Var: result}
}

func (p *Pipeline) processAnyBinary(bin *front.BinaryExpr, left, right IRValue, leftType, rightType string, irFn *IRFunction) IRValue {
	leftAny := p.wrapToAny(leftType, left, irFn)
	rightAny := p.wrapToAny(rightType, right, irFn)

	if isComparisonOp(bin.Op) {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         cmpOpcode(bin.Op),
			Result:     result,
			Arg1:       leftAny.Var,
			Arg2:       rightAny.Var,
			ReturnType: "bool",
			Metadata:   "any",
		})
		return IRValue{Type: "bool", Var: result}
	}

	if bin.Op == "&&" || bin.Op == "||" {
		leftBool := p.unwrapToBool(leftAny, irFn)
		rightBool := p.unwrapToBool(rightAny, irFn)
		opcode := OpAnd
		if bin.Op == "||" {
			opcode = OpOr
		}
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: opcode, Result: result, Arg1: leftBool.Var, Arg2: rightBool.Var, ReturnType: "bool",
		})
		return IRValue{Type: "bool", Var: result}
	}

	if bin.Op == "+" &&
		(leftType == "string" || rightType == "string" ||
			leftType == "char" || rightType == "char") {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpConcatAll,
			Result:     result,
			Arg1:       leftAny.Var,
			Arg2:       rightAny.Var,
			ReturnType: "string",
		})
		return IRValue{Type: "string", Var: result}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         arithOpcode(bin.Op),
		Result:     result,
		Arg1:       leftAny.Var,
		Arg2:       rightAny.Var,
		ReturnType: "any",
	})
	return IRValue{Type: "any", Var: result}
}

func (p *Pipeline) processCharBinary(bin *front.BinaryExpr, leftType, rightType string, irFn *IRFunction) IRValue {
	if bin.Op != "+" || !(leftType == "char" || leftType == "string") || !(rightType == "char" || rightType == "string") {
		errors.NewError("1520",
			fmt.Sprintf("Operation '%s' is not supported for 'char' type "+
				"(only char + char, char + string, string + char are allowed)", bin.Op),
			bin.GetLine(), bin.GetColumn(), irFn.File)
		return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
	}

	left := p.processExpression(bin.Left, irFn)
	right := p.processExpression(bin.Right, irFn)

	var opcode Opcode
	switch {
	case leftType == "char" && rightType == "char":
		opcode = OpConcatCC
	case leftType == "char" && rightType == "string":
		opcode = OpConcatCS
	case leftType == "string" && rightType == "char":
		opcode = OpConcatSC
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         opcode,
		Result:     result,
		Arg1:       left.Var,
		Arg2:       right.Var,
		ReturnType: "string",
	})
	return IRValue{Type: "string", Var: result}
}

func isComparisonOp(op string) bool {
	switch op {
	case "<", ">", "==", "!=", "<=", ">=":
		return true
	}
	return false
}

func cmpOpcode(op string) Opcode {
	switch op {
	case "<":
		return OpLt
	case ">":
		return OpGt
	case "==":
		return OpEq
	case "!=":
		return OpNe
	case "<=":
		return OpLe
	case ">=":
		return OpGe
	}
	return OpEq
}

func arithOpcode(op string) Opcode {
	switch op {
	case "+":
		return OpAdd
	case "-":
		return OpSub
	case "*":
		return OpMul
	case "/":
		return OpDiv
	case "%":
		return OpMod
	case "**":
		return OpPow
	}
	return OpAdd
}

// ============================================================================
// Expression dispatcher
// ============================================================================

func (p *Pipeline) processExpression(expr front.Node, irFn *IRFunction) IRValue {
	switch n := expr.(type) {
	case *front.Number:
		return p.emitNumber(n, irFn)
	case *front.NullLiteral:
		return IRValue{Type: "void", Var: ""}
	case *front.PostfixExpr:
		return p.processPostfixExpr(n, irFn)
	case *front.DictLiteral:
		return p.processDictLiteral(n, irFn)
	case *front.UnicodeLiteral:
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "char", Name: result})
		irFn.Instructions = append(irFn.Instructions, NewConstChar(result, n.Codepoint))
		return IRValue{Type: "char", Var: result}
	case *front.String:
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})
		irFn.Instructions = append(irFn.Instructions, NewConstString(result, n.Value))
		return IRValue{Type: "string", Var: result}
	case *front.CharLiteral:
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "char", Name: result})
		irFn.Instructions = append(irFn.Instructions, NewConstChar(result, uint32(n.Value)))
		return IRValue{Type: "char", Var: result}
	case *front.Ident:
		return p.processIdent(n, irFn)
	case *front.AwaitExpr:
		return p.processAwaitExpr(n, irFn)
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
		p.crash(
			fmt.Sprintf("processExpression: unhandled node %T", expr),
			expr.GetLine(), expr.GetColumn(), irFn.File,
		)
		return IRValue{}
	}
}

func (p *Pipeline) emitNumber(n *front.Number, irFn *IRFunction) IRValue {
	result := p.newTemp()
	if strings.HasSuffix(n.Value, "f") || strings.HasSuffix(n.Value, "F") {
		val := n.Value[:len(n.Value)-1]
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "float", Name: result})
		irFn.Instructions = append(irFn.Instructions, NewConstFloat(result, val))
		return IRValue{Type: "float", Var: result}
	}
	if strings.Contains(n.Value, ".") {
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "double", Name: result})
		irFn.Instructions = append(irFn.Instructions, NewConstDouble(result, n.Value))
		return IRValue{Type: "double", Var: result}
	}
	v, _ := strconv.ParseInt(n.Value, 10, 64)
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: result})
	irFn.Instructions = append(irFn.Instructions, NewConstInt(result, v))
	return IRValue{Type: "int", Var: result}
}

func (p *Pipeline) processIdent(n *front.Ident, irFn *IRFunction) IRValue {
	if n.Name == "true" || n.Name == "false" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, NewConstBool(result, n.Name == "true"))
		return IRValue{Type: "bool", Var: result}
	}

	if handle, ok := p.PromiseOwners[n.Name]; ok {
		skType := irFn.VarTypes[n.Name]
		if skType == "" {
			skType = "int"
		}
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: skType, Name: tmp})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpPromiseRead,
			Result:     tmp,
			Arg1:       handle,
			ReturnType: skType,
		})
		return IRValue{Type: skType, Var: tmp}
	}

	skType := irFn.VarTypes[n.Name]
	if skType == "" {
		for _, g := range p.IR.Globals {
			if g.Name == n.Name {
				skType = g.Type
				break
			}
		}
	}
	if skType == "" {
		skType = "int"
	}
	return IRValue{Type: skType, Var: n.Name}
}

// ============================================================================
// Postfix
// ============================================================================

func (p *Pipeline) processPostfixStatement(pe *front.PostfixExpr, irFn *IRFunction) {
	opcode := OpAdd
	if pe.Op == "--" {
		opcode = OpSub
	}
	one := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: one})
	irFn.Instructions = append(irFn.Instructions, NewConstInt(one, 1))

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         opcode,
		Result:     pe.VarName,
		Arg1:       pe.VarName,
		Arg2:       one,
		ReturnType: "int",
	})
}

func (p *Pipeline) processPostfixExpr(pe *front.PostfixExpr, irFn *IRFunction) IRValue {
	opcode := OpAdd
	if pe.Op == "--" {
		opcode = OpSub
	}
	one := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: one})
	irFn.Instructions = append(irFn.Instructions, NewConstInt(one, 1))

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         opcode,
		Result:     result,
		Arg1:       pe.VarName,
		Arg2:       one,
		ReturnType: "int",
	})
	return IRValue{Type: "int", Var: result}
}

// ============================================================================
// Dict access
// ============================================================================

func (p *Pipeline) processDictAccess(objName, key string, irFn *IRFunction) IRValue {
	keyType := "any"
	if irFn.DictKeyTypes != nil {
		if keyTypes, ok := irFn.DictKeyTypes[objName]; ok {
			if t, ok := keyTypes[key]; ok {
				keyType = t
			}
		}
	}

	keyVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: keyVar})
	irFn.Instructions = append(irFn.Instructions, NewConstString(keyVar, key))

	rawAny := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: rawAny})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpDictGet,
		Result:     rawAny,
		Arg1:       objName,
		Arg2:       keyVar,
		ReturnType: "any",
	})

	if keyType == "any" || keyType == "" {
		return IRValue{Type: "any", Var: rawAny}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: keyType, Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpUnwrapAny,
		Result:     result,
		Arg1:       rawAny,
		ReturnType: keyType,
	})
	return IRValue{Type: keyType, Var: result}
}

func (p *Pipeline) processDictLiteral(d *front.DictLiteral, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "dict", Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpDictNew,
		Result:     result,
		ReturnType: "dict",
	})

	for _, elem := range d.Elements {
		valType := p.getExprType(elem.Value, irFn)
		val := p.processExpression(elem.Value, irFn)
		valAny := p.wrapToAny(valType, val, irFn)

		keyVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: keyVar})
		irFn.Instructions = append(irFn.Instructions, NewConstString(keyVar, elem.Key))

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpDictSet,
			Arg1: result,
			Arg2: keyVar,
			Arg3: valAny.Var,
		})
	}

	return IRValue{Type: "dict", Var: result}
}

// ============================================================================
// Helpers
// ============================================================================

func (p *Pipeline) wrapToAny(t string, val IRValue, irFn *IRFunction) IRValue {
	if t == "any" || utils.IsUnionType(t) {
		return val
	}
	tmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: tmp})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpWrapAny,
		Result:     tmp,
		Arg1:       val.Var,
		ReturnType: "any",
		Metadata:   t,
	})
	return IRValue{Type: "any", Var: tmp}
}

func (p *Pipeline) unwrapToBool(val IRValue, irFn *IRFunction) IRValue {
	tmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: tmp})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpUnwrapAny,
		Result:     tmp,
		Arg1:       val.Var,
		ReturnType: "bool",
	})
	return IRValue{Type: "bool", Var: tmp}
}

// zeroOf — создаёт IR-инструкцию «нулевое значение типа t» и возвращает имя.
// Используется для error-fallback'ов, где нужна переменная.
func (p *Pipeline) zeroOf(t string, irFn *IRFunction) string {
	if t == "void" || t == "" {
		return ""
	}
	tmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: t, Name: tmp})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpConstNull,
		Result:     tmp,
		ReturnType: t,
	})
	return tmp
}

// ============================================================================
// Return
// ============================================================================

func (p *Pipeline) processReturn(ret *front.ReturnStmt, irFn *IRFunction) {
	exprResult := ""
	if ret.Expr != nil {
		exprType := p.getExprType(ret.Expr, irFn)

		if utils.IsUnionType(irFn.ReturnType) {
			expr := p.processExpression(ret.Expr, irFn)
			if exprType == "any" || utils.IsUnionType(exprType) {
				exprResult = expr.Var
			} else {
				val := p.wrapToAny(exprType, expr, irFn)
				exprResult = val.Var
			}
		} else if exprType == "void" {
			exprResult = p.zeroOf(irFn.ReturnType, irFn)
		} else {
			exprResult = p.processExpression(ret.Expr, irFn).Var
		}
	}

	for i := len(p.TryFrames) - 1; i >= 0; i-- {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpTryPop,
			Arg1: p.TryFrames[i],
		})
	}

	if irFn.IsAsync {
		if irFn.ReturnType == "void" {
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpPromiseFinishVoid,
				Arg1:       "__promise",
				ReturnType: "void",
			})
		} else {
			if exprResult == "" {
				exprResult = p.zeroOf(irFn.ReturnType, irFn)
			}
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpPromiseFinish,
				Arg1:       "__promise",
				Arg2:       exprResult,
				ReturnType: irFn.ReturnType,
			})
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{Op: OpRet})
		return
	}

	if exprResult != "" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpRet,
			Arg1: exprResult,
		})
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{Op: OpRet})
	}
}

// ============================================================================
// Call
// ============================================================================

func (p *Pipeline) processFireAndForget(call *front.CallExpr, targetFunc *front.Function, irFn *IRFunction) {
	p.processAsyncCall(call, irFn)
}

func isUnderscoreIdent(node front.Node) bool {
	if node == nil {
		return false
	}
	id, ok := node.(*front.Ident)
	return ok && id.Name == "_"
}

func (p *Pipeline) processCall(call *front.CallExpr, irFn *IRFunction) {
	targetFunc := p.findFunction(call.Name)

	simpleName := call.Name
	if strings.Contains(simpleName, ".") {
		parts := strings.Split(simpleName, ".")
		simpleName = parts[len(parts)-1]
	}

	if targetFunc != nil && targetFunc.IsAsync {
		p.processFireAndForget(call, targetFunc, irFn)
		return
	}

	args := []string{}

	if targetFunc != nil {
		argIndex := 0
		for _, param := range targetFunc.Params {
			if param.IsVariadic {
				variadicArr := p.newTemp()
				irFn.Locals = append(irFn.Locals, IRLocal{Type: "arr[any]", Name: variadicArr})
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         OpArrayNew,
					Result:     variadicArr,
					Metadata:   "any",
					ReturnType: "arr[any]",
				})
				for argIndex < len(call.Args) {
					argExpr := call.Args[argIndex]
					argIndex++
					if isUnderscoreIdent(argExpr) {
						continue
					}
					argType := p.getExprType(argExpr, irFn)
					argVal := p.processExpression(argExpr, irFn)
					argAny := p.wrapToAny(argType, argVal, irFn)
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:   OpArrayPush,
						Arg1: variadicArr,
						Arg2: argAny.Var,
					})
				}
				args = append(args, variadicArr)
				break
			}

			var argExpr front.Node
			if argIndex < len(call.Args) {
				argExpr = call.Args[argIndex]
				argIndex++
				if isUnderscoreIdent(argExpr) {
					argExpr = param.DefaultValue
				}
			} else {
				argExpr = param.DefaultValue
			}
			if argExpr == nil {
				args = append(args, p.zeroOf(param.Type, irFn))
				continue
			}
			args = append(args, p.prepareArg(argExpr, param.Type, irFn).Var)
		}
	} else {
		for _, arg := range call.Args {
			if isUnderscoreIdent(arg) {
				continue
			}
			args = append(args, p.processExpression(arg, irFn).Var)
		}
	}

	argsStr := strings.Join(args, ", ")
	funcName := call.Name
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		funcName = parts[len(parts)-1]
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpCall,
		Arg1:   p.cNameForFunc(funcName),
		Arg2:   argsStr,
		Line:   call.GetLine(),
		Column: call.GetColumn(),
		File:   irFn.File,
	})
}

func (p *Pipeline) processCallExpr(call *front.CallExpr, irFn *IRFunction) IRValue {
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

	simpleName := call.Name
	if strings.Contains(simpleName, ".") {
		parts := strings.Split(simpleName, ".")
		simpleName = parts[len(parts)-1]
	}

	if targetFunc := p.findFunction(simpleName); targetFunc != nil && targetFunc.IsAsync {
		promiseVar := p.processAsyncCall(call, irFn)
		if promiseVar == "" {
			return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
		}
		rawResult := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: targetFunc.ReturnType, Name: rawResult})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpPromiseRead,
			Result:     rawResult,
			Arg1:       promiseVar,
			ReturnType: targetFunc.ReturnType,
		})

		switch targetFunc.ReturnType {
		case "string", "arr", "dict":
			result := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: targetFunc.ReturnType, Name: result})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpCopy,
				Result:     result,
				Arg1:       rawResult,
				ReturnType: targetFunc.ReturnType,
			})
			return IRValue{Type: targetFunc.ReturnType, Var: result}
		default:
			return IRValue{Type: targetFunc.ReturnType, Var: rawResult}
		}
	}

	switch simpleName {
	case "getConfig":
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "dict", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpCall,
			Result:     result,
			Arg1:       "__sk__get_config",
			ReturnType: "dict",
		})
		return IRValue{Type: "dict", Var: result}

	case "isAsyncEnded":
		if len(args) != 1 {
			errors.NewError("0666",
				"isAsyncEnded requires exactly one argument (async function name as string)",
				call.GetLine(), call.GetColumn(), irFn.File)
			return IRValue{Type: "bool", Var: p.zeroOf("bool", irFn)}
		}
		lit, ok := args[0].(*front.String)
		if !ok {
			errors.NewError("0667",
				"isAsyncEnded argument must be a string literal with the async function name",
				call.GetLine(), call.GetColumn(), irFn.File)
			return IRValue{Type: "bool", Var: p.zeroOf("bool", irFn)}
		}
		handle, found := p.PromiseOwners[lit.Value]
		if !found {
			handle = p.findPromiseOwnerForFunc(lit.Value, irFn)
		}
		if handle == "" {
			errors.NewError("0668", fmt.Sprintf("isAsyncEnded(\"%s\"): no async call to '%s' found in this scope",
				lit.Value, lit.Value),
				call.GetLine(), call.GetColumn(), irFn.File)
			return IRValue{Type: "bool", Var: p.zeroOf("bool", irFn)}
		}
		skType := irFn.VarTypes[lit.Value]
		if skType == "" {
			skType = "int"
		}
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpPromiseEnded,
			Result:     result,
			Arg1:       handle,
			ReturnType: "bool",
		})
		return IRValue{Type: "bool", Var: result}

	case "getConfigField":
		if len(args) != 1 {
			break
		}
		lit, ok := args[0].(*front.String)
		if !ok {
			break
		}
		dictVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "dict", Name: dictVar})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpCall,
			Result:     dictVar,
			Arg1:       "__sk__get_config",
			ReturnType: "dict",
		})
		keyVar := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: keyVar})
		irFn.Instructions = append(irFn.Instructions, NewConstString(keyVar, lit.Value))
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpDictGet,
			Result:     result,
			Arg1:       dictVar,
			Arg2:       keyVar,
			ReturnType: "any",
		})
		return IRValue{Type: "any", Var: result}
	}

	switch simpleName {
	case "to_int":
		return p.processToInt(args, irFn)
	case "to_float":
		return p.processToFloat(args, irFn)
	case "to_char":
		return p.processToChar(args, irFn)
	case "to_double":
		return p.processToDouble(args, irFn)
	case "to_string":
		return p.processToString(args, irFn)
	case "to_bool":
		return p.processToBool(args, irFn)
	case "to_arr":
		return p.processToArr(args, irFn)
	case "detruncate":
		return p.processDetruncate(args, irFn)
	}

	targetFunc := p.findFunction(simpleName)

	argsStr := []string{}
	if targetFunc != nil {
		argIndex := 0
		for _, param := range targetFunc.Params {
			if param.IsVariadic {
				variadicArr := p.newTemp()
				irFn.Locals = append(irFn.Locals, IRLocal{Type: "arr[any]", Name: variadicArr})
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         OpArrayNew,
					Result:     variadicArr,
					Metadata:   "any",
					ReturnType: "arr[any]",
				})
				for argIndex < len(args) {
					argExpr := args[argIndex]
					argIndex++
					if isUnderscoreIdent(argExpr) {
						continue
					}
					argType := p.getExprType(argExpr, irFn)
					argVal := p.processExpression(argExpr, irFn)
					argAny := p.wrapToAny(argType, argVal, irFn)
					irFn.Instructions = append(irFn.Instructions, IRInstruction{
						Op:   OpArrayPush,
						Arg1: variadicArr,
						Arg2: argAny.Var,
					})
				}
				argsStr = append(argsStr, variadicArr)
				break
			}

			var argExpr front.Node
			if argIndex < len(args) {
				argExpr = args[argIndex]
				argIndex++
				if isUnderscoreIdent(argExpr) {
					argExpr = param.DefaultValue
				}
			} else {
				argExpr = param.DefaultValue
			}
			if argExpr == nil {
				argsStr = append(argsStr, p.zeroOf(param.Type, irFn))
				continue
			}
			argsStr = append(argsStr, p.prepareArg(argExpr, param.Type, irFn).Var)
		}
	} else {
		for _, arg := range args {
			if isUnderscoreIdent(arg) {
				continue
			}
			argsStr = append(argsStr, p.processExpression(arg, irFn).Var)
		}
	}

	argsJoined := strings.Join(argsStr, ", ")

	returnType := "int"
	found := false
	for _, fn := range p.Program.Functions {
		if fn.Name == simpleName {
			returnType = fn.ReturnType
			found = true
			break
		}
	}
	if !found {
		for _, fn := range p.Program.AllFunctions {
			if fn.Name == simpleName {
				returnType = fn.ReturnType
				break
			}
		}
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: returnType, Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCall,
		Result:     result,
		Arg1:       p.cNameForFunc(simpleName),
		Arg2:       argsJoined,
		ReturnType: returnType,
		Line:       call.GetLine(),
		Column:     call.GetColumn(),
		File:       irFn.File,
	})

	return IRValue{Type: returnType, Var: result}
}

func (p *Pipeline) findPromiseOwnerForFunc(funcName string, irFn *IRFunction) string {
	cName := p.cNameForFunc(funcName)
	for _, ins := range irFn.Instructions {
		if ins.Op == OpAsyncSpawn && ins.Arg1 == cName {
			return ins.Result
		}
	}
	return ""
}

func (p *Pipeline) processDetruncate(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})

	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, NewConstString(result, "void"))
		return IRValue{Type: "string", Var: result}
	}

	arg := args[0]
	declType := p.getDeclaredType(arg, irFn)

	irFn.Instructions = append(irFn.Instructions, NewConstString(result, declType))
	return IRValue{Type: "string", Var: result}
}

func (p *Pipeline) getDeclaredType(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Ident:
		for _, param := range irFn.Params {
			if param.Name == n.Name {
				return param.Type
			}
		}
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

func (p *Pipeline) prepareArg(argExpr front.Node, paramType string, irFn *IRFunction) IRValue {
	argType := p.getExprType(argExpr, irFn)
	argValue := p.processExpression(argExpr, irFn)

	if utils.IsUnionType(paramType) || paramType == "any" {
		if argType == "any" || utils.IsUnionType(argType) {
			return argValue
		}
		if argType == "void" || argType == "" {
			tmp := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: tmp})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op: OpConstNull, Result: tmp, ReturnType: "any",
			})
			return IRValue{Type: "any", Var: tmp}
		}
		return p.wrapToAny(argType, argValue, irFn)
	}

	if argType == "void" {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: paramType, Name: tmp})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: tmp, ReturnType: paramType,
		})
		return IRValue{Type: paramType, Var: tmp}
	}

	if argType == "any" || utils.IsUnionType(argType) {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: paramType, Name: tmp})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpUnwrapAny,
			Result:     tmp,
			Arg1:       argValue.Var,
			ReturnType: paramType,
		})
		return IRValue{Type: paramType, Var: tmp}
	}

	return argValue
}

func (p *Pipeline) findFunction(name string) *front.Function {
	for _, fn := range p.Program.Functions {
		if fn.Name == name {
			return fn
		}
	}
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
		for _, fn := range p.Program.AllFunctions {
			if fn.Name == simpleName {
				return fn
			}
		}
	}
	return nil
}

// ============================================================================
// Control flow: if / case / while / for
// ============================================================================

func (p *Pipeline) processIf(ifStmt *front.IfStmt, irFn *IRFunction) {
	condResult := p.processExpression(ifStmt.Condition, irFn)

	ifLabel := p.newLabel()
	nextLabel := p.newLabel()

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpIf,
		Result: condResult.Var,
		Arg1:   ifLabel,
		Arg2:   nextLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: ifLabel,
	})
	if ifStmt.Then != nil {
		p.processBlock(ifStmt.Then, irFn)
	}

	endLabel := p.newLabel()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpGoto,
		Arg1: endLabel,
	})

	currentLabel := nextLabel
	for _, elsif := range ifStmt.Elsifs {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: currentLabel,
		})

		condResult = p.processExpression(elsif.Condition, irFn)
		elsifLabel := p.newLabel()
		nextLabel = p.newLabel()

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpIf,
			Result: condResult.Var,
			Arg1:   elsifLabel,
			Arg2:   nextLabel,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: elsifLabel,
		})
		if elsif.Then != nil {
			p.processBlock(elsif.Then, irFn)
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpGoto,
			Arg1: endLabel,
		})

		currentLabel = nextLabel
	}

	if ifStmt.Else != nil {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: currentLabel,
		})
		p.processBlock(ifStmt.Else, irFn)
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: currentLabel,
		})
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: endLabel,
	})
}

func (p *Pipeline) processCase(caseStmt *front.CaseStmt, irFn *IRFunction) {
	value := p.processExpression(caseStmt.Value, irFn)
	valueType := p.getExprType(caseStmt.Value, irFn)

	if valueType == "any" {
		value = p.unwrapToInt(value, irFn)
		valueType = "int"
	}

	endLabel := p.newLabel()

	for _, branch := range caseStmt.Branches {
		branchLabel := p.newLabel()
		nextLabel := p.newLabel()

		var combinedCheck string

		for i, patNode := range branch.Patterns {
			pattern := p.processExpression(patNode, irFn)
			patternType := p.getExprType(patNode, irFn)

			if patternType == "any" {
				pattern = p.unwrapToInt(pattern, irFn)
			}

			cmpVar := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: cmpVar})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpEq,
				Result:     cmpVar,
				Arg1:       value.Var,
				Arg2:       pattern.Var,
				ReturnType: "bool",
				Metadata:   valueType,
			})

			if i == 0 {
				combinedCheck = cmpVar
			} else {
				orVar := p.newTemp()
				irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: orVar})
				irFn.Instructions = append(irFn.Instructions, IRInstruction{
					Op:         OpOr,
					Result:     orVar,
					Arg1:       combinedCheck,
					Arg2:       cmpVar,
					ReturnType: "bool",
				})
				combinedCheck = orVar
			}
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpIf,
			Result: combinedCheck,
			Arg1:   branchLabel,
			Arg2:   nextLabel,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: branchLabel,
		})

		if branch.Body != nil {
			p.processBlock(branch.Body, irFn)
		}

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpGoto,
			Arg1: endLabel,
		})

		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: nextLabel,
		})
	}

	if caseStmt.Default != nil {
		defaultLabel := p.newLabel()
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     OpLabel,
			Result: defaultLabel,
		})
		p.processBlock(caseStmt.Default, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: endLabel,
	})
}

func (p *Pipeline) unwrapToInt(val IRValue, irFn *IRFunction) IRValue {
	tmp := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: tmp})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpUnwrapAny,
		Result:     tmp,
		Arg1:       val.Var,
		ReturnType: "int",
	})
	return IRValue{Type: "int", Var: tmp}
}

func (p *Pipeline) processWhile(while *front.WhileStmt, irFn *IRFunction) {
	startLabel := p.newLabel()
	bodyLabel := p.newLabel()
	endLabel := p.newLabel()

	p.LoopStack = append(p.LoopStack, LoopLabels{Start: startLabel, End: endLabel})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: startLabel,
	})

	condResult := p.processExpression(while.Condition, irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpIf,
		Result: condResult.Var,
		Arg1:   bodyLabel,
		Arg2:   endLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: bodyLabel,
	})

	if while.Body != nil {
		p.processBlock(while.Body, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpGoto,
		Arg1: startLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: endLabel,
	})

	p.LoopStack = p.LoopStack[:len(p.LoopStack)-1]
}

func (p *Pipeline) processForIn(forIn *front.ForInStmt, irFn *IRFunction) {
	iterType := p.getExprType(forIn.Iterable, irFn)
	iterVal := p.processExpression(forIn.Iterable, irFn)

	idxVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: idxVar})
	irFn.Instructions = append(irFn.Instructions, NewConstInt(idxVar, 0))

	irFn.Locals = append(irFn.Locals, IRLocal{Type: forIn.VarType, Name: forIn.VarName})
	if irFn.VarTypes == nil {
		irFn.VarTypes = make(map[string]string)
	}
	irFn.VarTypes[forIn.VarName] = forIn.VarType

	oneVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: oneVar})
	irFn.Instructions = append(irFn.Instructions, NewConstInt(oneVar, 1))

	startLabel := p.newLabel()
	bodyLabel := p.newLabel()
	endLabel := p.newLabel()

	p.LoopStack = append(p.LoopStack, LoopLabels{Start: startLabel, End: endLabel})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: startLabel,
	})

	lenVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: lenVar})

	if iterType == "string" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpArrayLen,
			Result:     lenVar,
			Arg1:       iterVal.Var,
			ReturnType: "int",
			Metadata:   "string_utf8",
		})
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpArrayLen,
			Result:     lenVar,
			Arg1:       iterVal.Var,
			ReturnType: "int",
		})
	}

	condVar := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: condVar})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpLt,
		Result:     condVar,
		Arg1:       idxVar,
		Arg2:       lenVar,
		ReturnType: "bool",
		Metadata:   "int",
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpIf,
		Result: condVar,
		Arg1:   bodyLabel,
		Arg2:   endLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: bodyLabel,
	})

	if iterType == "string" {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpArrayGet,
			Result:     forIn.VarName,
			Arg1:       iterVal.Var,
			Arg2:       idxVar,
			ReturnType: "string",
			Metadata:   "string_utf8",
		})
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpArrayGet,
			Result:     forIn.VarName,
			Arg1:       iterVal.Var,
			Arg2:       idxVar,
			ReturnType: forIn.VarType,
		})
	}

	if forIn.Body != nil {
		p.processBlock(forIn.Body, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpAdd,
		Result:     idxVar,
		Arg1:       idxVar,
		Arg2:       oneVar,
		ReturnType: "int",
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   OpGoto,
		Arg1: startLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: endLabel,
	})

	p.LoopStack = p.LoopStack[:len(p.LoopStack)-1]
}

func (p *Pipeline) processDoWhile(dw *front.DoWhileStmt, irFn *IRFunction) {
	startLabel := p.newLabel()
	bodyLabel := p.newLabel()
	condLabel := p.newLabel()
	endLabel := p.newLabel()

	p.LoopStack = append(p.LoopStack, LoopLabels{Start: condLabel, End: endLabel})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: startLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: bodyLabel,
	})

	if dw.Body != nil {
		p.processBlock(dw.Body, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: condLabel,
	})

	condResult := p.processExpression(dw.Condition, irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpIf,
		Result: condResult.Var,
		Arg1:   startLabel,
		Arg2:   endLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     OpLabel,
		Result: endLabel,
	})

	p.LoopStack = p.LoopStack[:len(p.LoopStack)-1]
}

// ============================================================================
// Array index / length / add
// ============================================================================

func (p *Pipeline) processArrayIndex(idx *front.ArrayIndex, irFn *IRFunction) IRValue {
	if irFn.VarTypes != nil {
		if objType, ok := irFn.VarTypes[idx.Name]; ok && objType == "dict" {
			index := p.processExpression(idx.Index, irFn)
			rawAny := p.newTemp()
			irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: rawAny})
			irFn.Instructions = append(irFn.Instructions, IRInstruction{
				Op:         OpDictGet,
				Result:     rawAny,
				Arg1:       idx.Name,
				Arg2:       index.Var,
				ReturnType: "any",
			})
			return IRValue{Type: "any", Var: rawAny}
		}
	}

	elemType := ""
	if irFn.ArrayElemTypes != nil {
		elemType = irFn.ArrayElemTypes[idx.Name]
	}
	if elemType == "" {
		elemType = "any"
	}

	result := p.newTemp()
	index := p.processExpression(idx.Index, irFn)

	if elemType == "any" {
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "any", Name: result})
	} else if utils.IsArrayType(elemType) {
		irFn.Locals = append(irFn.Locals, IRLocal{Type: elemType, Name: result})
	} else {
		irFn.Locals = append(irFn.Locals, IRLocal{Type: elemType, Name: result})
	}
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpArrayGet,
		Result:     result,
		Arg1:       idx.Name,
		Arg2:       index.Var,
		ReturnType: elemType,
	})
	return IRValue{Type: elemType, Var: result}
}

func (p *Pipeline) processArrayLength(length *front.ArrayLength, irFn *IRFunction) IRValue {
	varType := p.getVarSkType(length.Name, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: result})

	meta := ""
	if varType == "string" {
		meta = "string_utf8"
	} else if varType == "dict" {
		meta = "dict"
	}
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpArrayLen,
		Result:     result,
		Arg1:       length.Name,
		ReturnType: "int",
		Metadata:   meta,
	})
	return IRValue{Type: "int", Var: result}
}

func (p *Pipeline) processArrayAdd(add *front.ArrayAdd, irFn *IRFunction) IRValue {
	elemType := p.getExprType(add.Elem, irFn)
	elemVal := p.processExpression(add.Elem, irFn)

	return p.emitArrayAdd(add.Name, elemType, elemVal, irFn)
}

func (p *Pipeline) processArrayAddName(name string, elemExpr front.Node, irFn *IRFunction) IRValue {
	elemType := p.getExprType(elemExpr, irFn)
	elemVal := p.processExpression(elemExpr, irFn)

	return p.emitArrayAdd(name, elemType, elemVal, irFn)
}

func (p *Pipeline) emitArrayAdd(arrName, elemType string, elemVal IRValue, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "arr", Name: result})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCopy,
		Result:     result,
		Arg1:       arrName,
		ReturnType: "arr",
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:       OpArrayPush,
		Arg1:     result,
		Arg2:     elemVal.Var,
		Metadata: elemType,
	})
	return IRValue{Type: "arr", Var: result}
}

// ============================================================================
// Built-in casts
// ============================================================================

func (p *Pipeline) processToInt(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: result})
	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: result, ReturnType: "int",
		})
		return IRValue{Type: "int", Var: result}
	}
	arg := p.processExpression(args[0], irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     result,
		Arg1:       arg.Var,
		ReturnType: "int",
		SrcType:    arg.Type,
	})
	return IRValue{Type: "int", Var: result}
}

func (p *Pipeline) processToFloat(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "float", Name: result})
	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: result, ReturnType: "float",
		})
		return IRValue{Type: "float", Var: result}
	}
	arg := p.processExpression(args[0], irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     result,
		Arg1:       arg.Var,
		ReturnType: "float",
		SrcType:    arg.Type,
	})
	return IRValue{Type: "float", Var: result}
}

func (p *Pipeline) processToDouble(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "double", Name: result})
	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: result, ReturnType: "double",
		})
		return IRValue{Type: "double", Var: result}
	}
	arg := p.processExpression(args[0], irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     result,
		Arg1:       arg.Var,
		ReturnType: "double",
		SrcType:    arg.Type,
	})
	return IRValue{Type: "double", Var: result}
}

func (p *Pipeline) processToString(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "string", Name: result})
	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, NewConstString(result, ""))
		return IRValue{Type: "string", Var: result}
	}
	arg := p.processExpression(args[0], irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     result,
		Arg1:       arg.Var,
		ReturnType: "string",
		SrcType:    arg.Type,
	})
	return IRValue{Type: "string", Var: result}
}

func (p *Pipeline) processToChar(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "char", Name: result})
	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, NewConstChar(result, 0))
		return IRValue{Type: "char", Var: result}
	}
	arg := p.processExpression(args[0], irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     result,
		Arg1:       arg.Var,
		ReturnType: "char",
		SrcType:    arg.Type,
	})
	return IRValue{Type: "char", Var: result}
}

func (p *Pipeline) processToBool(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "bool", Name: result})
	if len(args) == 0 {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op: OpConstNull, Result: result, ReturnType: "bool",
		})
		return IRValue{Type: "bool", Var: result}
	}
	arg := p.processExpression(args[0], irFn)
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCast,
		Result:     result,
		Arg1:       arg.Var,
		ReturnType: "bool",
		SrcType:    arg.Type,
	})
	return IRValue{Type: "bool", Var: result}
}

func (p *Pipeline) processToArr(args []front.Node, irFn *IRFunction) IRValue {
	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "arr[any]", Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpArrayNew,
		Result:     result,
		Metadata:   "any",
		ReturnType: "arr[any]",
	})
	for _, arg := range args {
		argType := p.getExprType(arg, irFn)
		val := p.processExpression(arg, irFn)
		valAny := p.wrapToAny(argType, val, irFn)
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:   OpArrayPush,
			Arg1: result,
			Arg2: valAny.Var,
		})
	}
	return IRValue{Type: "arr[any]", Var: result}
}

// ============================================================================
// Types
// ============================================================================

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
	case *front.AwaitExpr:
		switch e := n.Expr.(type) {
		case *front.CallExpr:
			simpleName := e.Name
			if strings.Contains(simpleName, ".") {
				parts := strings.Split(simpleName, ".")
				simpleName = parts[len(parts)-1]
			}
			if fn := p.findFunction(simpleName); fn != nil {
				return fn.ReturnType
			}
		case *front.Ident:
			if t, ok := irFn.VarTypes[e.Name]; ok {
				return t
			}
		}
		return ""
	case *front.PromiseSet:
		if irFn.IsAsync {
			return irFn.ReturnType
		}
		return ""
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
		if irFn.VarTypes != nil {
			if t, ok := irFn.VarTypes[n.Name]; ok {
				return t
			}
		}
		for _, g := range p.IR.Globals {
			if g.Name == n.Name {
				return g.Type
			}
		}
		return ""
	case *front.BinaryExpr:
		leftType := p.getExprType(n.Left, irFn)
		rightType := p.getExprType(n.Right, irFn)

		if isComparisonOp(n.Op) {
			return "bool"
		}
		if n.Op == "&&" || n.Op == "||" {
			return "bool"
		}

		if utils.IsUnionType(leftType) || utils.IsUnionType(rightType) {
			return "any"
		}
		if leftType == "any" || rightType == "any" {
			return "any"
		}

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
		case "getConfig":
			return "dict"
		case "getConfigField":
			return "any"
		}
		for _, fn := range p.Program.Functions {
			if fn.Name == simpleName {
				return fn.ReturnType
			}
		}
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
				return elemType
			}
		}
		return ""
	case *front.ArrayLength:
		return "int"
	case *front.ArrayAdd:
		return "arr"
	}
	return ""
}

func (p *Pipeline) newTemp() string {
	p.TempCounter++
	return fmt.Sprintf("t%d", p.TempCounter)
}

func (p *Pipeline) newLabel() string {
	p.LabelCounter++
	return fmt.Sprintf("L%d", p.LabelCounter)
}

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

func (p *Pipeline) cNameForFunc(name string) string {
	for _, fn := range p.Program.Functions {
		if fn.Name == name {
			return computeCName(fn.Name)
		}
	}
	for _, fn := range p.Program.AllFunctions {
		if fn.Name == name {
			return computeCName(fn.Name)
		}
	}
	return computeCName(name)
}

// ============================================================================
// Ternary / format / expressionTyped
// ============================================================================

func (p *Pipeline) processTernary(t *front.TernaryExpr, irFn *IRFunction) IRValue {
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
	irFn.Locals = append(irFn.Locals, IRLocal{Type: resultType, Name: result})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         "ternary",
		Result:     result,
		Arg1:       cond.Var,
		Arg2:       thenVal.Var,
		Arg3:       elseVal.Var,
		ReturnType: resultType,
	})

	return IRValue{Type: resultType, Var: result}
}

func (p *Pipeline) processFormatExpr(fe *front.FormatExpr, irFn *IRFunction) IRValue {
	exprType := p.getExprType(fe.Expr, irFn)
	if exprType == "int" {
		return p.processExpression(fe.Expr, irFn)
	}

	if exprType != "float" && exprType != "double" {
		p.crash(
			fmt.Sprintf("processFormatExpr: unsupported type %q for ^%s (expr %T)",
				exprType, fe.Mode, fe.Expr),
			fe.GetLine(), fe.GetColumn(), irFn.File,
		)
		return IRValue{}
	}
	expr := p.processExpression(fe.Expr, irFn)

	if fe.Mode == "Nf" && fe.N == 0 {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpFormatTrunc,
			Result:     result,
			Arg1:       expr.Var,
			ReturnType: "int",
			Metadata:   exprType,
		})
		return IRValue{Type: "int", Var: result}
	}

	if fe.Mode == "Nf" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: exprType, Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpFormatPrec,
			Result:     result,
			Arg1:       expr.Var,
			Arg2:       strconv.Itoa(fe.N),
			ReturnType: exprType,
		})
		return IRValue{Type: exprType, Var: result}
	}

	if fe.Mode == "X" {
		result := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: exprType, Name: result})
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:         OpFormatTrim,
			Result:     result,
			Arg1:       expr.Var,
			ReturnType: exprType,
			Metadata:   fe.TrimSet,
		})
		return IRValue{Type: exprType, Var: result}
	}

	return expr
}

func (p *Pipeline) processExpressionTyped(expr front.Node, expectedType string, irFn *IRFunction) IRValue {
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

func (p *Pipeline) processArrayLiteralTyped(lit *front.ArrayLiteral, expectedElemType string, irFn *IRFunction) IRValue {
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

	if expectedElemType == "" {
		expectedElemType = "any"
	}
	arrType := "arr[" + expectedElemType + "]"

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: arrType, Name: result})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpArrayNew,
		Result:     result,
		Metadata:   expectedElemType,
		ReturnType: arrType,
	})

	innerElemType := utils.ParseArrayElemType(expectedElemType)

	for _, elem := range expandedElements {
		val := p.processExpressionTyped(elem, innerElemType, irFn)

		if expectedElemType == "any" {
			elemTypeName := p.getExprType(elem, irFn)
			val = p.wrapToAny(elemTypeName, val, irFn)
		} else if p.getExprType(elem, irFn) == "void" {
			val = IRValue{Type: expectedElemType, Var: p.zeroOf(expectedElemType, irFn)}
		}
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:       OpArrayPush,
			Arg1:     result,
			Arg2:     val.Var,
			Metadata: expectedElemType,
		})
	}

	return IRValue{Type: arrType, Var: result}
}

func (p *Pipeline) processRange(r *front.RangeExpr, irFn *IRFunction) IRValue {
	start := p.processExpression(r.Start, irFn)
	end := p.processExpression(r.End, irFn)

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: "arr[int]", Name: result})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpRangeNew,
		Result:     result,
		Arg1:       start.Var,
		Arg2:       end.Var,
		ReturnType: "arr[int]",
	})
	return IRValue{Type: "arr[int]", Var: result}
}

func (p *Pipeline) processCallRange(call *front.CallRangeExpr, irFn *IRFunction) IRValue {
	targetFunc := p.findFunction(call.Name)
	if targetFunc == nil {
		return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
	}

	startVal := p.getConstantInt(call.Range.Start, irFn)
	endVal := p.getConstantInt(call.Range.End, irFn)

	if startVal < 0 || endVal < 0 {
		return IRValue{Type: "int", Var: p.zeroOf("int", irFn)}
	}

	args := []string{}
	for i := startVal; i <= endVal; i++ {
		tmp := p.newTemp()
		irFn.Locals = append(irFn.Locals, IRLocal{Type: "int", Name: tmp})
		irFn.Instructions = append(irFn.Instructions, NewConstInt(tmp, int64(i)))
		args = append(args, tmp)
	}
	for _, extra := range call.Extra {
		args = append(args, p.processExpression(extra, irFn).Var)
	}

	argsStr := strings.Join(args, ", ")

	funcName := call.Name
	if strings.Contains(funcName, ".") {
		parts := strings.Split(funcName, ".")
		funcName = parts[len(parts)-1]
	}

	result := p.newTemp()
	irFn.Locals = append(irFn.Locals, IRLocal{Type: targetFunc.ReturnType, Name: result})
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:         OpCall,
		Result:     result,
		Arg1:       p.cNameForFunc(funcName),
		Arg2:       argsStr,
		ReturnType: targetFunc.ReturnType,
		Line:       call.GetLine(),
		Column:     call.GetColumn(),
		File:       irFn.File,
	})
	return IRValue{Type: targetFunc.ReturnType, Var: result}
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

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (p *Pipeline) crash(msg string, line, col int, file string) {
	panicVal := fmt.Sprintf("%s at %s:%d:%d (pipeline)", msg, file, line, col)
	log.PrintCrashBanner(os.Stderr, panicVal, version)
	os.Exit(70)
}
