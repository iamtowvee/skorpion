package midlevel

import (
	"strconv"
	"strings"

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

	case *front.ForInStmt:
		return o.optimizeForIn(n)

	case *front.CharLiteral:
		return node

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

func (o *Optimizer) optimizeForIn(forIn *front.ForInStmt) front.Node {
	if forIn.Iterable != nil {
		forIn.Iterable = o.optimizeNode(forIn.Iterable)
	}
	if forIn.Body != nil {
		forIn.Body = o.optimizeBlock(forIn.Body)
	}
	return forIn
}

func (o *Optimizer) optimizeCase(caseStmt *front.CaseStmt) front.Node {
	if caseStmt.Value != nil {
		caseStmt.Value = o.optimizeNode(caseStmt.Value)
	}

	for i, branch := range caseStmt.Branches {
		for j, pat := range branch.Patterns {
			caseStmt.Branches[i].Patterns[j] = o.optimizeNode(pat)
		}
		if branch.Body != nil {
			caseStmt.Branches[i].Body = o.optimizeBlock(branch.Body)
		}
	}

	if caseStmt.Default != nil {
		caseStmt.Default = o.optimizeBlock(caseStmt.Default)
	}

	// Если value — числовая или строковая константа
	if o.isConstant(caseStmt.Value) {
		val := o.getConstantValue(caseStmt.Value)
		if val != "" {
			for _, branch := range caseStmt.Branches {
				// Все паттерны в ветке должны быть константными
				allConstant := true
				matched := false
				for _, pat := range branch.Patterns {
					if !o.isConstant(pat) {
						allConstant = false
						break
					}
					if o.getConstantValue(pat) == val {
						matched = true
					}
				}
				if !allConstant {
					// Первый не-константный паттерн — оставляем как есть
					break
				}
				if matched {
					o.Changed = true
					return branch.Body
				}
			}
			// Ни один константный паттерн не совпал
			if caseStmt.Default != nil {
				o.Changed = true
				return caseStmt.Default
			}
		}
	}

	// Если value — константа, можно выбрать одну ветку
	if _, isNum := caseStmt.Value.(*front.Number); isNum {
		if _, isStr := caseStmt.Value.(*front.String); isStr {
			// fallthrough
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

	leftChar, leftIsChar := bin.Left.(*front.CharLiteral)
	rightChar, rightIsChar := bin.Right.(*front.CharLiteral)

	if leftIsChar && rightIsChar && bin.Op == "+" {
		// char + char → string из 2 символов
		return &front.String{Value: string([]byte{leftChar.Value, rightChar.Value})}
	}

	// Числа
	if leftIsNum && rightIsNum {
		leftVal := numberParseFloat(leftNum.Value)
		rightVal := numberParseFloat(rightNum.Value)

		var result float64
		var isInt bool

		leftInt, leftOK := numberParseInt(leftNum.Value)
		rightInt, rightOK := numberParseInt(rightNum.Value)
		isInt = leftOK && rightOK

		// Был ли float (хотя бы один операнд с суффиксом f)
		isFloat := numberIsFloat(leftNum.Value) || numberIsFloat(rightNum.Value)

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
		if isFloat {
			return &front.Number{Value: strconv.FormatFloat(result, 'f', -1, 64) + "f"}
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
	if rightIsNum && isZeroNumber(rightNum.Value) {
		if bin.Op == "+" || bin.Op == "-" {
			return bin.Left
		}
	}

	// 0 + x → x
	if leftIsNum && isZeroNumber(leftNum.Value) {
		if bin.Op == "+" {
			return bin.Right
		}
	}

	// x * 1 → x, x / 1 → x
	if rightIsNum && isOneNumber(rightNum.Value) {
		if bin.Op == "*" || bin.Op == "/" {
			return bin.Left
		}
	}

	// 1 * x → x
	if leftIsNum && isOneNumber(leftNum.Value) {
		if bin.Op == "*" {
			return bin.Right
		}
	}

	// x * 0 → 0 (только если x — без побочных эффектов)
	if rightIsNum && isZeroNumber(rightNum.Value) {
		if bin.Op == "*" && o.hasNoSideEffects(bin.Left) {
			return &front.Number{Value: "0"}
		}
	}

	// 0 * x → 0
	if leftIsNum && isZeroNumber(leftNum.Value) {
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
	case *front.CharLiteral:
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
		// Для case-паттернов нормализуем: 2.0f → 2.0
		return numberStripSuffix(n.Value)
	case *front.String:
		return n.Value
	case *front.CharLiteral:
		return string([]byte{n.Value})
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
	case *front.Number, *front.String, *front.Ident, *front.CharLiteral:
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
			if stmt != nil {
				o.collectUsedIdents(stmt, used)
			}
		}
	case *front.VarDecl:
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.Assign:
		used[n.Name] = true
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.Ident:
		used[n.Name] = true
	case *front.BinaryExpr:
		if n.Left != nil {
			o.collectUsedIdents(n.Left, used)
		}
		if n.Right != nil {
			o.collectUsedIdents(n.Right, used)
		}
	case *front.UnaryExpr:
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.CallExpr:
		for _, arg := range n.Args {
			if arg != nil {
				o.collectUsedIdents(arg, used)
			}
		}
	case *front.CallRangeExpr:
		if n.Range != nil {
			if n.Range.Start != nil {
				o.collectUsedIdents(n.Range.Start, used)
			}
			if n.Range.End != nil {
				o.collectUsedIdents(n.Range.End, used)
			}
		}
		for _, arg := range n.Extra {
			if arg != nil {
				o.collectUsedIdents(arg, used)
			}
		}
	case *front.ReturnStmt:
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.IfStmt:
		if n.Condition != nil {
			o.collectUsedIdents(n.Condition, used)
		}
		if n.Then != nil {
			o.collectUsedIdents(n.Then, used)
		}
		for _, elsif := range n.Elsifs {
			if elsif.Condition != nil {
				o.collectUsedIdents(elsif.Condition, used)
			}
			if elsif.Then != nil {
				o.collectUsedIdents(elsif.Then, used)
			}
		}
		if n.Else != nil {
			o.collectUsedIdents(n.Else, used)
		}
	case *front.WhileStmt:
		if n.Condition != nil {
			o.collectUsedIdents(n.Condition, used)
		}
		if n.Body != nil {
			o.collectUsedIdents(n.Body, used)
		}
	case *front.ForInStmt:
		if n.Iterable != nil {
			o.collectUsedIdents(n.Iterable, used)
		}
		if n.Body != nil {
			o.collectUsedIdents(n.Body, used)
		}
	case *front.CaseStmt:
		if n.Value != nil {
			o.collectUsedIdents(n.Value, used)
		}
		for _, branch := range n.Branches {
			for _, pat := range branch.Patterns {
				o.collectUsedIdents(pat, used)
			}
			if branch.Body != nil {
				o.collectUsedIdents(branch.Body, used)
			}
		}
		if n.Default != nil {
			o.collectUsedIdents(n.Default, used)
		}
	case *front.TypeOf:
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.ArrayLiteral:
		for _, elem := range n.Elements {
			if elem != nil {
				o.collectUsedIdents(elem, used)
			}
		}
	case *front.ArrayIndex:
		used[n.Name] = true
		if n.Index != nil {
			o.collectUsedIdents(n.Index, used)
		}
	case *front.ArrayLength:
		used[n.Name] = true
	case *front.ArrayAdd:
		used[n.Name] = true
		if n.Elem != nil {
			o.collectUsedIdents(n.Elem, used)
		}
	case *front.TernaryExpr:
		if n.Condition != nil {
			o.collectUsedIdents(n.Condition, used)
		}
		if n.Then != nil {
			o.collectUsedIdents(n.Then, used)
		}
		if n.Else != nil {
			o.collectUsedIdents(n.Else, used)
		}
	case *front.ThrowStmt:
		if n.Expr != nil {
			o.collectUsedIdents(n.Expr, used)
		}
	case *front.ErrorInstance:
		if n.Fields != nil {
			for _, value := range n.Fields {
				if value != nil {
					o.collectUsedIdents(value, used)
				}
			}
		}
	case *front.TryStmt:
		if n.Body != nil {
			o.collectUsedIdents(n.Body, used)
		}
		for _, clause := range n.Catches {
			if clause.VarName != "" {
				used[clause.VarName] = true
			}
			if clause.Body != nil {
				o.collectUsedIdents(clause.Body, used)
			}
		}
	case *front.FieldAccess:
		used[n.Object] = true
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
	case *front.ForInStmt:
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

// numberIsFloat — есть ли суффикс f/F
func numberIsFloat(s string) bool {
	return strings.HasSuffix(s, "f") || strings.HasSuffix(s, "F")
}

// numberStripSuffix — убирает суффикс f/F
func numberStripSuffix(s string) string {
	if numberIsFloat(s) {
		return s[:len(s)-1]
	}
	return s
}

// numberParseFloat — парсит число как float64 (без суффикса)
func numberParseFloat(s string) float64 {
	v := numberStripSuffix(s)
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

// numberParseInt — парсит число как int (ok = true, если получилось)
func numberParseInt(s string) (int, bool) {
	v := numberStripSuffix(s)
	i, err := strconv.Atoi(v)
	return i, err == nil
}

// isZeroNumber — "0", "0.0", "0.0f", "0f"
func isZeroNumber(s string) bool {
	v := numberStripSuffix(s)
	return v == "0" || v == "0.0"
}

// isOneNumber — "1", "1.0", "1.0f", "1f"
func isOneNumber(s string) bool {
	v := numberStripSuffix(s)
	return v == "1" || v == "1.0"
}
