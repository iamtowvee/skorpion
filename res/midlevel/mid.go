package midlevel

import (
	"skrp/res/front"
)

// SemCheck выполняет семантический анализ
func SemCheck(ast *front.ASTNode) error {
	checker := NewSemanticChecker()
	return checker.Check(ast)
}

// OptimizeIR оптимизирует промежуточное представление
func OptimizeIR(ast *front.ASTNode) *front.ASTNode {
	optimizer := NewOptimizer()
	return optimizer.Optimize(ast)
}
