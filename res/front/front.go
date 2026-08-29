package front

import (
	"skrp/res/errors"
)

// Фасад для парсера
func NewParser(input string) *Parser {
	p := &Parser{
		lexer:     NewLexer(input),
		hasErrors: false,
	}
	// Не вызываем advance() здесь!
	// Первый токен будет прочитан в Parse()
	return p
}

// Фасад для AST
func InitAST(input string) *Program {
	// Проверяем, были ли ошибки
	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}}
	}

	parser := NewParser(input)
	prog := parser.Parse()

	// Если есть фатальные ошибки, возвращаем пустую программу
	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}}
	}

	return prog
}
