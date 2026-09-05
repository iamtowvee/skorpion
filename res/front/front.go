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
		FileName:  "<input>",
	}
	return p
}

func NewParserWithFile(input string, fileName string) *Parser {
	p := &Parser{
		lexer:     NewLexer(input),
		hasErrors: false,
		FileName:  fileName,
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
	allFunctions := make([]*Function, len(prog.Functions))
	copy(allFunctions, prog.Functions)

	if im != nil {
		imported := im.GetAllFunctions()
		allFunctions = append(allFunctions, imported...)
	}

	return allFunctions
}

// Создание AST из нескольких файлов
func BuildProgram(files map[string]string) *Program {
	mainProg := &Program{Imports: []*Import{}, Functions: []*Function{}}

	for path, content := range files {
		path += ""
		prog := InitAST(content)
		if prog != nil {
			mainProg.Imports = append(mainProg.Imports, prog.Imports...)
			mainProg.Functions = append(mainProg.Functions, prog.Functions...)
		}
	}

	return mainProg
}
