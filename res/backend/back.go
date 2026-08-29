package backend

import (
	"skrp/res/front"
)

type Backend struct {
	Program *front.Program
	IR      *IRProgram
	Builder *Builder
}

func NewBackend(prog *front.Program) *Backend {
	return &Backend{
		Program: prog,
	}
}

func (b *Backend) Pipe(ir *IRProgram) *IRProgram {
	b.IR = ir
	return b.IR
}

func (b *Backend) GenCFromIR(ir *IRProgram) string {
	gen := NewCodeGenerator(ir)
	return gen.Generate()
}

func (b *Backend) Build(ir *IRProgram, config *BuildConfig) bool {
	builder := NewBuilder(config)
	return builder.Build(ir)
}
