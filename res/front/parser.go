package front

import (
	"fmt"
	"skrp/res/debug"
	"skrp/res/errors"
	"strings"
)

type Parser struct {
	lexer     *Lexer
	current   Token
	peek      Token
	hasErrors bool
	FileName  string
}

// Конструктор без advance()
func NewParserInternal(input string) *Parser {
	p := &Parser{
		lexer:     NewLexer(input),
		hasErrors: false,
		FileName:  "<input>",
	}
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
		fmt.Sprintf("Expected '%s', got '%s'", tt.String(), p.peek.Literal),
		p.peek.Line, p.peek.Column, p.FileName)
	return p.current
}

func (p *Parser) isType(token Token) bool {
	result := false
	switch token.Literal {
	case "void", "int", "string", "float", "double", "bool", "char", "arr", "dict", "any":
		result = true
	}
	debug.Debug("isType(%s) = %v", token.Literal, result)
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

	debug.Debug("Starting parse, first token: %s type: %s", p.peek.Literal, p.peek.Type.String())

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

	debug.Debug("After imports, current token: %s type: %s", p.peek.Literal, p.peek.Type.String())

	// Затем функции
	funcCount := 0
	for p.peek.Type != TOKEN_EOF {
		debug.Debug("Loop iteration %d: token='%s', type=%d", funcCount, p.peek.Literal, p.peek.Type)

		if p.hasErrors || errors.HasFatal() {
			break
		}

		// Проверяем, что это функция (тип возврата)
		if p.isType(p.peek) {
			debug.Debug("Found type: '%s', parsing function...", p.peek.Literal)
			fn := p.parseFunction()
			if fn != nil {
				prog.Functions = append(prog.Functions, fn)
				funcCount++
				debug.Debug("Function parsed: %s", fn.Name)
			}
		} else {
			// Если не функция, пропускаем
			debug.Debug("Skipping token: '%s'", p.peek.Literal)
			p.advance()
		}
	}

	debug.Debug("Total functions parsed: %d", len(prog.Functions))

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
			fmt.Sprintf("Expected function name, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	name := p.peek.Literal
	p.advance()

	// Открывающая скобка
	if p.peek.Type != TOKEN_LPAREN {
		p.hasErrors = true
		errors.NewFatalError("0014",
			fmt.Sprintf("Expected '(', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
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
					fmt.Sprintf("Expected parameter type, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			paramType := p.peek.Literal
			p.advance()

			// Имя параметра
			if p.peek.Type != TOKEN_IDENT {
				p.hasErrors = true
				errors.NewFatalError("0009",
					fmt.Sprintf("Expected parameter name, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
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
			fmt.Sprintf("Expected ')', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance()

	// Тело функции
	if p.peek.Type != TOKEN_LBRACE {
		p.hasErrors = true
		errors.NewFatalError("0016",
			fmt.Sprintf("Expected '{', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
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
		case "case":
			return p.parseCase()
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
			fmt.Sprintf("Expected ``` after includeC, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
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

func (p *Parser) parseCase() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // case
	p.expect(TOKEN_LPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	value := p.parseExpression()
	if value == nil {
		return nil
	}

	p.expect(TOKEN_RPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.expect(TOKEN_LBRACE)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	branches := []*CaseBranch{}
	var defaultBlock *Block

	for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
		if p.hasErrors || errors.HasFatal() {
			break
		}

		debug.Debug("parseCase: parsing pattern, current token: %s (%s)", p.peek.Literal, p.peek.Type.String())

		// Парсим паттерн
		pattern := p.parseExpression()
		debug.Debug("parseCase: pattern parsed: %T", pattern)
		if pattern == nil {
			return nil
		}

		// Проверяем, не _ ли это (дефолт)
		if ident, ok := pattern.(*Ident); ok && ident.Name == "_" {
			defaultBlock = p.parseBlock()
			if defaultBlock == nil {
				return nil
			}
			debug.Debug("parseCase: default block parsed, current token: %s (%s)", p.peek.Literal, p.peek.Type.String())
			// После default запятая обязательна!
			if p.peek.Type != TOKEN_COMMA {
				p.hasErrors = true
				errors.NewFatalError("0019",
					fmt.Sprintf("Expected ',' after default branch (at %d:%d)", p.peek.Line, p.peek.Column),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance() // съедаем запятую
			break
		}

		// Обычная ветка
		body := p.parseBlock()
		if body == nil {
			return nil
		}
		debug.Debug("parseCase: body parsed, current token after body: %s (%s)", p.peek.Literal, p.peek.Type.String())

		branches = append(branches, &CaseBranch{
			Pattern: pattern,
			Body:    body,
		})

		// ЗАПЯТАЯ ОБЯЗАТЕЛЬНА ПОСЛЕ КАЖДОЙ ВЕТКИ
		if p.peek.Type == TOKEN_COMMA {
			debug.Debug("parseCase: found comma")
			p.advance()
		} else {
			debug.Debug("parseCase: expected comma, got: %s (%s)", p.peek.Literal, p.peek.Type.String())
			p.hasErrors = true
			errors.NewFatalError("0019",
				fmt.Sprintf("Expected ',' after case branch (at %d:%d)", p.peek.Line, p.peek.Column),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
	}

	p.expect(TOKEN_RBRACE)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	return &CaseStmt{
		Value:    value,
		Branches: branches,
		Default:  defaultBlock,
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

	// Проверяем, не arr ли это с типом
	if p.peek.Literal == "arr" {
		p.advance()

		var elemType string

		if p.peek.Type == TOKEN_LBRACKET {
			fullArrayType := p.parseArrayType() // "arr[int]" или "arr[arr[int]]"
			if p.hasErrors || errors.HasFatal() {
				return nil
			}
			elemType = parseArrayElemTypeFromFullType(fullArrayType)
		}

		// Имя переменной
		if p.peek.Type != TOKEN_IDENT {
			p.hasErrors = true
			errors.NewFatalError("0011",
				fmt.Sprintf("Expected variable name, got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
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

		return &VarDecl{
			Name:     name,
			Type:     "arr",
			ElemType: elemType,
			Expr:     expr,
			IsArray:  true,
		}
	}

	// Обычная переменная
	varType := p.peek.Literal
	p.advance()

	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0011",
			fmt.Sprintf("Expected variable name, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
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

	return &VarDecl{Name: name, Type: varType, Expr: expr, IsArray: false}
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
				fmt.Sprintf("Expected function call after '.', got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
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

func (p *Parser) parseTypeOf() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // type
	p.expect(TOKEN_LPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	expr := p.parseExpression()
	if expr == nil {
		return nil
	}

	p.expect(TOKEN_RPAREN)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	return &TypeOf{Expr: expr}
}

func (p *Parser) parseExpression() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}
	return p.parseBinary(0)
}

func (p *Parser) parseUnary() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	if p.peek.Type == TOKEN_NOT {
		p.advance()
		expr := p.parseUnary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Op: "!", Expr: expr}
	}
	if p.peek.Type == TOKEN_MINUS {
		p.advance()
		expr := p.parseUnary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Op: "-", Expr: expr}
	}
	return p.parsePrimary()
}

func (p *Parser) parseBinary(prec int) Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	left := p.parseUnary()
	if left == nil {
		return nil
	}

	for {
		if p.hasErrors || errors.HasFatal() {
			break
		}

		op := p.peek.Literal
		var nextPrec int
		switch p.peek.Type {
		case TOKEN_DOTDOT:
			nextPrec = 5
		case TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT:
			nextPrec = 4
		case TOKEN_PLUS, TOKEN_MINUS:
			nextPrec = 3
		case TOKEN_LT, TOKEN_GT, TOKEN_EQUALS:
			nextPrec = 2
		case TOKEN_AND:
			nextPrec = 1
		case TOKEN_OR:
			nextPrec = 0
		default:
			return left
		}

		if nextPrec < prec {
			break
		}

		p.advance()
		right := p.parseBinary(nextPrec + 1)
		if right == nil {
			return nil
		}

		if op == ".." {
			left = &RangeExpr{Start: left, End: right}
		} else {
			left = &BinaryExpr{Left: left, Op: op, Right: right}
		}
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

		// arr[index]
		if p.peek.Type == TOKEN_LBRACKET {
			p.advance()
			index := p.parseExpression()
			if index == nil {
				return nil
			}
			if p.peek.Type != TOKEN_RBRACKET {
				p.hasErrors = true
				errors.NewFatalError("0023",
					fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()
			return &ArrayIndex{Name: name, Index: index}
		}

		// arr.length или x.func()
		if p.peek.Type == TOKEN_DOT {
			p.advance()
			if p.peek.Literal == "length" {
				p.advance()
				return &ArrayLength{Name: name}
			}
			// x.func() — вызов метода
			if p.peek.Type == TOKEN_IDENT {
				funcName := p.peek.Literal
				p.advance()

				if p.peek.Type != TOKEN_LPAREN {
					p.hasErrors = true
					errors.NewFatalError("0040",
						fmt.Sprintf("Expected '(' after '%s'", funcName),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				p.advance() // (

				var callArgs []Node
				if p.peek.Type != TOKEN_RPAREN {
					for {
						arg := p.parseExpression()
						if arg == nil {
							return nil
						}
						callArgs = append(callArgs, arg)
						if p.peek.Type == TOKEN_COMMA {
							p.advance()
							continue
						}
						break
					}
				}
				p.expect(TOKEN_RPAREN)

				// Используем полное имя с точкой
				return &CallExpr{
					Name:     name + "." + funcName,
					Args:     callArgs,
					Receiver: name,
				}
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
		if p.peek.Literal == "type" {
			return p.parseTypeOf()
		}
		p.hasErrors = true
		errors.NewFatalError("0012",
			fmt.Sprintf("Unexpected keyword in expression: '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		p.advance()
		return nil

	case TOKEN_LPAREN:
		p.advance()
		first := p.parseExpression()
		if first == nil {
			return nil
		}

		// Кортеж (a, b, c).func()
		if p.peek.Type == TOKEN_COMMA {
			items := []Node{first}
			for p.peek.Type == TOKEN_COMMA {
				p.advance()
				item := p.parseExpression()
				if item == nil {
					return nil
				}
				items = append(items, item)
			}
			p.expect(TOKEN_RPAREN)

			// Проверяем .func()
			if p.peek.Type == TOKEN_DOT {
				p.advance()
				if p.peek.Type != TOKEN_IDENT {
					p.hasErrors = true
					errors.NewFatalError("0040",
						fmt.Sprintf("Expected method name after '.'"),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				funcName := p.peek.Literal
				p.advance()
				p.expect(TOKEN_LPAREN)

				var callArgs []Node
				if p.peek.Type != TOKEN_RPAREN {
					for {
						arg := p.parseExpression()
						if arg == nil {
							return nil
						}
						callArgs = append(callArgs, arg)
						if p.peek.Type == TOKEN_COMMA {
							p.advance()
							continue
						}
						break
					}
				}
				p.expect(TOKEN_RPAREN)

				allArgs := items
				allArgs = append(allArgs, callArgs...)
				return &CallExpr{Name: funcName, Args: allArgs}
			}

			return &ArrayLiteral{Elements: items}
		}

		p.expect(TOKEN_RPAREN)
		if p.hasErrors || errors.HasFatal() {
			return nil
		}

		// (1..5).func() — метод на RangeExpr
		if p.peek.Type == TOKEN_DOT {
			p.advance()
			if p.peek.Type == TOKEN_IDENT {
				funcName := p.peek.Literal
				p.advance()

				if p.peek.Type != TOKEN_LPAREN {
					p.hasErrors = true
					errors.NewFatalError("0040",
						fmt.Sprintf("Expected '(' after '%s'", funcName),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				p.advance()

				var callArgs []Node
				if p.peek.Type != TOKEN_RPAREN {
					for {
						arg := p.parseExpression()
						if arg == nil {
							return nil
						}
						callArgs = append(callArgs, arg)
						if p.peek.Type == TOKEN_COMMA {
							p.advance()
							continue
						}
						break
					}
				}
				p.expect(TOKEN_RPAREN)

				// Если first — RangeExpr, раскрываем в аргументы
				if rangeExpr, ok := first.(*RangeExpr); ok {
					return &CallRangeExpr{
						Name:  funcName,
						Range: rangeExpr,
						Extra: callArgs,
					}
				}

				allArgs := []Node{first}
				allArgs = append(allArgs, callArgs...)
				return &CallExpr{Name: funcName, Args: allArgs}
			}
		}

		return first

	case TOKEN_LBRACKET:
		p.advance()
		elements := []Node{}
		if p.peek.Type != TOKEN_RBRACKET {
			for {
				expr := p.parseExpression()
				if expr == nil {
					return nil
				}
				elements = append(elements, expr)
				if p.peek.Type == TOKEN_COMMA {
					p.advance()
					continue
				} else {
					break
				}
			}
		}
		if p.peek.Type != TOKEN_RBRACKET {
			p.hasErrors = true
			errors.NewFatalError("0022",
				fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()
		return &ArrayLiteral{Elements: elements}

	default:
		p.hasErrors = true
		errors.NewFatalError("0013",
			fmt.Sprintf("Unexpected token in expression: '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		p.advance()
		return nil
	}
}

func (p *Parser) parseArrayType() string {
	// уже прочитали "arr", не читаем
	if p.peek.Type != TOKEN_LBRACKET {
		return "arr"
	}

	p.advance() // [

	var innerType string
	if p.peek.Type == TOKEN_KEYWORD {
		switch p.peek.Literal {
		case "int", "string", "float", "double", "bool", "char", "any":
			innerType = p.peek.Literal
			p.advance()
		case "arr":
			p.advance()
			innerType = p.parseArrayType() // возвращает "arr[int]" или "arr" или "arr[arr]"
		default:
			p.hasErrors = true
			errors.NewFatalError("0020",
				fmt.Sprintf("Expected type in arr[], got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return ""
		}
	} else {
		p.hasErrors = true
		errors.NewFatalError("0020",
			fmt.Sprintf("Expected type in arr[], got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}

	if p.peek.Type != TOKEN_RBRACKET {
		p.hasErrors = true
		errors.NewFatalError("0021",
			fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}
	p.advance() // ]

	// Возвращаем тип ВСЕГО массива: "arr[innerType]"
	return "arr[" + innerType + "]"
}

// parseArrayElemTypeFromFullType извлекает тип элемента из полного типа массива
// "arr[int]" → "int"
// "arr[arr[int]]" → "arr[int]"
// "arr[arr]" → "arr"
// "arr" → ""
func parseArrayElemTypeFromFullType(fullType string) string {
	if fullType == "arr" {
		return "" // нетипизированный
	}
	if !strings.HasPrefix(fullType, "arr[") {
		return fullType
	}
	return fullType[4 : len(fullType)-1]
}
