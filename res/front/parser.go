package front

import (
	"fmt"
	"skrp/res/debug"
	"skrp/res/errors"
	"strconv"
	"strings"
)

type Parser struct {
	lexer     *Lexer
	current   Token
	peek      Token
	hasErrors bool
	FileName  string
}

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

// pos возвращает текущую позицию peek
func (p *Parser) pos() Position {
	return Position{Line: p.peek.Line, Column: p.peek.Column}
}

// posPrev возвращает позицию current (только что съеденного)
func (p *Parser) posPrev() Position {
	return Position{Line: p.current.Line, Column: p.current.Column}
}

func (p *Parser) expect(tt TokenType) Token {
	if p.peek.Type == tt {
		p.advance()
		return p.current
	}
	p.hasErrors = true

	if p.peek.Type == TOKEN_EOF {
		errors.NewFatalError("0523",
			fmt.Sprintf("Unexpected end of file — expected '%s'", tt.String()),
			p.peek.Line, p.peek.Column, p.FileName)
		return p.current
	}

	code, msg := expectErrorInfo(tt)
	errors.NewFatalError(code,
		fmt.Sprintf("%s, got '%s'", msg, p.peek.Literal),
		p.peek.Line, p.peek.Column, p.FileName)
	return p.current
}

// expectErrorInfo возвращает код и сообщение для ожидаемого токена.
func expectErrorInfo(tt TokenType) (string, string) {
	switch tt {
	case TOKEN_LPAREN:
		return "0540", "Expected '('"
	case TOKEN_RPAREN:
		return "0541", "Expected ')'"
	case TOKEN_LBRACE:
		return "0542", "Expected '{'"
	case TOKEN_RBRACE:
		return "0543", "Expected '}'"
	case TOKEN_LBRACKET:
		return "0544", "Expected '['"
	case TOKEN_RBRACKET:
		return "0545", "Expected ']'"
	case TOKEN_COLON:
		return "0546", "Expected ':'"
	case TOKEN_SEMICOLON:
		return "0547", "Expected ';'"
	case TOKEN_COMMA:
		return "0548", "Expected ','"
	case TOKEN_EQUALS:
		return "0549", "Expected '='"
	case TOKEN_DOTDOT:
		return "0550", "Expected '..'"
	case TOKEN_IDENT:
		return "0551", "Expected identifier"
	default:
		return "0552", "Expected expression"
	}
}

func (p *Parser) isType(token Token) bool {
	result := false
	switch token.Literal {
	case "void", "int", "string", "float", "double", "bool", "char", "arr", "dict", "any", "T":
		result = true
	}
	debug.Debug("isType(%s) = %v", token.Literal, result)
	return result
}

func (p *Parser) Parse() *Program {
	p.advance()

	if errors.HasFatal() {
		return &Program{Imports: []*Import{}, Functions: []*Function{}}
	}

	prog := &Program{
		Position:  p.pos(),
		Imports:   []*Import{},
		Functions: []*Function{},
	}

	debug.Debug("Starting parse, first token: %s type: %s", p.peek.Literal, p.peek.Type.String())

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

	// Обработка const-объявлений ошибок (до функций)
	for p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "const" {
		if p.hasErrors || errors.HasFatal() {
			break
		}
		errDecl := p.parseErrorDecl()
		if errDecl != nil {
			if ed, ok := errDecl.(*ErrorDecl); ok {
				prog.ErrorDecls = append(prog.ErrorDecls, ed)
			}
		}
		if errors.HasFatal() {
			return prog
		}
	}

	debug.Debug("After imports, current token: %s type: %s", p.peek.Literal, p.peek.Type.String())

	funcCount := 0
	for p.peek.Type != TOKEN_EOF {
		debug.Debug("Loop iteration %d: token='%s', type=%d", funcCount, p.peek.Literal, p.peek.Type)

		if p.hasErrors || errors.HasFatal() {
			break
		}

		if p.peek.Type == TOKEN_INCLUDE_C {
			inc := p.parseIncludeC()
			if inc != nil {
				prog.GlobalIncludeC = append(prog.GlobalIncludeC, inc.Code)
				debug.Debug("Global includeC parsed: %d bytes", len(inc.Code))
			}
			continue
		}

		if p.isType(p.peek) {
			debug.Debug("Found type: '%s', parsing function...", p.peek.Literal)
			fn := p.parseFunction()
			if fn != nil {
				prog.Functions = append(prog.Functions, fn)
				funcCount++
				debug.Debug("Function parsed: %s", fn.Name)
			}
		} else {
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

	pos := p.pos()
	p.advance() // use
	imp := &Import{Position: pos}

	if p.peek.Type == TOKEN_HASH {
		p.advance()
		imp.All = true
	}

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

	pos := p.pos()

	// Проверяем, что начинается с типа
	if !p.isType(p.peek) {
		p.hasErrors = true
		errors.NewFatalError("0500",
			fmt.Sprintf("Expected return type, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	// Тип возврата (может быть arr[int], arr[arr[int]] и т.д.)
	retType := p.parseType()
	if p.hasErrors || errors.HasFatal() {
		return nil
	}
	if retType == "" {
		p.hasErrors = true
		errors.NewFatalError("0500",
			"Expected return type",
			pos.Line, pos.Column, p.FileName)
		return nil
	}

	// Звёздочка неэкспортируемости
	isExport := true
	if p.peek.Type == TOKEN_STAR {
		isExport = false
		p.advance()
	}

	// Имя функции
	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0501",
			fmt.Sprintf("Expected function name, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	namePos := p.pos() // позиция peek ДО advance
	name := p.peek.Literal
	p.advance()

	// Открывающая скобка
	if p.peek.Type != TOKEN_LPAREN {
		p.hasErrors = true
		errors.NewFatalError("0507",
			fmt.Sprintf("Expected '(', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance()

	// Параметры
	params := []*Param{}
	if p.peek.Type != TOKEN_RPAREN {
		for {
			paramPos := p.pos()

			if !p.isType(p.peek) {
				p.hasErrors = true
				errors.NewFatalError("0502",
					fmt.Sprintf("Expected parameter type, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}

			paramType := p.parseType()
			if p.hasErrors || errors.HasFatal() {
				return nil
			}
			if paramType == "" {
				p.hasErrors = true
				errors.NewFatalError("0502",
					"Expected parameter type",
					paramPos.Line, paramPos.Column, p.FileName)
				return nil
			}

			if p.peek.Type != TOKEN_IDENT {
				p.hasErrors = true
				errors.NewFatalError("0503",
					fmt.Sprintf("Expected parameter name, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			paramName := p.peek.Literal
			p.advance()

			var defaultValue Node
			if p.peek.Type == TOKEN_EQUALS {
				p.advance()
				defaultValue = p.parseExpression()
				if defaultValue == nil {
					return nil
				}
			}

			params = append(params, &Param{
				Position:     paramPos,
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

	if p.peek.Type != TOKEN_RPAREN {
		p.hasErrors = true
		errors.NewFatalError("0508",
			fmt.Sprintf("Expected ')', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance()

	if p.peek.Type != TOKEN_LBRACE {
		p.hasErrors = true
		errors.NewFatalError("0509",
			fmt.Sprintf("Expected '{', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &Function{
		Position:   pos,
		NamePos:    namePos,
		Name:       name,
		ReturnType: retType,
		Params:     params,
		Body:       body,
		IsExport:   isExport,
		File:       p.FileName,
	}
}

func (p *Parser) parseBlock() *Block {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.expect(TOKEN_LBRACE)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	block := &Block{Position: pos, Statements: []Node{}}

	for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
		if p.hasErrors || errors.HasFatal() {
			break
		}
		stmt := p.parseStatement()
		if p.hasErrors || errors.HasFatal() {
			break
		}
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		// Опциональная ';' после statement'а (после выражений, вызовов, etc.)
		if p.peek.Type == TOKEN_SEMICOLON {
			p.advance()
		}
	}

	if p.peek.Type == TOKEN_EOF {
		p.hasErrors = true
		errors.NewFatalError("0523",
			"Unexpected end of file — expected '}'",
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
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
		case "IknowIdo":
			return p.parseIknowIdo()
		case "for":
			return p.parseFor()
		case "return":
			return p.parseReturn()
		case "case":
			return p.parseCase()
		case "try":
			return p.parseTry()
		case "const":
			return p.parseErrorDecl()
		case "throw":
			return p.parseThrow()
		case "int", "string", "float", "double", "bool", "char", "arr", "dict", "any", "T":
			return p.parseVarDecl()
		}
		p.hasErrors = true
		errors.NewFatalError("0597",
			fmt.Sprintf("Unexpected keyword in statement position: '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil

	case TOKEN_INCLUDE_C:
		return p.parseIncludeC()

	case TOKEN_IDENT:
		return p.parseAssignmentOrCall()

	case TOKEN_STRING, TOKEN_NUMBER, TOKEN_LPAREN:
		return p.parseExpression()

	case TOKEN_EOF:
		return nil

	default:
		p.hasErrors = true
		errors.NewFatalError("0598",
			fmt.Sprintf("Expected statement, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
}

func (p *Parser) parseIknowIdo() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // IknowIdo

	if p.peek.Type != TOKEN_LBRACE {
		p.hasErrors = true
		errors.NewFatalError("0640",
			fmt.Sprintf("Expected '{' after 'IknowIdo', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &IknowIdoBlock{
		Position: pos,
		Body:     body,
	}
}

func (p *Parser) parseTry() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // try

	// try-блок
	if p.peek.Type != TOKEN_LBRACE {
		p.hasErrors = true
		errors.NewFatalError("0574",
			fmt.Sprintf("Expected '{' after 'try', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}

	// catch-блоки
	catches := []*CatchClause{}

	for p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "catch" {
		catchPos := p.pos()
		p.advance() // catch

		clause := &CatchClause{
			Position: catchPos,
		}

		// catch (Type as e) { }
		if p.peek.Type == TOKEN_LPAREN {
			p.advance()

			// TypeName — Ident или Ident.Ident.Ident
			if p.peek.Type != TOKEN_IDENT && !(p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "Error") {
				p.hasErrors = true
				errors.NewFatalError("0576",
					fmt.Sprintf("Expected error type in catch, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			clause.TypeName = p.peek.Literal
			p.advance()

			// Цепочка .Ident
			for p.peek.Type == TOKEN_DOT {
				p.advance()
				if p.peek.Type != TOKEN_IDENT {
					p.hasErrors = true
					errors.NewFatalError("0576",
						fmt.Sprintf("Expected identifier after '.' in catch type, got '%s'", p.peek.Literal),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				clause.TypeName += "." + p.peek.Literal
				p.advance()
			}

			// as e
			if p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "as" {
				p.advance()

				if p.peek.Type != TOKEN_IDENT {
					p.hasErrors = true
					errors.NewFatalError("0577",
						fmt.Sprintf("Expected variable name after 'as', got '%s'", p.peek.Literal),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				clause.VarName = p.peek.Literal
				p.advance()
			}

			if p.peek.Type != TOKEN_RPAREN {
				p.hasErrors = true
				errors.NewFatalError("0578",
					fmt.Sprintf("Expected ')' after catch clause, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()
		}

		// Тело catch
		if p.peek.Type != TOKEN_LBRACE {
			p.hasErrors = true
			errors.NewFatalError("0575",
				fmt.Sprintf("Expected '{' after 'catch', got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		clause.Body = p.parseBlock()
		if clause.Body == nil {
			return nil
		}

		catches = append(catches, clause)
	}

	// Хотя бы один catch обязателен
	if len(catches) == 0 {
		p.hasErrors = true
		errors.NewFatalError("0519",
			"try requires at least one catch clause",
			pos.Line, pos.Column, p.FileName)
		return nil
	}

	return &TryStmt{
		Position: pos,
		Body:     body,
		Catches:  catches,
	}
}

func (p *Parser) parseIncludeC() *IncludeC {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // includeC

	if p.peek.Type != TOKEN_TILDE {
		p.hasErrors = true
		errors.NewFatalError("0510",
			fmt.Sprintf("Expected ~~~ after includeC, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	code := p.peek.Literal
	p.advance()

	return &IncludeC{Position: pos, Code: code}
}

func (p *Parser) parseIf() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
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

	for p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "elsif" {
		elsifPos := p.pos()
		p.advance()

		elsifCond := p.parseExpression()
		if elsifCond == nil {
			return nil
		}

		elsifThen := p.parseBlock()
		if elsifThen == nil {
			return nil
		}

		elsifs = append(elsifs, &Elsif{
			Position:  elsifPos,
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
		Position:  pos,
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

	pos := p.pos()
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

		branchPos := p.pos()

		// Default branch: _ { ... }
		if p.peek.Type == TOKEN_IDENT && p.peek.Literal == "_" {
			p.advance()

			defaultBlock = p.parseBlock()
			if defaultBlock == nil {
				return nil
			}

			if p.peek.Type != TOKEN_COMMA {
				p.hasErrors = true
				errors.NewFatalError("0572",
					fmt.Sprintf("Expected ',' after default branch (at %d:%d)", p.peek.Line, p.peek.Column),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()
			break
		}

		// Мультипаттерн: (a, b, c) { ... }
		patterns := []Node{}
		if p.peek.Type == TOKEN_LPAREN {
			p.advance() // (

			for {
				pat := p.parseExpression()
				if pat == nil {
					return nil
				}
				patterns = append(patterns, pat)

				if p.peek.Type == TOKEN_COMMA {
					p.advance()
					continue
				}
				break
			}

			if p.peek.Type != TOKEN_RPAREN {
				p.hasErrors = true
				errors.NewFatalError("0599",
					fmt.Sprintf("Expected ')' after multi-pattern, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance() // )
		} else {
			// Обычный паттерн
			pat := p.parseExpression()
			if pat == nil {
				return nil
			}
			patterns = append(patterns, pat)
		}

		body := p.parseBlock()
		if body == nil {
			return nil
		}

		branches = append(branches, &CaseBranch{
			Position: branchPos,
			Patterns: patterns,
			Body:     body,
		})

		if p.peek.Type == TOKEN_COMMA {
			p.advance()
		} else {
			p.hasErrors = true
			errors.NewFatalError("0512",
				fmt.Sprintf("Expected ',' after case branch (at %d:%d)", p.peek.Line, p.peek.Column),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
	}

	if len(branches) == 0 && defaultBlock == nil {
		p.hasErrors = true
		errors.NewFatalError("0522",
			"Empty case statement — at least one branch is required",
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	if p.peek.Type == TOKEN_EOF {
		p.hasErrors = true
		errors.NewFatalError("0523",
			"Unexpected end of file — expected '}' in case statement",
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	p.expect(TOKEN_RBRACE)
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	return &CaseStmt{
		Position: pos,
		Value:    value,
		Branches: branches,
		Default:  defaultBlock,
	}
}

func (p *Parser) parseWhile() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // while
	cond := p.parseExpression()
	if cond == nil {
		return nil
	}

	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &WhileStmt{Position: pos, Condition: cond, Body: body}
}

func (p *Parser) parseFor() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // for

	if p.peek.Type != TOKEN_LPAREN {
		p.hasErrors = true
		errors.NewFatalError("0507",
			fmt.Sprintf("Expected '(' after 'for', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance() // (

	// Тип переменной
	if !p.isType(p.peek) {
		p.hasErrors = true
		errors.NewFatalError("0502",
			fmt.Sprintf("Expected element type in for-in, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	varType := p.parseType()
	if varType == "" {
		return nil
	}

	// Имя переменной
	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0503",
			fmt.Sprintf("Expected element name in for-in, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	varName := p.peek.Literal
	p.advance()

	// Ключевое слово in
	if p.peek.Type != TOKEN_KEYWORD || p.peek.Literal != "in" {
		p.hasErrors = true
		errors.NewFatalError("0630",
			fmt.Sprintf("Expected 'in' in for-in, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance() // in

	// Iterable
	iterable := p.parseExpression()
	if iterable == nil {
		return nil
	}

	// Закрывающая скобка
	if p.peek.Type != TOKEN_RPAREN {
		p.hasErrors = true
		errors.NewFatalError("0541",
			fmt.Sprintf("Expected ')' in for-in, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance() // )

	// Тело
	body := p.parseBlock()
	if body == nil {
		return nil
	}

	return &ForInStmt{
		Position: pos,
		VarType:  varType,
		VarName:  varName,
		Iterable: iterable,
		Body:     body,
	}
}

func (p *Parser) parseReturn() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
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

	return &ReturnStmt{Position: pos, Expr: expr}
}

func (p *Parser) parseVarDecl() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()

	// Массив: arr, arr[int], arr[arr[int]]
	if p.peek.Literal == "arr" {
		fullType := p.parseType()
		if p.hasErrors || errors.HasFatal() {
			return nil
		}
		if fullType == "" {
			p.hasErrors = true
			errors.NewFatalError("0513",
				"Expected array type",
				pos.Line, pos.Column, p.FileName)
			return nil
		}

		// fullType = "arr" или "arr[int]" или "arr[arr[int]]"
		elemType := parseArrayElemTypeFromFullType(fullType)
		if elemType == "" {
			elemType = "any"
		}

		if p.peek.Type != TOKEN_IDENT {
			p.hasErrors = true
			errors.NewFatalError("0504",
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
			Position: pos,
			Name:     name,
			Type:     "arr",
			ElemType: elemType,
			Expr:     expr,
			IsArray:  true,
		}
	}

	// Скалярный тип или union
	if !p.isType(p.peek) {
		p.hasErrors = true
		errors.NewFatalError("0504",
			fmt.Sprintf("Expected type, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	varType := p.parseType()
	if varType == "" {
		return nil
	}

	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0504",
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
		Position: pos,
		Name:     name,
		Type:     varType,
		Expr:     expr,
		IsArray:  false,
	}
}

func (p *Parser) parseAssignmentOrCall() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	name := p.peek.Literal
	p.advance()

	// module.func(args)
	if p.peek.Type == TOKEN_DOT {
		p.advance()
		if p.peek.Type == TOKEN_IDENT {
			moduleName := name
			funcName := p.peek.Literal
			p.advance()
			fullName := moduleName + "." + funcName

			if p.peek.Type == TOKEN_LPAREN {
				return p.parseCall(fullName, pos)
			}

			p.hasErrors = true
			errors.NewFatalError("0511",
				fmt.Sprintf("Expected function call after '.', got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.hasErrors = true
		errors.NewFatalError("0520",
			fmt.Sprintf("Expected identifier after '.', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}

	// func(args)
	if p.peek.Type == TOKEN_LPAREN {
		return p.parseCall(name, pos)
	}

	// x[index] = value
	if p.peek.Type == TOKEN_LBRACKET {
		p.advance()
		index := p.parseExpression()
		if index == nil {
			return nil
		}
		if p.peek.Type != TOKEN_RBRACKET {
			p.hasErrors = true
			errors.NewFatalError("0516",
				fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()

		if p.peek.Type != TOKEN_EQUALS {
			p.hasErrors = true
			errors.NewFatalError("0516",
				fmt.Sprintf("Expected '=' after array index, got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()

		value := p.parseExpression()
		if value == nil {
			return nil
		}
		if p.peek.Type == TOKEN_SEMICOLON {
			p.advance()
		}
		return &Assign{
			Position: pos,
			Name:     name,
			Index:    index,
			Expr:     value,
		}
	}

	// x.field = value
	if p.peek.Type == TOKEN_DOT {
		p.advance()
		if p.peek.Type != TOKEN_IDENT {
			p.hasErrors = true
			errors.NewFatalError("0520",
				"Expected method name after '.'",
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		fieldName := p.peek.Literal
		p.advance()

		if p.peek.Type != TOKEN_EQUALS {
			p.hasErrors = true
			errors.NewFatalError("0653",
				fmt.Sprintf("Expected '=' after field name, got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()
		value := p.parseExpression()
		if value == nil {
			return nil
		}
		if p.peek.Type == TOKEN_SEMICOLON {
			p.advance()
		}
		return &Assign{Position: pos, Name: name, Field: fieldName, Expr: value}
	}

	// x = value
	if p.peek.Type == TOKEN_EQUALS {
		p.advance()
		expr := p.parseExpression()
		if expr == nil {
			return nil
		}
		if p.peek.Type == TOKEN_SEMICOLON {
			p.advance()
		}
		return &Assign{Position: pos, Name: name, Expr: expr}
	}

	// x;
	if p.peek.Type == TOKEN_SEMICOLON {
		p.advance()
		return &Ident{Position: pos, Name: name}
	}

	p.hasErrors = true
	errors.NewFatalError("0597",
		fmt.Sprintf("Unexpected token after identifier '%s': '%s'", name, p.peek.Literal),
		p.peek.Line, p.peek.Column, p.FileName)
	return nil
}

func (p *Parser) parseCall(name string, pos Position) Node {
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

	return &CallExpr{Position: pos, Name: name, Args: args}
}

func (p *Parser) parseTypeOf() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
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

	return &TypeOf{Position: pos, Expr: expr}
}

func (p *Parser) parseExpression() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	cond := p.parseBinary(0)
	if cond == nil {
		return nil
	}

	// 1..4.test() → CallRangeExpr
	if rangeExpr, ok := cond.(*RangeExpr); ok && p.peek.Type == TOKEN_DOT {
		p.advance() // .
		if p.peek.Type != TOKEN_IDENT {
			p.hasErrors = true
			errors.NewFatalError("0520",
				fmt.Sprintf("Expected method name after '.'"),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		funcName := p.peek.Literal
		funcPos := p.pos()
		p.advance()

		if p.peek.Type != TOKEN_LPAREN {
			p.hasErrors = true
			errors.NewFatalError("0520",
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

		return &CallRangeExpr{
			Position: funcPos,
			Name:     funcName,
			Range:    rangeExpr,
			Extra:    callArgs,
		}
	}

	if p.peek.Type == TOKEN_QUESTION {
		pos := p.pos()
		p.advance()
		thenExpr := p.parseExpression()
		if thenExpr == nil {
			return nil
		}

		if p.peek.Type != TOKEN_COLON {
			p.hasErrors = true
			errors.NewFatalError("0521",
				fmt.Sprintf("Expected ':' in ternary, got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()

		elseExpr := p.parseExpression()
		if elseExpr == nil {
			return nil
		}

		return &TernaryExpr{Position: pos, Condition: cond, Then: thenExpr, Else: elseExpr}
	}

	return cond
}

func (p *Parser) parseUnary() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()

	if p.peek.Type == TOKEN_DOLLAR {
		p.advance()
		expr := p.parseUnary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Position: pos, Op: "$", Expr: expr}
	}

	if p.peek.Type == TOKEN_NOT {
		p.advance()
		expr := p.parseUnary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Position: pos, Op: "!", Expr: expr}
	}
	if p.peek.Type == TOKEN_MINUS {
		p.advance()
		expr := p.parseUnary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Position: pos, Op: "-", Expr: expr}
	}

	expr := p.parsePrimary()
	if expr == nil {
		return nil
	}

	// Постфиксные операции: .field, .method(), [index]
	expr = p.parsePostfix(expr)
	if expr == nil {
		return nil
	}

	// Постфиксный формат ^Nf или ^X...
	if p.peek.Type == TOKEN_CARET {
		return p.parseFormatSuffix(expr)
	}

	return expr
}

// parseFormatSuffix читает ^Nf или ^X... после выражения.
func (p *Parser) parseFormatSuffix(expr Node) Node {
	if p.peek.Type != TOKEN_CARET {
		return expr
	}

	pos := p.pos()
	p.advance() // ^

	// ^Nf — ^ + число + f
	if p.peek.Type == TOKEN_NUMBER {
		val := p.peek.Literal
		p.advance()

		if !strings.HasSuffix(val, "f") && !strings.HasSuffix(val, "F") {
			p.hasErrors = true
			errors.NewFatalError("0620",
				fmt.Sprintf("Expected 'f' after precision in ^%s", val),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}

		nStr := val[:len(val)-1]
		n, err := strconv.Atoi(nStr)
		if err != nil || n < 0 {
			p.hasErrors = true
			errors.NewFatalError("0621",
				fmt.Sprintf("Invalid precision '%s' in format", nStr),
				pos.Line, pos.Column, p.FileName)
			return nil
		}
		if n > 20 {
			p.hasErrors = true
			errors.NewFatalError("0622",
				fmt.Sprintf("Precision %d too large (max 20)", n),
				pos.Line, pos.Column, p.FileName)
			return nil
		}

		return &FormatExpr{
			Position: pos,
			Expr:     expr,
			Mode:     "Nf",
			N:        n,
		}
	}

	// ^X... — ^ + X + (цифры | {цифры})
	if p.peek.Type == TOKEN_IDENT && strings.HasPrefix(p.peek.Literal, "X") {
		val := p.peek.Literal[1:] // убрать X
		p.advance()

		// ^X{1,8}
		if p.peek.Type == TOKEN_LBRACE {
			p.advance()

			trimSet := ""
			for p.peek.Type == TOKEN_NUMBER {
				trimSet += p.peek.Literal
				p.advance()
				if p.peek.Type == TOKEN_COMMA {
					p.advance()
				}
			}

			if p.peek.Type != TOKEN_RBRACE {
				p.hasErrors = true
				errors.NewFatalError("0623",
					fmt.Sprintf("Expected '}' in ^X{...}, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()

			if trimSet == "" {
				p.hasErrors = true
				errors.NewFatalError("0624",
					"Empty trim set in ^X{}",
					pos.Line, pos.Column, p.FileName)
				return nil
			}

			return &FormatExpr{
				Position: pos,
				Expr:     expr,
				Mode:     "X",
				TrimSet:  trimSet,
			}
		}

		// ^X9 — уже val содержит "9"
		if val == "" {
			p.hasErrors = true
			errors.NewFatalError("0624",
				"Expected digits after 'X' in format",
				pos.Line, pos.Column, p.FileName)
			return nil
		}

		return &FormatExpr{
			Position: pos,
			Expr:     expr,
			Mode:     "X",
			TrimSet:  val,
		}
	}

	p.hasErrors = true
	errors.NewFatalError("0625",
		fmt.Sprintf("Expected 'Nf' or 'X...' after '^', got '%s'", p.peek.Literal),
		p.peek.Line, p.peek.Column, p.FileName)
	return nil
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
		opPos := p.pos()
		var nextPrec int
		switch p.peek.Type {
		case TOKEN_DOTDOT:
			nextPrec = 6
		case TOKEN_POW:
			nextPrec = 5
		case TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT:
			nextPrec = 4
		case TOKEN_PLUS, TOKEN_MINUS:
			nextPrec = 3
		case TOKEN_LT, TOKEN_GT, TOKEN_EQUALS,
			TOKEN_NEQ, TOKEN_EQEQ, TOKEN_LTE, TOKEN_GTE:
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

		// ** — правоассоциативный
		nextMinPrec := nextPrec + 1
		if nextPrec == 5 { // TOKEN_POW
			nextMinPrec = nextPrec
		}
		right := p.parseBinary(nextMinPrec)
		if right == nil {
			return nil
		}

		if op == ".." {
			left = &RangeExpr{Position: opPos, Start: left, End: right}
		} else {
			left = &BinaryExpr{Position: opPos, Left: left, Op: op, Right: right}
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
		pos := p.pos()
		val := p.peek.Literal
		p.advance()
		return &Number{Position: pos, Value: val}

	case TOKEN_UNICODE_LIT:
		pos := p.pos()
		hex := p.peek.Literal
		p.advance()

		cp, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			p.hasErrors = true
			errors.NewFatalError("0124",
				fmt.Sprintf("Invalid Unicode codepoint '%s'", hex),
				pos.Line, pos.Column, p.FileName)
			return nil
		}
		if cp > 0x10FFFF {
			p.hasErrors = true
			errors.NewFatalError("0125",
				fmt.Sprintf("Unicode codepoint out of range: %s (max 10FFFF)", hex),
				pos.Line, pos.Column, p.FileName)
			return nil
		}
		if cp >= 0xD800 && cp <= 0xDFFF {
			p.hasErrors = true
			errors.NewFatalError("0126",
				fmt.Sprintf("Unicode codepoint %s is in surrogate range (D800-DFFF) — not valid UTF-8", hex),
				pos.Line, pos.Column, p.FileName)
			return nil
		}

		return &UnicodeLiteral{Position: pos, Codepoint: uint32(cp)}

	case TOKEN_LBRACE:
		pos := p.pos()
		p.advance() // {
		elements := []*DictElement{}
		if p.peek.Type != TOKEN_RBRACE {
			for {
				// ключ
				if p.peek.Type != TOKEN_IDENT {
					p.hasErrors = true
					errors.NewFatalError("0650",
						fmt.Sprintf("Expected key in dict literal, got '%s'", p.peek.Literal),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				keyName := p.peek.Literal
				keyPos := p.pos()
				p.advance()

				// :
				if p.peek.Type != TOKEN_COLON {
					p.hasErrors = true
					errors.NewFatalError("0651",
						fmt.Sprintf("Expected ':' after key, got '%s'", p.peek.Literal),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				p.advance()

				// значение
				value := p.parseExpression()
				if value == nil {
					return nil
				}

				elements = append(elements, &DictElement{
					Position: keyPos,
					Key:      keyName,
					Value:    value,
				})

				if p.peek.Type == TOKEN_COMMA {
					p.advance()
					continue
				}
				break
			}
		}
		if p.peek.Type != TOKEN_RBRACE {
			p.hasErrors = true
			errors.NewFatalError("0652",
				fmt.Sprintf("Expected '}' in dict literal, got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()
		return &DictLiteral{Position: pos, Elements: elements}

	case TOKEN_STRING:
		pos := p.pos()
		val := p.peek.Literal
		p.advance()
		return &String{Position: pos, Value: val}

	case TOKEN_CHARLIT:
		pos := p.pos()
		val := p.peek.Literal
		p.advance()
		if len(val) == 0 {
			p.hasErrors = true
			errors.NewFatalError("0112", "Empty char literal",
				pos.Line, pos.Column, p.FileName)
			return nil
		}
		return &CharLiteral{Position: pos, Value: val[0]}

	case TOKEN_IDENT:
		pos := p.pos()
		name := p.peek.Literal
		p.advance()

		// ErrorInstance: Name{field: value, ...}
		if p.peek.Type == TOKEN_LBRACE {
			return p.parseErrorInstance(name, pos)
		}

		// Простой вызов функции: name(args)
		if p.peek.Type == TOKEN_LPAREN {
			return p.parseCall(name, pos)
		}

		return &Ident{Position: pos, Name: name}

	case TOKEN_KEYWORD:
		pos := p.pos()

		if p.peek.Literal == "null" {
			p.advance()
			return &NullLiteral{Position: pos}
		}

		if p.peek.Literal == "true" || p.peek.Literal == "false" {
			val := p.peek.Literal
			p.advance()
			return &Ident{Position: pos, Name: val}
		}
		if p.peek.Literal == "const" {
			p.advance()
			return p.parsePrimary()
		}
		if p.peek.Literal == "type" {
			return p.parseTypeOf()
		}
		p.hasErrors = true
		errors.NewFatalError("0505",
			fmt.Sprintf("Unexpected keyword in expression: '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		p.advance()
		return nil

	case TOKEN_LPAREN:
		pos := p.pos()
		p.advance()
		first := p.parseExpression()
		if first == nil {
			return nil
		}

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
			if p.hasErrors || errors.HasFatal() {
				return nil
			}
			return &ArrayLiteral{Position: pos, Elements: items}
		}

		p.expect(TOKEN_RPAREN)
		if p.hasErrors || errors.HasFatal() {
			return nil
		}

		// Возвращаем выражение в скобках как есть.
		// Постфикс (.method, [index]) обработает parsePostfix.
		return first

	case TOKEN_LBRACKET:
		pos := p.pos()
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
			errors.NewFatalError("0515",
				fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}
		p.advance()
		return &ArrayLiteral{Position: pos, Elements: elements}

	default:
		p.hasErrors = true

		if p.peek.Type == TOKEN_EOF {
			errors.NewFatalError("0523",
				"Unexpected end of file — expected an expression",
				p.peek.Line, p.peek.Column, p.FileName)
			return nil
		}

		errors.NewFatalError("0506",
			fmt.Sprintf("Unexpected token in expression: '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		p.advance()
		return nil
	}
}

func (p *Parser) parseArrayType() string {
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
		case "void":
			p.hasErrors = true
			errors.NewFatalError("0560",
				"Array element type cannot be 'void'",
				p.peek.Line, p.peek.Column, p.FileName)
			return ""
		case "arr":
			p.advance()
			innerType = p.parseArrayType()
		default:
			p.hasErrors = true
			errors.NewFatalError("0561",
				fmt.Sprintf("Unknown element type in arr[], got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return ""
		}
	} else {
		p.hasErrors = true
		errors.NewFatalError("0561",
			fmt.Sprintf("Unknown element type in arr[], got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}

	if p.peek.Type != TOKEN_RBRACKET {
		p.hasErrors = true
		errors.NewFatalError("0514",
			fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}
	p.advance() // ]

	return "arr[" + innerType + "]"
}

func parseArrayElemTypeFromFullType(fullType string) string {
	if fullType == "arr" {
		return ""
	}
	if !strings.HasPrefix(fullType, "arr[") {
		return fullType
	}
	return fullType[4 : len(fullType)-1]
}

func (p *Parser) parseErrorDecl() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // const

	// Имя типа — с заглавной буквы
	if p.peek.Type != TOKEN_IDENT {
		p.hasErrors = true
		errors.NewFatalError("0580",
			fmt.Sprintf("Expected error type name, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	name := p.peek.Literal
	namePos := p.pos()
	p.advance()

	if name[0] < 'A' || name[0] > 'Z' {
		p.hasErrors = true
		errors.NewFatalError("0581",
			fmt.Sprintf("Error type name must start with uppercase letter, got '%s'", name),
			namePos.Line, namePos.Column, p.FileName)
		return nil
	}

	// Поля: {field: type[default], ...}
	if p.peek.Type != TOKEN_LBRACE {
		p.hasErrors = true
		errors.NewFatalError("0582",
			fmt.Sprintf("Expected '{' after error type name, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance() // {

	fields := []*ErrorField{}

	if p.peek.Type != TOKEN_RBRACE {
		for {
			fieldPos := p.pos()

			// Имя поля
			if p.peek.Type != TOKEN_IDENT {
				p.hasErrors = true
				errors.NewFatalError("0583",
					fmt.Sprintf("Expected field name, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			fieldName := p.peek.Literal
			p.advance()

			// Двоеточие
			if p.peek.Type != TOKEN_COLON {
				p.hasErrors = true
				errors.NewFatalError("0584",
					fmt.Sprintf("Expected ':' after field name, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()

			// Тип
			if !p.isType(p.peek) {
				p.hasErrors = true
				errors.NewFatalError("0585",
					fmt.Sprintf("Expected field type, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			fieldType := p.peek.Literal
			p.advance()

			// Default: [expr]
			var defaultValue Node
			if p.peek.Type == TOKEN_LBRACKET {
				p.advance()
				defaultValue = p.parseExpression()
				if defaultValue == nil {
					return nil
				}
				if p.peek.Type != TOKEN_RBRACKET {
					p.hasErrors = true
					errors.NewFatalError("0586",
						fmt.Sprintf("Expected ']' after default value, got '%s'", p.peek.Literal),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				p.advance()
			}

			fields = append(fields, &ErrorField{
				Position:     fieldPos,
				Name:         fieldName,
				Type:         fieldType,
				DefaultValue: defaultValue,
			})

			if p.peek.Type == TOKEN_COMMA {
				p.advance()
				continue
			}
			break
		}
	}

	if p.peek.Type != TOKEN_RBRACE {
		p.hasErrors = true
		errors.NewFatalError("0587",
			fmt.Sprintf("Expected '}' after error fields, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance() // }

	// = new Parent  или  = Parent
	if p.peek.Type != TOKEN_EQUALS {
		p.hasErrors = true
		errors.NewFatalError("0588",
			fmt.Sprintf("Expected '=' after error fields, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance()

	isNew := false
	if p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "new" {
		isNew = true
		p.advance()
	}

	// Родитель
	if p.peek.Type != TOKEN_IDENT && !(p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "Error") {
		p.hasErrors = true
		errors.NewFatalError("0589",
			fmt.Sprintf("Expected parent error type, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	parent := p.peek.Literal
	p.advance()

	return &ErrorDecl{
		Position: pos,
		Name:     name,
		Fields:   fields,
		Parent:   parent,
		IsNew:    isNew,
	}
}

func (p *Parser) parseErrorInstance(typeName string, pos Position) Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	p.advance() // {

	fields := make(map[string]Node)

	if p.peek.Type != TOKEN_RBRACE {
		for {
			// Имя поля
			if p.peek.Type != TOKEN_IDENT {
				p.hasErrors = true
				errors.NewFatalError("0591",
					fmt.Sprintf("Expected field name, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			fieldName := p.peek.Literal
			p.advance()

			// Двоеточие
			if p.peek.Type != TOKEN_COLON {
				p.hasErrors = true
				errors.NewFatalError("0592",
					fmt.Sprintf("Expected ':' after field name, got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()

			// Значение
			value := p.parseExpression()
			if value == nil {
				return nil
			}

			if _, exists := fields[fieldName]; exists {
				p.hasErrors = true
				errors.NewFatalError("0594",
					fmt.Sprintf("Duplicate field '%s' in error instance", fieldName),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			fields[fieldName] = value

			if p.peek.Type == TOKEN_COMMA {
				p.advance()
				continue
			}
			break
		}
	}

	if p.peek.Type != TOKEN_RBRACE {
		p.hasErrors = true
		errors.NewFatalError("0593",
			fmt.Sprintf("Expected '}' after error fields, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
	p.advance() // }

	return &ErrorInstance{
		Position: pos,
		TypeName: typeName,
		Fields:   fields,
	}
}

func (p *Parser) parseThrow() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // throw

	// Expression: IncorrectType{...} или переменная
	if p.peek.Type == TOKEN_SEMICOLON || p.peek.Type == TOKEN_RBRACE || p.peek.Type == TOKEN_EOF {
		p.hasErrors = true
		errors.NewFatalError("0579",
			"throw requires an expression",
			pos.Line, pos.Column, p.FileName)
		return nil
	}

	expr := p.parseExpression()
	if expr == nil {
		return nil
	}

	if p.peek.Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return &ThrowStmt{
		Position: pos,
		Expr:     expr,
	}
}

// parseType читает тип: простое имя, arr[...], T<...>.
// Возвращает полную строку типа: "int", "arr[int]", "arr[any]", "T<int,float>".
// Для T<A> возвращает "A" (нормализация).
// Для T<A,B> возвращает "T<A,B>".
func (p *Parser) parseType() string {
	// T<...>
	if p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "T" {
		return p.parseUnionType()
	}

	if p.peek.Literal == "arr" {
		p.advance()
		if p.peek.Type == TOKEN_LBRACKET {
			return p.parseArrayType()
		}
		return "arr[any]"
	}

	if !p.isType(p.peek) {
		return ""
	}

	typ := p.peek.Literal
	p.advance()
	return typ
}

// parseUnionType читает T<A, B, ...>.
// T<A> → "A". T<A,B> → "T<A,B>".
// Запрещает вложенные T<...>.
func (p *Parser) parseUnionType() string {
	p.advance() // T

	if p.peek.Type != TOKEN_LT {
		p.hasErrors = true
		errors.NewFatalError("0608",
			fmt.Sprintf("Expected '<' after 'T', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}
	p.advance()

	types := []string{}
	for {
		// Явно запрещаем вложенные T<...>
		if p.peek.Type == TOKEN_KEYWORD && p.peek.Literal == "T" {
			p.hasErrors = true
			errors.NewFatalError("0611",
				"Cannot nest T<...> inside T<...>",
				p.peek.Line, p.peek.Column, p.FileName)
			return ""
		}

		t := p.parseType()
		if t == "" {
			return ""
		}
		types = append(types, t)

		if p.peek.Type == TOKEN_COMMA {
			p.advance()
			continue
		}
		break
	}

	if len(types) == 0 {
		p.hasErrors = true
		errors.NewFatalError("0610",
			"T<...> requires at least one type",
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}

	if p.peek.Type != TOKEN_GT {
		p.hasErrors = true
		errors.NewFatalError("0609",
			fmt.Sprintf("Expected '>' after T<...>, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return ""
	}
	p.advance()

	if len(types) == 1 {
		return types[0]
	}
	return "T<" + strings.Join(types, ",") + ">"
}

// parsePostfix обрабатывает цепочку постфиксных операций:
//
//	expr.field
//	expr.method(args)
//	expr.length
//	expr[index]
//
// Работает для ЛЮБОГО выражения, а не только для Ident.
func (p *Parser) parsePostfix(expr Node) Node {
	if p.hasErrors || errors.HasFatal() {
		return expr
	}

	for {
		if p.hasErrors || errors.HasFatal() {
			return nil
		}

		// [index]
		if p.peek.Type == TOKEN_LBRACKET {
			p.advance()
			indexPos := p.pos()
			index := p.parseExpression()
			if index == nil {
				return nil
			}
			if p.peek.Type != TOKEN_RBRACKET {
				p.hasErrors = true
				errors.NewFatalError("0516",
					fmt.Sprintf("Expected ']', got '%s'", p.peek.Literal),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()

			if ident, ok := expr.(*Ident); ok {
				expr = &ArrayIndex{
					Position: ident.Position,
					Name:     ident.Name,
					Index:    index,
				}
			} else {
				p.hasErrors = true
				errors.NewFatalError("0516",
					"Array indexing is only supported on identifiers",
					indexPos.Line, indexPos.Column, p.FileName)
				return nil
			}
			continue
		}

		if p.peek.Type != TOKEN_DOT {
			return expr
		}
		p.advance()

		// .length
		if p.peek.Type == TOKEN_IDENT && p.peek.Literal == "length" {
			p.advance()
			lenPos := p.pos()
			if ident, ok := expr.(*Ident); ok {
				expr = &ArrayLength{
					Position: ident.Position,
					Name:     ident.Name,
				}
			} else {
				p.hasErrors = true
				errors.NewFatalError("0520",
					".length is only supported on identifiers",
					lenPos.Line, lenPos.Column, p.FileName)
				return nil
			}
			continue
		}

		// .field или .method(args)
		if p.peek.Type == TOKEN_IDENT {
			methodName := p.peek.Literal
			methodPos := p.pos()
			p.advance()

			if p.peek.Type == TOKEN_LPAREN {
				// Метод с аргументами: expr.method(args)
				p.advance()

				callArgs := []Node{}
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
				if p.peek.Type != TOKEN_RPAREN {
					p.hasErrors = true
					errors.NewFatalError("0602",
						fmt.Sprintf("Expected ')' after arguments, got '%s'", p.peek.Literal),
						p.peek.Line, p.peek.Column, p.FileName)
					return nil
				}
				p.advance()

				// Имя receiver'а (если это простой Ident) — для обратной совместимости.
				var recvName string
				if id, ok := expr.(*Ident); ok {
					recvName = id.Name
				}

				expr = &CallExpr{
					Position:     methodPos,
					Name:         methodName,
					Args:         callArgs,
					Receiver:     recvName,
					ReceiverNode: expr,
				}
				continue
			}

			// .field
			if ident, ok := expr.(*Ident); ok {
				expr = &FieldAccess{
					Position: ident.Position,
					Object:   ident.Name,
					Field:    methodName,
				}
			} else {
				p.hasErrors = true
				errors.NewFatalError("0520",
					"Field access is only supported on identifiers",
					methodPos.Line, methodPos.Column, p.FileName)
				return nil
			}
			continue
		}

		p.hasErrors = true
		errors.NewFatalError("0520",
			fmt.Sprintf("Expected field or method name after '.', got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
}
