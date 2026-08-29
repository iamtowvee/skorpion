package midlevel

import (
	"skrp/res/front"
)

type Optimizer struct {
	Program *front.Program
}

func NewOptimizer(prog *front.Program) *Optimizer {
	return &Optimizer{Program: prog}
}

func (o *Optimizer) Optimize() *front.Program {
	// Оптимизация на уровне AST
	for _, fn := range o.Program.Functions {
		if fn.Body != nil {
			o.optimizeBlock(fn.Body)
		}
	}
	return o.Program
}

func (o *Optimizer) optimizeBlock(block *front.Block) {
	// Удаляем недостижимый код (после return)
	newStatements := []front.Node{}
	deadCode := false

	for _, stmt := range block.Statements {
		if deadCode {
			// Пропускаем все последующие операторы
			continue
		}

		if _, ok := stmt.(*front.ReturnStmt); ok {
			deadCode = true
		}

		newStatements = append(newStatements, stmt)
	}

	block.Statements = newStatements

	// Оптимизация внутри каждого оператора
	for _, stmt := range block.Statements {
		o.optimizeNode(stmt)
	}
}

func (o *Optimizer) optimizeNode(node front.Node) {
	switch n := node.(type) {
	case *front.Block:
		o.optimizeBlock(n)
	case *front.IfStmt:
		if n.Then != nil {
			o.optimizeBlock(n.Then)
		}
		if n.Else != nil {
			o.optimizeBlock(n.Else)
		}
	case *front.WhileStmt:
		if n.Body != nil {
			o.optimizeBlock(n.Body)
		}
	case *front.ForStmt:
		if n.Body != nil {
			o.optimizeBlock(n.Body)
		}
	case *front.BinaryExpr:
		// Константная свёртка для арифметических операций
		o.foldConstants(n)
	}
}

func (o *Optimizer) foldConstants(bin *front.BinaryExpr) {

	// Вычисляем результат
	// Пока просто помечаем, что можно свернуть
	// Реализуем вычисления позже, когда будет интерпретатор
}
