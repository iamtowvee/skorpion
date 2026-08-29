package front

import (
	"path/filepath"
	"skrp/res/errors"
)

// Фасад для парсера
func NewParser(input string) *Parser {
	p := &Parser{
		lexer:     NewLexer(input),
		hasErrors: false,
	}
	return p
}

// Фасад для AST из одного файла
func InitAST(input string) *Program {
	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}}
	}

	parser := NewParser(input)
	prog := parser.Parse()

	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}}
	}

	return prog
}

// Загрузка программы с импортами
func LoadProgram(mainFile string) (*Program, error) {
	baseDir := filepath.Dir(mainFile)
	im := NewImportManager(baseDir)
	return im.LoadMain(mainFile)
}

// Получение всех функций из программы и импортов
func GetAllFunctions(prog *Program, im *ImportManager) []*Function {
	// Только функции из main, не из импортов
	return prog.Functions
}

// Создание AST из нескольких файлов
func BuildProgram(files map[string]string) *Program {
	mainProg := &Program{Imports: []*Import{}, Functions: []*Function{}}

	return mainProg
}
