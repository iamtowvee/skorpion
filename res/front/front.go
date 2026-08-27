package front

// InitLexer инициализирует лексер
func InitLexer(code string) ([]Token, error) {
	lexer := NewLexer(code)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// InitParser инициализирует парсер
func InitParser(tokens []Token) (*ASTNode, error) {
	parser := NewParser(tokens)
	ast, err := parser.Parse()
	if err != nil {
		return nil, err
	}
	return ast, nil
}

// ParseConfig парсит конфигурационный файл
func ParseConfig(path string) (*ConfigData, error) {
	parser := NewConfigParser()
	return parser.Parse(path)
}
