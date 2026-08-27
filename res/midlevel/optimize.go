package midlevel

import (
	"skrp/res/front"
)

// Optimizer оптимизирует IR
type Optimizer struct{}

// NewOptimizer создает новый оптимизатор
func NewOptimizer() *Optimizer {
	return &Optimizer{}
}

// Optimize оптимизирует AST
func (o *Optimizer) Optimize(ast *front.ASTNode) *front.ASTNode {
	// Простая оптимизация: удаляем мертвый код
	return o.removeDeadCode(ast)
}

func (o *Optimizer) removeDeadCode(node *front.ASTNode) *front.ASTNode {
	if node == nil {
		return nil
	}

	// Рекурсивно обрабатываем детей
	for i := 0; i < len(node.Children); i++ {
		child := node.Children[i]
		if child.Type == "Return" {
			// Удаляем все узлы после return
			node.Children = node.Children[:i+1]
			break
		}
		o.removeDeadCode(child)
	}

	return node
}
