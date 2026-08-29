package backend

import (
	"fmt"
	"skrp/res/front"
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

	// 2. Обрабатываем функции
	for _, fn := range p.Program.Functions {
		p.processFunction(fn)
	}

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
			Op:     "$",
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

	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     bin.Op,
		Result: result,
		Arg1:   left,
		Arg2:   right,
	})

	return result
}

func (p *Pipeline) processExpression(expr front.Node, irFn *IRFunction) string {
	switch n := expr.(type) {
	case *front.Number:
		return n.Value
	case *front.String:
		// Строка уже содержит кавычки из лексера, но нужно экранировать
		// В лексере строка читается без кавычек, добавляем их здесь
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

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:   "call",
		Arg1: call.Name,
		Arg2: argsStr,
	})
}

func (p *Pipeline) processCallExpr(call *front.CallExpr, irFn *IRFunction) string {
	args := []string{}
	for _, arg := range call.Args {
		exprResult := p.processExpression(arg, irFn)
		// Если это строка, она уже содержит кавычки
		args = append(args, exprResult)
	}

	argsStr := ""
	if len(args) > 0 {
		argsStr = args[0]
		for i := 1; i < len(args); i++ {
			argsStr += ", " + args[i]
		}
	}

	result := p.newTemp()
	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "call",
		Result: result,
		Arg1:   call.Name,
		Arg2:   argsStr,
	})

	return result
}

func (p *Pipeline) processIf(ifStmt *front.IfStmt, irFn *IRFunction) {
	condResult := p.processExpression(ifStmt.Condition, irFn)

	thenLabel := p.newLabel()
	endLabel := p.newLabel()

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "if",
		Result: condResult,
		Arg1:   thenLabel,
		Arg2:   endLabel,
	})

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "label",
		Result: thenLabel,
	})

	if ifStmt.Then != nil {
		p.processBlock(ifStmt.Then, irFn)
	}

	irFn.Instructions = append(irFn.Instructions, IRInstruction{
		Op:     "goto",
		Result: endLabel,
	})

	if ifStmt.Else != nil {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: endLabel,
		})
		p.processBlock(ifStmt.Else, irFn)
	} else {
		irFn.Instructions = append(irFn.Instructions, IRInstruction{
			Op:     "label",
			Result: endLabel,
		})
	}
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
