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
		return &Program{Imports: []*Import{}, Functions: []*Function{}, ErrorDecls: []*ErrorDecl{}}
	}

	parser := NewParser(input)
	prog := parser.Parse()

	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}, ErrorDecls: []*ErrorDecl{}}
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

// LoadProgramWithConfig — как LoadProgram, но с настройками библиотек.
// Возвращает также ImportManager, чтобы передать его в SemanticAnalyzer.
func LoadProgramWithConfig(
	mainFile, projectDir, libsDir string,
	deps []string,
) (*Program, *ImportManager, error) {
	baseDir := filepath.Dir(mainFile)
	im := NewImportManager(baseDir)
	im.SetLibraryConfig(projectDir, libsDir, deps)

	prog, err := im.LoadMain(mainFile)
	if err != nil {
		return nil, im, err
	}
	return prog, im, nil
}

// Создание AST из нескольких файлов
func BuildProgram(files map[string]string) *Program {
	mainProg := &Program{Imports: []*Import{}, Functions: []*Function{}, ErrorDecls: []*ErrorDecl{}}

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
