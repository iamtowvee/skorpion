package front

import (
	"fmt"
	"skrp/res/errors"
)

type Parser struct {
	lexer     *Lexer
	current   Token
	peek      Token
	hasErrors bool
}

// Конструктор без advance()
func NewParserInternal(input string) *Parser {
	p := &Parser{
		lexer:     NewLexer(input),
		hasErrors: false,
	}
	// Не вызываем advance() здесь!
	return p
}

func (p *Parser) advance() {
	p.current = p.peek
	p.peek = p.lexer.NextToken()
}

func (p *Parser) match(tt TokenType) bool {
	if p.peek.Type == tt {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(tt TokenType) Token {
	if p.peek.Type == tt {
		p.advance()
		return p.current
	}
	p.hasErrors = true
	errors.NewFatalError("0004",
		fmt.Sprintf("Expected '%s', got '%s' (at %d:%d)", tt.String(), p.peek.Literal, p.peek.Line, p.peek.Column),
		p.peek.Line, p.peek.Column, "")
	return p.current
}

func (p *Parser) isType(token Token) bool {
	result := false
	switch token.Literal {
	case "void", "int", "string", "float", "double", "bool", "char", "arr", "dict", "any":
		result = true
	}
	fmt.Printf("[DEBUG] isType(%s) = %v\n", token.Literal, result)
	return result
}

func (p *Parser) Parse() *Program {
	// Инициализируем первый токен
	p.advance()

	// Проверяем, были ли ошибки в лексере
	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}}
	}

	prog := &Program{Imports: []*Import{}, Functions: []*Function{}}

	fmt.Println("[DEBUG] Starting parse, first token:", p.peek.Literal, "type:", p.peek.Type)

	// Сначала импорты
	for p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "use" {
		if p.hasErrors || errors.HasFatal() {
			break
		}
		imp := p.parseImport()
		if imp != nil {
			prog.Imports = append(prog.Imports, imp)
		}
		if errors.HasFatal() {
			return prog
		}
	}

	fmt.Println("[DEBUG] After imports, current token:", p.peek.Literal, "type:", p.peek.Type)

	// Затем функции
	funcCount := 0
	for p.peek.Type != TOKEN_EOF {
		fmt.Printf("[DEBUG] Loop iteration %d: token='%s', type=%d\n", funcCount, p.peek.Literal, p.peek.Type)

		if p.hasErrors || errors.HasFatal() {
			break
		}

		// Проверяем, что это функция (тип возврата)
		if p.isType(p.peek) {
			fmt.Printf("[DEBUG] Found type: '%s', parsing function...\n", p.peek.Literal)
			fn := p.parseFunction()
			if fn != nil {
				prog.Functions = append(prog.Functions, fn)
				funcCount++
				fmt.Printf("[DEBUG] Function parsed: %s\n", fn.Name)
			}
		} else {
			// Если не функция, пропускаем
			fmt.Printf("[DEBUG] Skipping token: '%s'\n", p.peek.Literal)
			p.advance()
		}
	}

	fmt.Printf("[DEBUG] Total functions parsed: %d\n", len(prog.Functions))

	return prog
}

func (p *Parser) parseImport() *Import {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // use
	imp := &Import{}

	if p.peek.Type == TOKEN_HASH {
		p.advance()
		imp.All = true
	}

	// Путь: path/to/module
	path := ""
	for p.peek.Type == TOKEN_IDENT || p.peek.Literal == "/" {
		if p.peek.Type == TOKEN_IDENT {
			path += p.peek.Literal
		} else if p.peek.Literal == "/" {
			path += "/"
		}
		p.advance()
	}
	imp.Path = path

	if p.peek.Type == TOKEN_AMPERSAND {
		p.advance()
		if p.peek.Type == TOKEN_IDENT {
			imp.Alias = p.peek.Literal
			p.advance()
		}
	}

	return imp
}

func (p *Parser) parseFunction() *Function {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	// Тип возврата
	retType := p.peek.Literal
	p.advance()

	isExport := true
	if p.peek.Type == TOKEN_STAR {
		isExport = false
		p.advance()
	}

	// Имя
	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0006",
			fmt.Sprintf("Expected function name, got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		return nil
	}
	name := p.peek.Literal
	p.advance()

	// Открывающая скобка
	if p.peek.Type != TOKEN_LPAREN {
		p.hasErrors = true
		errors.NewFatalError("0014",
			fmt.Sprintf("Expected '(', got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		return nil
	}
	p.advance()

	// Параметры
	params := []*Param{}
	if p.peek.Type != TOKEN_RPAREN {
		for {
			// Тип параметра
			if !p.isType(p.peek) {
				p.hasErrors = true
				errors.NewFatalError("0008",
					fmt.Sprintf("Expected parameter type, got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
					p.peek.Line, p.peek.Column, "")
				return nil
			}
			paramType := p.peek.Literal
			p.advance()

			// Имя параметра
			if p.peek.Type != TOKEN_IDENT {
				p.hasErrors = true
				errors.NewFatalError("0009",
					fmt.Sprintf("Expected parameter name, got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
					p.peek.Line, p.peek.Column, "")
				return nil
			}
			paramName := p.peek.Literal
			p.advance()

			// Проверяем значение по умолчанию
			var defaultValue Node
			if p.peek.Type == TOKEN_EQUALS {
				p.advance() // пропускаем =
				defaultValue = p.parseExpression()
				if defaultValue == nil {
					return nil
				}
			}

			params = append(params, &Param{
				Name:         paramName,
				Type:         paramType,
				DefaultValue: defaultValue,
			})

			if p.peek.Type == TOKEN_COMMA {
				p.advance()
				continue
			} else {
				break
			}
		}
	}

	// Закрывающая скобка
	if p.peek.Type != TOKEN_RPAREN {
		p.hasErrors = true
		errors.NewFatalError("0015",
			fmt.Sprintf("Expected ')', got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		return nil
	}
	p.advance()

	// Тело функции
	if p.peek.Type != TOKEN_LBRACE {
		p.hasErrors = true
		errors.NewFatalError("0016",
			fmt.Sprintf("Expected '{', got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		return nil
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &Function{
		Name:       name,
		ReturnType: retType,
		Params:     params,
		Body:       body,
		IsExport:   isExport,
	}
}

func (p *Parser) parseBlock() *Block {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.expect(TOKEN_LBRACE)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	block := &Block{Statements: []Node{}}

	for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
		if p.hasErrors || errors.HasFatal() {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
	}

	p.expect(TOKEN_RBRACE)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	return block
}

func (p *Parser) parseStatement() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	switch p.peek.Type {
	case TOKEN_KEYWORD:
		switch p.peek.Literal {
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile()
		case "for":
			return p.parseFor()
		case "return":
			return p.parseReturn()
		case "int", "string", "float", "double", "bool", "char", "arr", "dict", "any":
			return p.parseVarDecl()
		}
	case TOKEN_INCLUDE_C:
		return p.parseIncludeC()
	case TOKEN_IDENT:
		return p.parseAssignmentOrCall()
	}

	// Если не распознали, пропускаем
	p.advance()
	return nil
}

func (p *Parser) parseIncludeC() *IncludeC {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // includeC

	// Ожидаем ```
	if p.peek.Type != TOKEN_BACKTICK {
		p.hasErrors = true
		errors.NewFatalError("0017",
			fmt.Sprintf("Expected ``` after includeC, got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		return nil
	}

	// Берем код из токена
	code := p.peek.Literal
	p.advance() // пропускаем TOKEN_BACKTICK

	return &IncludeC{Code: code}
}

func (p *Parser) parseIf() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // if
	cond := p.parseExpression()
	if cond == nil {
		return nil
	}

	then := p.parseBlock()
	if then == nil {
		return nil
	}

	elsifs := []*Elsif{}

	// Парсим все elsif
	for p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "elsif" {
		p.advance() // elsif

		elsifCond := p.parseExpression()
		if elsifCond == nil {
			return nil
		}

		elsifThen := p.parseBlock()
		if elsifThen == nil {
			return nil
		}

		elsifs = append(elsifs, &Elsif{
			Condition: elsifCond,
			Then:      elsifThen,
		})
	}

	var elseBlock *Block
	if p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "else" {
		p.advance()
		elseBlock = p.parseBlock()
		if elseBlock == nil {
			return nil
		}
	}

	return &IfStmt{
		Condition: cond,
		Then:      then,
		Elsifs:    elsifs,
		Else:      elseBlock,
	}
}

func (p *Parser) parseWhile() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // while
	cond := p.parseExpression()
	if cond == nil {
		return nil
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &WhileStmt{Condition: cond, Body: body}
}

func (p *Parser) parseFor() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // for
	p.expect(TOKEN_LPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	var init Node
	if p.peek.Type != TOKEN_SEMICOLON {
		init = p.parseStatement()
		if init == nil {
			return nil
		}
	}
	p.expect(TOKEN_SEMICOLON)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	var cond Node
	if p.peek.Type != TOKEN_SEMICOLON {
		cond = p.parseExpression()
		if cond == nil {
			return nil
		}
	}
	p.expect(TOKEN_SEMICOLON)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	var post Node
	if p.peek.Type != TOKEN_RPAREN {
		post = p.parseStatement()
		if post == nil {
			return nil
		}
	}
	p.expect(TOKEN_RPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &ForStmt{Init: init, Cond: cond, Post: post, Body: body}
}

func (p *Parser) parseReturn() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // return

	var expr Node
	if p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_SEMICOLON {
		expr = p.parseExpression()
		if expr == nil {
			return nil
		}
	}

	if p.peek.Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return &ReturnStmt{Expr: expr}
}

func (p *Parser) parseVarDecl() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	varType := p.peek.Literal
	p.advance()

	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0011",
			fmt.Sprintf("Expected variable name, got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		return nil
	}
	name := p.peek.Literal
	p.advance()

	var expr Node
	if p.peek.Type == TOKEN_EQUALS {
		p.advance()
		expr = p.parseExpression()
		if expr == nil {
			return nil
		}
	}

	if p.peek.Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return &VarDecl{Name: name, Type: varType, Expr: expr}
}

func (p *Parser) parseAssignmentOrCall() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	name := p.peek.Literal
	p.advance()

	// Проверяем, не точка ли это (module.function)
	if p.peek.Type == TOKEN_DOT {
		p.advance()
		if p.peek.Type == TOKEN_IDENT {
			// module.function
			moduleName := name
			funcName := p.peek.Literal
			p.advance()
			fullName := moduleName + "." + funcName

			if p.peek.Type == TOKEN_LPAREN {
				return p.parseCall(fullName)
			}

			// Если после точки не вызов, то это ошибка
			p.hasErrors = true
			errors.NewFatalError("0018",
				fmt.Sprintf("Expected function call after '.', got '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
				p.peek.Line, p.peek.Column, "")
			return nil
		}
	}

	if p.peek.Type == TOKEN_LPAREN {
		return p.parseCall(name)
	}

	var expr Node
	if p.peek.Type == TOKEN_EQUALS {
		p.advance()
		expr = p.parseExpression()
		if expr == nil {
			return nil
		}
	}

	if p.peek.Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return &Assign{Name: name, Expr: expr}
}

func (p *Parser) parseCall(name string) Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.expect(TOKEN_LPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	args := []Node{}
	if p.peek.Type != TOKEN_RPAREN {
		for {
			expr := p.parseExpression()
			if expr == nil {
				return nil
			}
			args = append(args, expr)
			if p.peek.Type == TOKEN_COMMA {
				p.advance()
			} else {
				break
			}
		}
	}
	p.expect(TOKEN_RPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	return &CallExpr{Name: name, Args: args}
}

func (p *Parser) parseExpression() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}
	return p.parseBinary(0)
}

func (p *Parser) parseBinary(prec int) Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	left := p.parsePrimary()
	if left == nil {
		return nil
	}

	for {
		if p.hasErrors || errors.HasFatal() {
			break
		}

		op := p.peek.Literal
		if p.peek.Type != TOKEN_PLUS && p.peek.Type != TOKEN_MINUS &&
			p.peek.Type != TOKEN_STAR && p.peek.Type != TOKEN_SLASH &&
			p.peek.Type != TOKEN_LT && p.peek.Type != TOKEN_GT &&
			p.peek.Type != TOKEN_EQUALS {
			break
		}

		nextPrec := 0
		switch p.peek.Type {
		case TOKEN_PLUS, TOKEN_MINUS:
			nextPrec = 1
		case TOKEN_STAR, TOKEN_SLASH:
			nextPrec = 2
		default:
			nextPrec = 0
		}

		if nextPrec < prec {
			break
		}

		p.advance()
		right := p.parseBinary(nextPrec + 1)
		if right == nil {
			return nil
		}
		left = &BinaryExpr{Left: left, Op: op, Right: right}
	}
	return left
}

func (p *Parser) parsePrimary() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	switch p.peek.Type {
	case TOKEN_NUMBER:
		val := p.peek.Literal
		p.advance()
		return &Number{Value: val}

	case TOKEN_STRING:
		val := p.peek.Literal
		p.advance()
		return &String{Value: val}

	case TOKEN_DOLLAR:
		p.advance()
		expr := p.parsePrimary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Op: "$", Expr: expr}

	case TOKEN_IDENT:
		name := p.peek.Literal
		p.advance()

		// Проверяем, не точка ли это (module.function)
		if p.peek.Type == TOKEN_DOT {
			p.advance()
			if p.peek.Type == TOKEN_IDENT {
				moduleName := name
				funcName := p.peek.Literal
				p.advance()
				fullName := moduleName + "." + funcName

				if p.peek.Type == TOKEN_LPAREN {
					return p.parseCall(fullName)
				}
				return &Ident{Name: fullName}
			}
		}

		if p.peek.Type == TOKEN_LPAREN {
			return p.parseCall(name)
		}
		return &Ident{Name: name}

	case TOKEN_KEYWORD:
		if p.peek.Literal == "true" || p.peek.Literal == "false" {
			val := p.peek.Literal
			p.advance()
			return &Ident{Name: val}
		}
		if p.peek.Literal == "const" {
			p.advance()
			return p.parsePrimary()
		}
		p.hasErrors = true
		errors.NewFatalError("0012",
			fmt.Sprintf("Unexpected keyword in expression: '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		p.advance()
		return nil

	case TOKEN_LPAREN:
		p.advance()
		expr := p.parseExpression()
		if expr == nil {
			return nil
		}
		p.expect(TOKEN_RPAREN)
		if p.hasErrors || errors.HasFatal() {
			return nil
		}
		return expr

	default:
		p.hasErrors = true
		errors.NewFatalError("0013",
			fmt.Sprintf("Unexpected token in expression: '%s' (at %d:%d)", p.peek.Literal, p.peek.Line, p.peek.Column),
			p.peek.Line, p.peek.Column, "")
		p.advance()
		return nil
	}
}
