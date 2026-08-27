package front

import (
	"fmt"
	"skrp/res/errors"
)

// ASTNode структура узла AST
type ASTNode struct {
	Type     string
	Value    interface{}
	Children []*ASTNode
	Line     int
	Column   int
}

// Parser структура парсера
type Parser struct {
	tokens  []Token
	pos     int
	current Token
}

// NewParser создает новый парсер
func NewParser(tokens []Token) *Parser {
	p := &Parser{
		tokens: tokens,
		pos:    0,
	}
	p.advance()
	return p
}

func (p *Parser) advance() {
	if p.pos >= len(p.tokens) {
		p.current = Token{TOKEN_EOF, "EOF", 0, 0}
	} else {
		p.current = p.tokens[p.pos]
		p.pos++
	}
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{TOKEN_EOF, "EOF", 0, 0}
	}
	return p.tokens[p.pos]
}

func (p *Parser) expect(tokenType TokenType) error {
	if p.current.Type != tokenType {
		errors.NewErrorWithPosition(
			errors.ERR_UNEXPECTED_TOKEN,
			fmt.Sprintf("Ожидается %s, получено %s", tokenType, p.current.Type),
			p.current.Line, p.current.Column, "",
		)
		return fmt.Errorf("unexpected token")
	}
	p.advance()
	return nil
}

// Parse парсит входной поток токенов
func (p *Parser) Parse() (*ASTNode, error) {
	root := &ASTNode{
		Type:     "Program",
		Children: make([]*ASTNode, 0),
	}

	// Парсим программу
	for p.current.Type != TOKEN_EOF {
		switch p.current.Type {
		case TOKEN_USE:
			node, err := p.parseUse()
			if err != nil {
				return nil, err
			}
			root.Children = append(root.Children, node)
		case TOKEN_INCLUDE_C:
			node, err := p.parseIncludeC()
			if err != nil {
				return nil, err
			}
			root.Children = append(root.Children, node)
		case TOKEN_TYPE_VOID, TOKEN_TYPE_INT, TOKEN_TYPE_STRING,
			TOKEN_TYPE_FLOAT, TOKEN_TYPE_DOUBLE, TOKEN_TYPE_BOOL,
			TOKEN_TYPE_ARR, TOKEN_TYPE_DICT, TOKEN_TYPE_ANY,
			TOKEN_TYPE_T, TOKEN_TYPE_CHAR:
			node, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			root.Children = append(root.Children, node)
		default:
			// Если это идентификатор типа, возможно объявление переменной
			if p.current.Type == TOKEN_IDENT {
				// Проверяем, что это тип
				if p.isType(p.current.Literal) {
					node, err := p.parseVariableDeclaration()
					if err != nil {
						return nil, err
					}
					root.Children = append(root.Children, node)
				} else {
					errors.NewErrorWithPosition(
						errors.ERR_UNEXPECTED_TOKEN,
						fmt.Sprintf("Неожиданный токен: %s", p.current.Literal),
						p.current.Line, p.current.Column, "",
					)
					return nil, fmt.Errorf("unexpected token")
				}
			} else {
				errors.NewErrorWithPosition(
					errors.ERR_UNEXPECTED_TOKEN,
					fmt.Sprintf("Неожиданный токен: %s", p.current.Literal),
					p.current.Line, p.current.Column, "",
				)
				return nil, fmt.Errorf("unexpected token")
			}
		}
	}

	return root, nil
}

func (p *Parser) isType(literal string) bool {
	types := map[string]bool{
		"int": true, "char": true, "string": true,
		"arr": true, "dict": true, "float": true,
		"double": true, "bool": true, "void": true,
		"any": true, "T": true,
	}
	return types[literal]
}

// parseUse парсит use модуль
func (p *Parser) parseUse() (*ASTNode, error) {
	node := &ASTNode{
		Type:   "Use",
		Line:   p.current.Line,
		Column: p.current.Column,
	}

	if err := p.expect(TOKEN_USE); err != nil {
		return nil, err
	}

	// Путь модуля или модуль с алиасом
	if p.current.Type == TOKEN_IDENT || p.current.Type == TOKEN_TYPE_ANY {
		path := p.current.Literal
		p.advance()

		// Может быть путь с / (например std/io)
		for p.current.Type == TOKEN_DIV {
			path += "/"
			p.advance()
			if p.current.Type == TOKEN_IDENT {
				path += p.current.Literal
				p.advance()
			}
		}

		node.Value = map[string]interface{}{
			"path": path,
		}

		// Проверяем алиас
		if p.current.Type == TOKEN_AMP {
			p.advance()
			if p.current.Type == TOKEN_IDENT {
				node.Value.(map[string]interface{})["alias"] = p.current.Literal
				p.advance()
			}
		}

		return node, nil
	}

	return nil, fmt.Errorf("invalid use syntax")
}

// parseIncludeC парсит includeC { ... }
func (p *Parser) parseIncludeC() (*ASTNode, error) {
	node := &ASTNode{
		Type:   "IncludeC",
		Line:   p.current.Line,
		Column: p.current.Column,
	}

	if err := p.expect(TOKEN_INCLUDE_C); err != nil {
		return nil, err
	}

	if err := p.expect(TOKEN_LBRACE); err != nil {
		return nil, err
	}

	// Собираем C код до закрывающей скобки
	cCode := ""
	depth := 1

	for depth > 0 && p.current.Type != TOKEN_EOF {
		if p.current.Type == TOKEN_LBRACE {
			depth++
		} else if p.current.Type == TOKEN_RBRACE {
			depth--
			if depth == 0 {
				p.advance()
				break
			}
		}

		// Добавляем токен как есть
		if p.current.Type != TOKEN_EOF {
			cCode += p.current.Literal + " "
		}
		p.advance()
	}

	node.Value = cCode
	return node, nil
}

// parseFunction парсит функцию
func (p *Parser) parseFunction() (*ASTNode, error) {
	node := &ASTNode{
		Type:     "Function",
		Line:     p.current.Line,
		Column:   p.current.Column,
		Children: make([]*ASTNode, 0),
	}

	// Тип возврата
	returnType := p.current.Literal
	p.advance()

	// Имя функции
	if p.current.Type != TOKEN_IDENT {
		return nil, fmt.Errorf("expected function name")
	}
	name := p.current.Literal
	p.advance()

	// Параметры
	if err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}

	params := make([]map[string]string, 0)
	for p.current.Type != TOKEN_RPAREN && p.current.Type != TOKEN_EOF {
		if p.current.Type == TOKEN_TYPE_VOID || p.current.Type == TOKEN_TYPE_INT ||
			p.current.Type == TOKEN_TYPE_STRING || p.current.Type == TOKEN_TYPE_ARR ||
			p.current.Type == TOKEN_TYPE_DICT || p.current.Type == TOKEN_TYPE_FLOAT ||
			p.current.Type == TOKEN_TYPE_DOUBLE || p.current.Type == TOKEN_TYPE_BOOL ||
			p.current.Type == TOKEN_TYPE_ANY || p.current.Type == TOKEN_TYPE_CHAR ||
			p.current.Type == TOKEN_TYPE_T {

			paramType := p.current.Literal
			p.advance()

			if p.current.Type != TOKEN_IDENT {
				return nil, fmt.Errorf("expected parameter name")
			}
			paramName := p.current.Literal
			p.advance()

			params = append(params, map[string]string{
				"type": paramType,
				"name": paramName,
			})

			if p.current.Type == TOKEN_COMMA {
				p.advance()
			}
		} else {
			break
		}
	}

	if err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	// Тело функции
	if err := p.expect(TOKEN_LBRACE); err != nil {
		return nil, err
	}

	body := &ASTNode{
		Type:     "Block",
		Children: make([]*ASTNode, 0),
	}

	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			body.Children = append(body.Children, stmt)
		}
	}

	if err := p.expect(TOKEN_RBRACE); err != nil {
		return nil, err
	}

	node.Value = map[string]interface{}{
		"name":       name,
		"returnType": returnType,
		"params":     params,
	}
	node.Children = append(node.Children, body)

	return node, nil
}

// parseStatement парсит выражение
func (p *Parser) parseStatement() (*ASTNode, error) {
	switch p.current.Type {
	case TOKEN_RETURN:
		return p.parseReturn()
	case TOKEN_TYPE_INT, TOKEN_TYPE_STRING, TOKEN_TYPE_ANY,
		TOKEN_TYPE_FLOAT, TOKEN_TYPE_DOUBLE, TOKEN_TYPE_BOOL,
		TOKEN_TYPE_ARR, TOKEN_TYPE_DICT, TOKEN_TYPE_CHAR:
		return p.parseVariableDeclaration()
	case TOKEN_IF:
		return p.parseIf()
	case TOKEN_WHILE:
		return p.parseWhile()
	case TOKEN_IDENT:
		return p.parseIdentifierStatement()
	default:
		// Может быть выражение
		return p.parseExpression()
	}
}

func (p *Parser) parseReturn() (*ASTNode, error) {
	node := &ASTNode{
		Type:   "Return",
		Line:   p.current.Line,
		Column: p.current.Column,
	}

	if err := p.expect(TOKEN_RETURN); err != nil {
		return nil, err
	}

	// Может быть возвращаемое значение
	if p.current.Type != TOKEN_SEMICOLON && p.current.Type != TOKEN_RBRACE {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, expr)
	}

	// ; опциональна
	if p.current.Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return node, nil
}

func (p *Parser) parseVariableDeclaration() (*ASTNode, error) {
	node := &ASTNode{
		Type:   "VariableDeclaration",
		Line:   p.current.Line,
		Column: p.current.Column,
	}

	varType := p.current.Literal
	p.advance()

	if p.current.Type != TOKEN_IDENT {
		return nil, fmt.Errorf("expected variable name")
	}
	varName := p.current.Literal
	p.advance()

	node.Value = map[string]interface{}{
		"type": varType,
		"name": varName,
	}

	// Проверяем инициализацию
	if p.current.Type == TOKEN_ASSIGN {
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, expr)
	}

	// ; опциональна
	if p.current.Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return node, nil
}

func (p *Parser) parseIf() (*ASTNode, error) {
	node := &ASTNode{
		Type:     "If",
		Line:     p.current.Line,
		Column:   p.current.Column,
		Children: make([]*ASTNode, 0),
	}

	if err := p.expect(TOKEN_IF); err != nil {
		return nil, err
	}

	if err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}

	cond, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	node.Children = append(node.Children, cond)

	if err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	thenBlock, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Children = append(node.Children, thenBlock)

	// Проверяем else
	if p.current.Type == TOKEN_ELSE {
		p.advance()
		elseBlock, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		elseNode := &ASTNode{
			Type:     "Else",
			Children: []*ASTNode{elseBlock},
		}
		node.Children = append(node.Children, elseNode)
	}

	return node, nil
}

func (p *Parser) parseWhile() (*ASTNode, error) {
	node := &ASTNode{
		Type:     "While",
		Line:     p.current.Line,
		Column:   p.current.Column,
		Children: make([]*ASTNode, 0),
	}

	if err := p.expect(TOKEN_WHILE); err != nil {
		return nil, err
	}

	if err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}

	cond, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	node.Children = append(node.Children, cond)

	if err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Children = append(node.Children, block)

	return node, nil
}

func (p *Parser) parseBlock() (*ASTNode, error) {
	if err := p.expect(TOKEN_LBRACE); err != nil {
		return nil, err
	}

	block := &ASTNode{
		Type:     "Block",
		Children: make([]*ASTNode, 0),
	}

	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			block.Children = append(block.Children, stmt)
		}
	}

	if err := p.expect(TOKEN_RBRACE); err != nil {
		return nil, err
	}

	return block, nil
}

func (p *Parser) parseIdentifierStatement() (*ASTNode, error) {
	// Может быть присваивание или вызов функции
	ident := p.current.Literal
	p.advance()

	if p.current.Type == TOKEN_ASSIGN {
		// Присваивание
		node := &ASTNode{
			Type:   "Assignment",
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		node.Value = map[string]interface{}{
			"name": ident,
		}
		p.advance()

		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, expr)

		if p.current.Type == TOKEN_SEMICOLON {
			p.advance()
		}
		return node, nil
	} else if p.current.Type == TOKEN_LPAREN {
		// Вызов функции
		node := &ASTNode{
			Type:   "Call",
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		node.Value = map[string]interface{}{
			"name": ident,
		}

		if err := p.expect(TOKEN_LPAREN); err != nil {
			return nil, err
		}

		args := make([]*ASTNode, 0)
		for p.current.Type != TOKEN_RPAREN && p.current.Type != TOKEN_EOF {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)

			if p.current.Type == TOKEN_COMMA {
				p.advance()
			}
		}

		if err := p.expect(TOKEN_RPAREN); err != nil {
			return nil, err
		}

		for _, arg := range args {
			node.Children = append(node.Children, arg)
		}

		if p.current.Type == TOKEN_SEMICOLON {
			p.advance()
		}
		return node, nil
	}

	return nil, fmt.Errorf("unexpected identifier")
}

func (p *Parser) parseExpression() (*ASTNode, error) {
	return p.parseBinaryExpression(0)
}

func (p *Parser) parseBinaryExpression(minPrec int) (*ASTNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for {
		prec := p.getPrecedence()
		if prec < minPrec {
			break
		}

		op := p.current
		p.advance()

		right, err := p.parseBinaryExpression(prec + 1)
		if err != nil {
			return nil, err
		}

		node := &ASTNode{
			Type:   "BinaryOp",
			Line:   op.Line,
			Column: op.Column,
		}
		node.Value = map[string]interface{}{
			"operator": op.Literal,
		}
		node.Children = []*ASTNode{left, right}
		left = node
	}

	return left, nil
}

func (p *Parser) parsePrimary() (*ASTNode, error) {
	switch p.current.Type {
	case TOKEN_NUMBER:
		node := &ASTNode{
			Type:   "NumberLiteral",
			Value:  p.current.Literal,
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		p.advance()
		return node, nil
	case TOKEN_STRING:
		node := &ASTNode{
			Type:   "StringLiteral",
			Value:  p.current.Literal,
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		p.advance()
		return node, nil
	case TOKEN_CHAR:
		node := &ASTNode{
			Type:   "CharLiteral",
			Value:  p.current.Literal,
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		p.advance()
		return node, nil
	case TOKEN_TRUE, TOKEN_FALSE:
		node := &ASTNode{
			Type:   "BoolLiteral",
			Value:  p.current.Literal == "true",
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		p.advance()
		return node, nil
	case TOKEN_IDENT:
		node := &ASTNode{
			Type:   "Identifier",
			Value:  p.current.Literal,
			Line:   p.current.Line,
			Column: p.current.Column,
		}
		p.advance()
		return node, nil
	case TOKEN_LPAREN:
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expect(TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return expr, nil
	default:
		return nil, fmt.Errorf("unexpected token in expression: %s", p.current.Literal)
	}
}

func (p *Parser) getPrecedence() int {
	precedence := map[TokenType]int{
		TOKEN_OR:    1,
		TOKEN_AND:   2,
		TOKEN_EQ:    3,
		TOKEN_NEQ:   3,
		TOKEN_LT:    4,
		TOKEN_GT:    4,
		TOKEN_LE:    4,
		TOKEN_GE:    4,
		TOKEN_PLUS:  5,
		TOKEN_MINUS: 5,
		TOKEN_MUL:   6,
		TOKEN_DIV:   6,
		TOKEN_MOD:   6,
	}
	if prec, ok := precedence[p.current.Type]; ok {
		return prec
	}
	return 0
}
