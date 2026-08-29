package midlevel

import (
	"skrp/res/errors"
	"skrp/res/front"
)

type MidLevel struct {
	Program   *front.Program
	Semantic  *SemanticAnalyzer
	Optimizer *Optimizer
}

func NewMidLevel(prog *front.Program) *MidLevel {
	return &MidLevel{
		Program:   prog,
		Semantic:  NewSemanticAnalyzer(prog),
		Optimizer: NewOptimizer(prog),
	}
}

func (m *MidLevel) SemCheck() bool {
	// Запускаем семантический анализ
	if !m.Semantic.Analyze() {
		// Копируем ошибки в глобальную систему
		for _, err := range m.Semantic.Errors {
			errors.NewError(err.Code, err.Message, err.Line, err.Column, err.File)
		}
		return false
	}
	return true
}

func (m *MidLevel) OptimizeIR() *front.Program {
	// Оптимизируем программу
	m.Program = m.Optimizer.Optimize()
	return m.Program
}

func (m *MidLevel) GetProgram() *front.Program {
	return m.Program
}
