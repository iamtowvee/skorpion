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

	errors.NewFatalError("0500",
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

	debug.Debug("After imports, current token: %s type: %s", p.peek.Literal, p.peek.Type.String())

	funcCount := 0
	for p.peek.Type != TOKEN_EOF {
		debug.Debug("Loop iteration %d: token='%s', type=%d", funcCount, p.peek.Literal, p.peek.Type)

		if p.hasErrors || errors.HasFatal() {
			break
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
		errors.NewFatalError("0501",
			fmt.Sprintf("Expected function name, got '%s'", p.peek.Literal),
			p.peek.Line, p.peek.Column, p.FileName)
		return nil
	}
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
			paramType := p.peek.Literal
			p.advance()

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
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
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
	case TOKEN_STRING, TOKEN_NUMBER, TOKEN_LPAREN:
		return p.parseExpression()
	}

	p.advance()
	return nil
}

func (p *Parser) parseIncludeC() *IncludeC {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	p.advance() // includeC

	if p.peek.Type != TOKEN_BACKTICK {
		p.hasErrors = true
		errors.NewFatalError("0510",
			fmt.Sprintf("Expected ``` after includeC, got '%s'", p.peek.Literal),
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
		pattern := p.parseExpression()
		if pattern == nil {
			return nil
		}

		if ident, ok := pattern.(*Ident); ok && ident.Name == "_" {
			defaultBlock = p.parseBlock()
			if defaultBlock == nil {
				return nil
			}
			if p.peek.Type != TOKEN_COMMA {
				p.hasErrors = true
				errors.NewFatalError("0512",
					fmt.Sprintf("Expected ',' after default branch (at %d:%d)", p.peek.Line, p.peek.Column),
					p.peek.Line, p.peek.Column, p.FileName)
				return nil
			}
			p.advance()
			break
		}

		body := p.parseBlock()
		if body == nil {
			return nil
		}

		branches = append(branches, &CaseBranch{
			Position: branchPos,
			Pattern:  pattern,
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

	return &ForStmt{Position: pos, Init: init, Cond: cond, Post: post, Body: body}
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

	if p.peek.Literal == "arr" {
		p.advance()

		var elemType string

		if p.peek.Type == TOKEN_LBRACKET {
			fullArrayType := p.parseArrayType()
			if p.hasErrors || errors.HasFatal() {
				return nil
			}
			elemType = parseArrayElemTypeFromFullType(fullArrayType)
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

	varType := p.peek.Literal
	p.advance()

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

	return &VarDecl{Position: pos, Name: name, Type: varType, Expr: expr, IsArray: false}
}

func (p *Parser) parseAssignmentOrCall() Node {
	if p.hasErrors || errors.HasFatal() {
		return nil
	}

	pos := p.pos()
	name := p.peek.Literal
	p.advance()

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
	}

	if p.peek.Type == TOKEN_LPAREN {
		return p.parseCall(name, pos)
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

	return &Assign{Position: pos, Name: name, Expr: expr}
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
		opPos := p.pos()
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

	case TOKEN_STRING:
		pos := p.pos()
		val := p.peek.Literal
		p.advance()
		str := &String{Position: pos, Value: val}

		if p.peek.Type == TOKEN_DOT {
			p.advance()
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

			allArgs := []Node{str}
			allArgs = append(allArgs, callArgs...)
			return &CallExpr{Position: funcPos, Name: funcName, Args: allArgs}
		}

		return str

	case TOKEN_DOLLAR:
		pos := p.pos()
		p.advance()
		expr := p.parsePrimary()
		if expr == nil {
			return nil
		}
		return &UnaryExpr{Position: pos, Op: "$", Expr: expr}

	case TOKEN_IDENT:
		pos := p.pos()
		name := p.peek.Literal
		p.advance()

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
			return &ArrayIndex{Position: pos, Name: name, Index: index}
		}

		if p.peek.Type == TOKEN_DOT {
			p.advance()
			if p.peek.Literal == "length" {
				p.advance()
				return &ArrayLength{Position: pos, Name: name}
			}
			if p.peek.Type == TOKEN_IDENT {
				funcName := p.peek.Literal
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

				return &CallExpr{
					Position: pos,
					Name:     name + "." + funcName,
					Args:     callArgs,
					Receiver: name,
				}
			}
		}

		if p.peek.Type == TOKEN_PLUS {
			p.advance()
			elem := p.parseExpression()
			if elem == nil {
				return nil
			}
			return &ArrayAdd{Position: pos, Name: name, Elem: elem}
		}

		if p.peek.Type == TOKEN_LPAREN {
			return p.parseCall(name, pos)
		}
		return &Ident{Position: pos, Name: name}

	case TOKEN_KEYWORD:
		pos := p.pos()
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

			if p.peek.Type == TOKEN_DOT {
				p.advance()
				if p.peek.Type != TOKEN_IDENT {
					p.hasErrors = true
					errors.NewFatalError("0520",
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
				return &CallExpr{Position: pos, Name: funcName, Args: allArgs}
			}

			return &ArrayLiteral{Position: pos, Elements: items}
		}

		p.expect(TOKEN_RPAREN)
		if p.hasErrors || errors.HasFatal() {
			return nil
		}

		if p.peek.Type == TOKEN_DOT {
			p.advance()
			if p.peek.Type == TOKEN_IDENT {
				funcName := p.peek.Literal
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

				if rangeExpr, ok := first.(*RangeExpr); ok {
					return &CallRangeExpr{
						Position: pos,
						Name:     funcName,
						Range:    rangeExpr,
						Extra:    callArgs,
					}
				}

				allArgs := []Node{first}
				allArgs = append(allArgs, callArgs...)
				return &CallExpr{Position: pos, Name: funcName, Args: allArgs}
			}
		}

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
		case "arr":
			p.advance()
			innerType = p.parseArrayType()
		default:
			p.hasErrors = true
			errors.NewFatalError("0513",
				fmt.Sprintf("Expected type in arr[], got '%s'", p.peek.Literal),
				p.peek.Line, p.peek.Column, p.FileName)
			return ""
		}
	} else {
		p.hasErrors = true
		errors.NewFatalError("0513",
			fmt.Sprintf("Expected type in arr[], got '%s'", p.peek.Literal),
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
