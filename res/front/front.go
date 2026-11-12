package front

import "path/filepath"

// Фасад для парсера.
//
// Удалены (не вызывались нигде):
//   - InitAST         (не путать с front.NewParser — используется в parser_test)
//   - LoadProgram     (заменён на LoadProgramWithConfig)
//   - GetAllFunctions (заменён на im.GetAllFunctionsInternal)
//   - BuildProgram    (не использовался)
func NewParser(input string) *Parser {
	return &Parser{
		lexer:    NewLexer(input),
		FileName: "<input>",
	}
}

func NewParserWithFile(input string, fileName string) *Parser {
	return &Parser{
		lexer:    NewLexer(input),
		FileName: fileName,
	}
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
