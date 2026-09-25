package midlevel

import (
	"strconv"

	"skrp/res/front"
)

type Optimizer struct {
	Program *front.Program
	Changed bool
}

func NewOptimizer(prog *front.Program) *Optimizer {
	return &Optimizer{Program: prog}
}

func (o *Optimizer) Optimize() *front.Program {
	for {
		o.Changed = false

		for _, fn := range o.Program.Functions {
			if fn.Body != nil {
				fn.Body = o.optimizeBlock(fn.Body)
				o.removeUnusedVariables(fn)
			}
		}

		if !o.Changed {
			break
		}
	}

	return o.Program
}

func (o *Optimizer) optimizeBlock(block *front.Block) *front.Block {
	if block == nil {
		return block
	}

	// 1. Рекурсивно оптимизируем каждый statement
	newStatements := []front.Node{}
	for _, stmt := range block.Statements {
		opt := o.optimizeNode(stmt)
		if opt != nil {
			newStatements = append(newStatements, opt)
		}
	}

	// 2. Удаляем мёртвый код (после return)
	finalStatements := []front.Node{}
	deadCode := false
	for _, stmt := range newStatements {
		if deadCode {
			o.Changed = true
			continue
		}

		finalStatements = append(finalStatements, stmt)

		if _, ok := stmt.(*front.ReturnStmt); ok {
			deadCode = true
		}
	}

	block.Statements = finalStatements
	return block
}

func (o *Optimizer) optimizeNode(node front.Node) front.Node {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *front.Block:
		return o.optimizeBlock(n)

	case *front.IfStmt:
		return o.optimizeIf(n)

	case *front.WhileStmt:
		return o.optimizeWhile(n)

	case *front.ForStmt:
		return o.optimizeFor(n)

	case *front.TernaryExpr:
		return o.optimizeTernary(n)

	case *front.CaseStmt:
		return o.optimizeCase(n)

	case *front.BinaryExpr:
		return o.optimizeBinary(n)

	case *front.UnaryExpr:
		return o.optimizeUnary(n)

	case *front.CallExpr:
		return o.optimizeCall(n)

	case *front.VarDecl:
		if n.Expr != nil {
			n.Expr = o.optimizeNode(n.Expr)
		}
		return n

	case *front.Assign:
		if n.Expr != nil {
			n.Expr = o.optimizeNode(n.Expr)
		}
		return n

	case *front.ReturnStmt:
		if n.Expr != nil {
			n.Expr = o.optimizeNode(n.Expr)
		}
		return n

	case *front.ArrayIndex:
		n.Index = o.optimizeNode(n.Index)
		return n

	case *front.ArrayAdd:
		n.Elem = o.optimizeNode(n.Elem)
		return n

	case *front.ArrayLiteral:
		for i, elem := range n.Elements {
			n.Elements[i] = o.optimizeNode(elem)
		}
		return n

	case *front.TypeOf:
		n.Expr = o.optimizeNode(n.Expr)
		return n

	default:
		return node
	}
}

func (o *Optimizer) optimizeIf(ifStmt *front.IfStmt) front.Node {
	if ifStmt.Then != nil {
		ifStmt.Then = o.optimizeBlock(ifStmt.Then)
	}
	for i, elsif := range ifStmt.Elsifs {
		if elsif.Then != nil {
			elsif.Then = o.optimizeBlock(elsif.Then)
		}
		if elsif.Condition != nil {
			ifStmt.Elsifs[i].Condition = o.optimizeNode(elsif.Condition)
		}
	}
	if ifStmt.Else != nil {
		ifStmt.Else = o.optimizeBlock(ifStmt.Else)
	}

	if ifStmt.Condition != nil {
		ifStmt.Condition = o.optimizeNode(ifStmt.Condition)
	}

	// Постоянное условие
	if val, ok := o.getBoolConstant(ifStmt.Condition); ok {
		o.Changed = true

		if val {
			// if (true) { ... } → заменяем на содержимое then
			result := &front.Block{Statements: []front.Node{}}
			if ifStmt.Then != nil {
				result.Statements = append(result.Statements, ifStmt.Then.Statements...)
			}
			return result
		} else {
			// if (false) { ... } — проверяем elsif
			for _, elsif := range ifStmt.Elsifs {
				if val2, ok2 := o.getBoolConstant(elsif.Condition); ok2 {
					if val2 {
						return elsif.Then
					}
					continue
				}
				// Первый не-константный elsif — оставляем его как if
				newIf := &front.IfStmt{
					Condition: elsif.Condition,
					Then:      elsif.Then,
					Else:      ifStmt.Else,
				}
				return o.optimizeIf(newIf)
			}
			// Все elsif константные false → возвращаем else
			if ifStmt.Else != nil {
				return ifStmt.Else
			}
			return &front.Block{Statements: []front.Node{}}
		}
	}

	return ifStmt
}

func (o *Optimizer) optimizeWhile(while *front.WhileStmt) front.Node {
	if while.Condition != nil {
		while.Condition = o.optimizeNode(while.Condition)
	}
	if while.Body != nil {
		while.Body = o.optimizeBlock(while.Body)
	}

	// while (false) → ничего
	if val, ok := o.getBoolConstant(while.Condition); ok && !val {
		o.Changed = true
		return &front.Block{Statements: []front.Node{}}
	}

	return while
}

func (o *Optimizer) optimizeFor(forStmt *front.ForStmt) front.Node {
	if forStmt.Init != nil {
		forStmt.Init = o.optimizeNode(forStmt.Init)
	}
	if forStmt.Cond != nil {
		forStmt.Cond = o.optimizeNode(forStmt.Cond)
	}
	if forStmt.Post != nil {
		forStmt.Post = o.optimizeNode(forStmt.Post)
	}
	if forStmt.Body != nil {
		forStmt.Body = o.optimizeBlock(forStmt.Body)
	}

	// for (init; false; post) → init
	if forStmt.Cond != nil {
		if val, ok := o.getBoolConstant(forStmt.Cond); ok && !val {
			o.Changed = true
			result := &front.Block{Statements: []front.Node{}}
			if forStmt.Init != nil {
				result.Statements = append(result.Statements, forStmt.Init)
			}
			return result
		}
	}

	return forStmt
}

func (o *Optimizer) optimizeCase(caseStmt *front.CaseStmt) front.Node {
	if caseStmt.Value != nil {
		caseStmt.Value = o.optimizeNode(caseStmt.Value)
	}

	for i, branch := range caseStmt.Branches {
		if branch.Pattern != nil {
			caseStmt.Branches[i].Pattern = o.optimizeNode(branch.Pattern)
		}
		if branch.Body != nil {
			caseStmt.Branches[i].Body = o.optimizeBlock(branch.Body)
		}
	}

	if caseStmt.Default != nil {
		caseStmt.Default = o.optimizeBlock(caseStmt.Default)
	}

	// Если value — константа, можно выбрать одну ветку
	if _, isNum := caseStmt.Value.(*front.Number); isNum {
		if _, isStr := caseStmt.Value.(*front.String); isStr {
			// fallthrough
		}
	}

	// Если value — числовая или строковая константа
	if o.isConstant(caseStmt.Value) {
		val := o.getConstantValue(caseStmt.Value)
		if val != "" {
			for _, branch := range caseStmt.Branches {
				if o.isConstant(branch.Pattern) {
					pat := o.getConstantValue(branch.Pattern)
					if pat == val {
						o.Changed = true
						return branch.Body
					}
				} else {
					// Первый не-константный паттерн — оставляем как есть
					break
				}
			}
			// Ни один константный паттерн не совпал
			if caseStmt.Default != nil {
				o.Changed = true
				return caseStmt.Default
			}
		}
	}

	return caseStmt
}

func (o *Optimizer) optimizeBinary(bin *front.BinaryExpr) front.Node {
	// Сначала оптимизируем операнды
	if bin.Left != nil {
		bin.Left = o.optimizeNode(bin.Left)
	}
	if bin.Right != nil {
		bin.Right = o.optimizeNode(bin.Right)
	}

	// 1. Constant folding
	if result := o.tryFoldConstants(bin); result != nil {
		o.Changed = true
		return result
	}

	// 2. Упрощение: x * 1, x + 0, x - 0, x * 0, x / 1
	if simplified := o.trySimplify(bin); simplified != nil {
		o.Changed = true
		return simplified
	}

	return bin
}

func (o *Optimizer) tryFoldConstants(bin *front.BinaryExpr) front.Node {
	leftNum, leftIsNum := bin.Left.(*front.Number)
	rightNum, rightIsNum := bin.Right.(*front.Number)

	// Числа
	if leftIsNum && rightIsNum {
		leftVal, _ := strconv.ParseFloat(leftNum.Value, 64)
		rightVal, _ := strconv.ParseFloat(rightNum.Value, 64)

		var result float64
		var isInt bool

		// Проверяем, что оба операнда — int
		leftInt, leftErr := strconv.Atoi(leftNum.Value)
		rightInt, rightErr := strconv.Atoi(rightNum.Value)
		isInt = leftErr == nil && rightErr == nil

		switch bin.Op {
		case "+":
			if isInt {
				result = float64(leftInt + rightInt)
			} else {
				result = leftVal + rightVal
			}
		case "-":
			if isInt {
				result = float64(leftInt - rightInt)
			} else {
				result = leftVal - rightVal
			}
		case "*":
			if isInt {
				result = float64(leftInt * rightInt)
			} else {
				result = leftVal * rightVal
			}
		case "/":
			if rightVal == 0 {
				return nil // деление на ноль — не сворачиваем
			}
			if isInt {
				result = float64(leftInt / rightInt)
			} else {
				result = leftVal / rightVal
			}
		case "<":
			if isInt {
				if leftInt < rightInt {
					return &front.Ident{Name: "true"}
				}
				return &front.Ident{Name: "false"}
			}
			if leftVal < rightVal {
				return &front.Ident{Name: "true"}
			}
			return &front.Ident{Name: "false"}
		case ">":
			if isInt {
				if leftInt > rightInt {
					return &front.Ident{Name: "true"}
				}
				return &front.Ident{Name: "false"}
			}
			if leftVal > rightVal {
				return &front.Ident{Name: "true"}
			}
			return &front.Ident{Name: "false"}
		default:
			return nil
		}

		if isInt {
			return &front.Number{Value: strconv.Itoa(int(result))}
		}
		return &front.Number{Value: strconv.FormatFloat(result, 'f', -1, 64)}
	}

	// Строки
	leftStr, leftIsStr := bin.Left.(*front.String)
	rightStr, rightIsStr := bin.Right.(*front.String)

	if leftIsStr && rightIsStr && bin.Op == "+" {
		return &front.String{Value: leftStr.Value + rightStr.Value}
	}

	// Строка + число / число + строка
	if bin.Op == "+" {
		if leftIsStr && rightIsNum {
			return &front.String{Value: leftStr.Value + rightNum.Value}
		}
		if leftIsNum && rightIsStr {
			return &front.String{Value: leftNum.Value + rightStr.Value}
		}
	}

	return nil
}

func (o *Optimizer) trySimplify(bin *front.BinaryExpr) front.Node {
	rightNum, rightIsNum := bin.Right.(*front.Number)
	leftNum, leftIsNum := bin.Left.(*front.Number)

	// x + 0 → x, x - 0 → x
	if rightIsNum && (rightNum.Value == "0" || rightNum.Value == "0.0") {
		if bin.Op == "+" || bin.Op == "-" {
			return bin.Left
		}
	}

	// 0 + x → x
	if leftIsNum && (leftNum.Value == "0" || leftNum.Value == "0.0") {
		if bin.Op == "+" {
			return bin.Right
		}
	}

	// x * 1 → x, x / 1 → x
	if rightIsNum && (rightNum.Value == "1" || rightNum.Value == "1.0") {
		if bin.Op == "*" || bin.Op == "/" {
			return bin.Left
		}
	}

	// 1 * x → x
	if leftIsNum && (leftNum.Value == "1" || leftNum.Value == "1.0") {
		if bin.Op == "*" {
			return bin.Right
		}
	}

	// x * 0 → 0 (только если x — без побочных эффектов)
	if rightIsNum && (rightNum.Value == "0" || rightNum.Value == "0.0") {
		if bin.Op == "*" && o.hasNoSideEffects(bin.Left) {
			return &front.Number{Value: "0"}
		}
	}

	// 0 * x → 0
	if leftIsNum && (leftNum.Value == "0" || leftNum.Value == "0.0") {
		if bin.Op == "*" && o.hasNoSideEffects(bin.Right) {
			return &front.Number{Value: "0"}
		}
	}

	return nil
}

func (o *Optimizer) optimizeUnary(unary *front.UnaryExpr) front.Node {
	if unary.Expr != nil {
		unary.Expr = o.optimizeNode(unary.Expr)
	}

	// $ на константе → строка
	if unary.Op == "$" {
		if num, ok := unary.Expr.(*front.Number); ok {
			o.Changed = true
			return &front.String{Value: num.Value}
		}
		if str, ok := unary.Expr.(*front.String); ok {
			o.Changed = true
			return &front.String{Value: str.Value}
		}
	}

	return unary
}

func (o *Optimizer) optimizeCall(call *front.CallExpr) front.Node {
	for i, arg := range call.Args {
		call.Args[i] = o.optimizeNode(arg)
	}
	return call
}

// ============ Хелперы ============

func (o *Optimizer) getBoolConstant(node front.Node) (bool, bool) {
	if ident, ok := node.(*front.Ident); ok {
		if ident.Name == "true" {
			return true, true
		}
		if ident.Name == "false" {
			return false, true
		}
	}
	return false, false
}

func (o *Optimizer) isConstant(node front.Node) bool {
	switch node.(type) {
	case *front.Number:
		return true
	case *front.String:
		return true
	case *front.Ident:
		if ident, ok := node.(*front.Ident); ok {
			return ident.Name == "true" || ident.Name == "false"
		}
		return false
	}
	return false
}

func (o *Optimizer) getConstantValue(node front.Node) string {
	switch n := node.(type) {
	case *front.Number:
		return n.Value
	case *front.String:
		return n.Value
	case *front.Ident:
		if n.Name == "true" {
			return "true"
		}
		if n.Name == "false" {
			return "false"
		}
	}
	return ""
}

func (o *Optimizer) hasNoSideEffects(node front.Node) bool {
	switch n := node.(type) {
	case *front.Number, *front.String, *front.Ident:
		return true
	case *front.BinaryExpr:
		return o.hasNoSideEffects(n.Left) && o.hasNoSideEffects(n.Right)
	case *front.UnaryExpr:
		return o.hasNoSideEffects(n.Expr)
	case *front.TernaryExpr:
		return o.hasNoSideEffects(n.Condition) &&
			o.hasNoSideEffects(n.Then) &&
			o.hasNoSideEffects(n.Else)
	default:
		return false
	}
}

// removeUnusedVariables удаляет объявления и присваивания неиспользуемых переменных
func (o *Optimizer) removeUnusedVariables(fn *front.Function) {
	if fn.Body == nil {
		return
	}

	// Собираем все используемые идентификаторы (кроме результатов var decl)
	used := make(map[string]bool)
	o.collectUsedIdents(fn.Body, used)

	// Удаляем неиспользуемые VarDecl
	fn.Body = o.filterUnusedDecls(fn.Body, used)
}

// collectUsedIdents собирает все идентификаторы, которые читаются (не результат присваивания)
func (o *Optimizer) collectUsedIdents(node front.Node, used map[string]bool) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *front.Block:
		for _, stmt := range n.Statements {
			o.collectUsedIdents(stmt, used)
		}
	case *front.VarDecl:
		// Идентификатор в Expr — используется
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.TernaryExpr:
		o.collectUsedIdents(n.Condition, used)
		o.collectUsedIdents(n.Then, used)
		o.collectUsedIdents(n.Else, used)
	case *front.Assign:
		// Идентификатор в Expr — используется
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.Ident:
		used[n.Name] = true
	case *front.BinaryExpr:
		o.collectUsedIdents(n.Left, used)
		o.collectUsedIdents(n.Right, used)
	case *front.UnaryExpr:
		o.collectUsedIdents(n.Expr, used)
	case *front.CallExpr:
		for _, arg := range n.Args {
			o.collectUsedIdents(arg, used)
		}
	case *front.ReturnStmt:
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.IfStmt:
		o.collectUsedIdents(n.Condition, used)
		o.collectUsedIdents(n.Then, used)
		for _, elsif := range n.Elsifs {
			o.collectUsedIdents(elsif.Condition, used)
			o.collectUsedIdents(elsif.Then, used)
		}
		o.collectUsedIdents(n.Else, used)
	case *front.WhileStmt:
		o.collectUsedIdents(n.Condition, used)
		o.collectUsedIdents(n.Body, used)
	case *front.ForStmt:
		o.collectUsedIdents(n.Init, used)
		o.collectUsedIdents(n.Cond, used)
		o.collectUsedIdents(n.Post, used)
		o.collectUsedIdents(n.Body, used)
	case *front.CaseStmt:
		o.collectUsedIdents(n.Value, used)
		for _, branch := range n.Branches {
			o.collectUsedIdents(branch.Pattern, used)
			o.collectUsedIdents(branch.Body, used)
		}
		o.collectUsedIdents(n.Default, used)
	case *front.TypeOf:
		o.collectUsedIdents(n.Expr, used)
	case *front.ArrayLiteral:
		for _, elem := range n.Elements {
			o.collectUsedIdents(elem, used)
		}
	case *front.ArrayIndex:
		used[n.Name] = true
		o.collectUsedIdents(n.Index, used)
	case *front.ArrayLength:
		used[n.Name] = true
	case *front.ArrayAdd:
		used[n.Name] = true
		o.collectUsedIdents(n.Elem, used)
	}
}

// filterUnusedDecls удаляет VarDecl и Assign, чей результат не используется
func (o *Optimizer) filterUnusedDecls(block *front.Block, used map[string]bool) *front.Block {
	if block == nil {
		return nil
	}

	newStatements := []front.Node{}

	for _, stmt := range block.Statements {
		switch n := stmt.(type) {
		case *front.VarDecl:
			// Проверяем, используется ли переменная
			if !used[n.Name] {
				// Проверяем, есть ли побочные эффекты в Expr
				if n.Expr == nil || o.hasNoSideEffects(n.Expr) {
					o.Changed = true
					continue // пропускаем
				}
				// Если Expr имеет побочные эффекты — оставляем только Expr как statement
				newStatements = append(newStatements, n.Expr)
				o.Changed = true
				continue
			}
			// Оптимизируем внутри
			if n.Expr != nil {
				n.Expr = o.optimizeNode(n.Expr)
			}
			newStatements = append(newStatements, n)
		case *front.Assign:
			if !used[n.Name] {
				if n.Expr == nil || o.hasNoSideEffects(n.Expr) {
					o.Changed = true
					continue
				}
				newStatements = append(newStatements, n.Expr)
				o.Changed = true
				continue
			}
			newStatements = append(newStatements, n)
		default:
			// Рекурсивно фильтруем вложенные блоки
			o.filterNestedBlocks(stmt, used)
			newStatements = append(newStatements, stmt)
		}
	}

	block.Statements = newStatements
	return block
}

func (o *Optimizer) filterNestedBlocks(node front.Node, used map[string]bool) {
	switch n := node.(type) {
	case *front.IfStmt:
		if n.Then != nil {
			n.Then = o.filterUnusedDecls(n.Then, used)
		}
		for i := range n.Elsifs {
			if n.Elsifs[i].Then != nil {
				n.Elsifs[i].Then = o.filterUnusedDecls(n.Elsifs[i].Then, used)
			}
		}
		if n.Else != nil {
			n.Else = o.filterUnusedDecls(n.Else, used)
		}
	case *front.WhileStmt:
		if n.Body != nil {
			n.Body = o.filterUnusedDecls(n.Body, used)
		}
	case *front.ForStmt:
		if n.Body != nil {
			n.Body = o.filterUnusedDecls(n.Body, used)
		}
	case *front.Block:
		o.filterUnusedDecls(n, used)
	}
}

func (o *Optimizer) optimizeTernary(t *front.TernaryExpr) front.Node {
	if t.Condition != nil {
		t.Condition = o.optimizeNode(t.Condition)
	}
	if t.Then != nil {
		t.Then = o.optimizeNode(t.Then)
	}
	if t.Else != nil {
		t.Else = o.optimizeNode(t.Else)
	}

	// Если условие — константа, выбираем одну ветку
	if val, ok := o.getBoolConstant(t.Condition); ok {
		o.Changed = true
		if val {
			return t.Then
		}
		return t.Else
	}

	return t
}
