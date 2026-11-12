package midlevel

import "skrp/res/front"

// MidLevel — тонкая обёртка над Optimizer'ом.
//
// Удалено:
//   - SemCheck()   — не вызывается нигде; semantic запускается напрямую
//                    из skorpion.go::buildProject.
//   - поле Semantic — дублировало Program; Optimizer его не использует.
//   - GetProgram()  — не вызывается.
type MidLevel struct {
	Program   *front.Program
	Optimizer *Optimizer
}

func NewMidLevel(prog *front.Program) *MidLevel {
	return &MidLevel{
		Program:   prog,
		Optimizer: NewOptimizer(prog),
	}
}

func (m *MidLevel) OptimizeIR() *front.Program {
	m.Program = m.Optimizer.Optimize()
	return m.Program
}
