package front

import (
	"skrp/res/errors"
	"strings"
	"unicode"
)

type TokenType int

const (
	TOKEN_EOF TokenType = iota
	TOKEN_IDENT
	TOKEN_NUMBER
	TOKEN_STRING
	TOKEN_KEYWORD
	// Символы
	TOKEN_LPAREN
	TOKEN_RPAREN
	TOKEN_LBRACE
	TOKEN_RBRACE
	TOKEN_LBRACKET
	TOKEN_RBRACKET
	TOKEN_SEMICOLON
	TOKEN_COMMA
	TOKEN_PLUS
	TOKEN_MINUS
	TOKEN_STAR
	TOKEN_SLASH
	TOKEN_EQUALS
	TOKEN_LT
	TOKEN_GT
	TOKEN_NOT
	TOKEN_AND
	TOKEN_OR
	TOKEN_AMPERSAND
	TOKEN_HASH
	TOKEN_DOLLAR
	TOKEN_DOT
	TOKEN_DOTDOT
	TOKEN_QUESTION
	TOKEN_COLON
	TOKEN_INCLUDE_C
	TOKEN_PERCENT
	TOKEN_BACKTICK
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

type Lexer struct {
	input   string
	pos     int
	line    int
	col     int
	peekPos int
}

func NewLexer(input string) *Lexer {
	return &Lexer{
		input:   input,
		pos:     0,
		line:    1,
		col:     1,
		peekPos: 0,
	}
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return Token{Type: TOKEN_EOF, Literal: "", Line: l.line, Column: l.col}
	}

	ch := l.input[l.pos]

	if ch == '/' && l.peek() == '*' {
		return l.readMultilineComment()
	}

	if ch == '/' && l.peek() == '/' {
		l.readSingleLineComment()
		return l.NextToken()
	}

	if ch == '`' {
		return l.readBackticks()
	}

	if unicode.IsDigit(rune(ch)) || (ch == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1]))) {
		return l.readNumber()
	}

	if ch == '"' {
		return l.readString()
	}

	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.readIdent()
	}

	switch ch {
	case '(':
		return l.makeToken(TOKEN_LPAREN, "(")
	case ')':
		return l.makeToken(TOKEN_RPAREN, ")")
	case '{':
		return l.makeToken(TOKEN_LBRACE, "{")
	case '}':
		return l.makeToken(TOKEN_RBRACE, "}")
	case '[':
		return l.makeToken(TOKEN_LBRACKET, "[")
	case ']':
		return l.makeToken(TOKEN_RBRACKET, "]")
	case ';':
		return l.makeToken(TOKEN_SEMICOLON, ";")
	case ',':
		return l.makeToken(TOKEN_COMMA, ",")
	case '+':
		return l.makeToken(TOKEN_PLUS, "+")
	case '-':
		return l.makeToken(TOKEN_MINUS, "-")
	case '*':
		return l.makeToken(TOKEN_STAR, "*")
	case '/':
		return l.makeToken(TOKEN_SLASH, "/")
	case '=':
		return l.makeToken(TOKEN_EQUALS, "=")
	case '<':
		return l.makeToken(TOKEN_LT, "<")
	case '>':
		return l.makeToken(TOKEN_GT, ">")
	case '?':
		return l.makeToken(TOKEN_QUESTION, "?")
	case ':':
		return l.makeToken(TOKEN_COLON, ":")
	case '!':
		return l.makeToken(TOKEN_NOT, "!")
	case '&':
		if l.peek() == '&' {
			col := l.col
			l.pos += 2
			l.col += 2
			return Token{Type: TOKEN_AND, Literal: "&&", Line: l.line, Column: col}
		}
		return l.makeToken(TOKEN_AMPERSAND, "&")
	case '|':
		if l.peek() == '|' {
			col := l.col
			l.pos += 2
			l.col += 2
			return Token{Type: TOKEN_OR, Literal: "||", Line: l.line, Column: col}
		}
		errors.NewError("0101", "Unknown character '|'", l.line, l.col, "")
		l.pos++
		l.col++
		return l.NextToken()
	case '%':
		return l.makeToken(TOKEN_PERCENT, "%")
	case '#':
		return l.makeToken(TOKEN_HASH, "#")
	case '$':
		return l.makeToken(TOKEN_DOLLAR, "$")
	case '.':
		if l.peek() == '.' {
			col := l.col
			l.pos += 2
			l.col += 2
			return Token{Type: TOKEN_DOTDOT, Literal: "..", Line: l.line, Column: col}
		}
		return l.makeToken(TOKEN_DOT, ".")
	default:
		errors.NewError("0100", "Unknown character", l.line, l.col, "")
		l.pos++
		l.col++
		return l.NextToken()
	}
}

func (l *Lexer) makeToken(tt TokenType, lit string) Token {
	tok := Token{Type: tt, Literal: lit, Line: l.line, Column: l.col}
	l.pos++
	l.col++
	return tok
}

func (l *Lexer) readIdent() Token {
	start := l.pos
	startCol := l.col
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
		l.pos++
		l.col++
	}
	literal := l.input[start:l.pos]
	tokType := TOKEN_IDENT
	keywords := map[string]TokenType{
		"use":      TOKEN_KEYWORD,
		"const":    TOKEN_KEYWORD,
		"func":     TOKEN_KEYWORD,
		"if":       TOKEN_KEYWORD,
		"elsif":    TOKEN_KEYWORD,
		"else":     TOKEN_KEYWORD,
		"case":     TOKEN_KEYWORD,
		"for":      TOKEN_KEYWORD,
		"while":    TOKEN_KEYWORD,
		"type":     TOKEN_KEYWORD,
		"return":   TOKEN_KEYWORD,
		"void":     TOKEN_KEYWORD,
		"int":      TOKEN_KEYWORD,
		"char":     TOKEN_KEYWORD,
		"string":   TOKEN_KEYWORD,
		"arr":      TOKEN_KEYWORD,
		"dict":     TOKEN_KEYWORD,
		"float":    TOKEN_KEYWORD,
		"double":   TOKEN_KEYWORD,
		"bool":     TOKEN_KEYWORD,
		"any":      TOKEN_KEYWORD,
		"true":     TOKEN_KEYWORD,
		"false":    TOKEN_KEYWORD,
		"T":        TOKEN_KEYWORD,
		"includeC": TOKEN_INCLUDE_C,
	}
	if kwType, ok := keywords[literal]; ok {
		tokType = kwType
	}
	return Token{Type: tokType, Literal: literal, Line: l.line, Column: startCol}
}

func (l *Lexer) readNumber() Token {
	start := l.pos
	startCol := l.col
	if l.input[l.pos] == '-' {
		l.pos++
		l.col++
	}
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.pos++
		l.col++
	}
	if l.pos+1 < len(l.input) && l.input[l.pos] == '.' && unicode.IsDigit(rune(l.input[l.pos+1])) {
		l.pos++
		l.col++
		for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
			l.pos++
			l.col++
		}
	}
	literal := l.input[start:l.pos]
	return Token{Type: TOKEN_NUMBER, Literal: literal, Line: l.line, Column: startCol}
}

func (l *Lexer) readString() Token {
	startCol := l.col
	l.pos++ // пропустить "
	l.col++
	var result strings.Builder

	for l.pos < len(l.input) && l.input[l.pos] != '"' {
		if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
			result.WriteByte('\\')
			l.pos++
			l.col++
			result.WriteByte(l.input[l.pos])
			l.pos++
			l.col++
		} else {
			result.WriteByte(l.input[l.pos])
			l.pos++
			l.col++
		}
	}

	if l.pos >= len(l.input) {
		errors.NewError("0102", "Unterminated string", l.line, l.col, "")
		return Token{Type: TOKEN_STRING, Literal: "", Line: l.line, Column: startCol}
	}

	l.pos++ // пропустить "
	l.col++
	return Token{Type: TOKEN_STRING, Literal: result.String(), Line: l.line, Column: startCol}
}

func (l *Lexer) readSingleLineComment() {
	for l.pos < len(l.input) && l.input[l.pos] != '\n' {
		l.pos++
	}
}

func (l *Lexer) readMultilineComment() Token {
	l.pos += 2
	l.col += 2
	for l.pos < len(l.input)-1 && !(l.input[l.pos] == '*' && l.input[l.pos+1] == '/') {
		if l.input[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
	if l.pos >= len(l.input)-1 {
		errors.NewError("0104", "Unterminated multi-line comment", l.line, l.col, "")
		return Token{Type: TOKEN_EOF, Literal: "", Line: l.line, Column: l.col}
	}
	l.pos += 2
	l.col += 2
	return l.NextToken()
}

func (l *Lexer) readBackticks() Token {
	startCol := l.col
	l.pos++ // первый `
	l.col++

	if l.pos < len(l.input) && l.input[l.pos] == '`' {
		l.pos++
		l.col++
	}
	if l.pos < len(l.input) && l.input[l.pos] == '`' {
		l.pos++
		l.col++
	}

	var code strings.Builder
	for l.pos < len(l.input) {
		if l.pos+2 < len(l.input) && l.input[l.pos] == '`' && l.input[l.pos+1] == '`' && l.input[l.pos+2] == '`' {
			l.pos += 3
			l.col += 3
			return Token{
				Type:    TOKEN_BACKTICK,
				Literal: strings.TrimSpace(code.String()),
				Line:    l.line,
				Column:  startCol,
			}
		}
		if l.input[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		code.WriteByte(l.input[l.pos])
		l.pos++
	}

	errors.NewError("0103", "Unterminated backticks (```)", l.line, l.col, "")
	return Token{Type: TOKEN_BACKTICK, Literal: "", Line: l.line, Column: startCol}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.pos++
			l.col++
		} else if ch == '\n' {
			l.pos++
			l.line++
			l.col = 1
		} else {
			break
		}
	}
}

func (l *Lexer) peek() byte {
	if l.pos+1 < len(l.input) {
		return l.input[l.pos+1]
	}
	return 0
}

func (p *Param) HasDefault() bool {
	return p.DefaultValue != nil
}

func (tt TokenType) String() string {
	switch tt {
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_IDENT:
		return "IDENT"
	case TOKEN_NUMBER:
		return "NUMBER"
	case TOKEN_STRING:
		return "STRING"
	case TOKEN_KEYWORD:
		return "KEYWORD"
	case TOKEN_LPAREN:
		return "("
	case TOKEN_RPAREN:
		return ")"
	case TOKEN_LBRACE:
		return "{"
	case TOKEN_RBRACE:
		return "}"
	case TOKEN_LBRACKET:
		return "["
	case TOKEN_RBRACKET:
		return "]"
	case TOKEN_SEMICOLON:
		return ";"
	case TOKEN_COMMA:
		return ","
	case TOKEN_PLUS:
		return "+"
	case TOKEN_MINUS:
		return "-"
	case TOKEN_STAR:
		return "*"
	case TOKEN_SLASH:
		return "/"
	case TOKEN_EQUALS:
		return "="
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_NOT:
		return "!"
	case TOKEN_AMPERSAND:
		return "&"
	case TOKEN_HASH:
		return "#"
	case TOKEN_DOLLAR:
		return "$"
	case TOKEN_DOT:
		return "."
	case TOKEN_DOTDOT:
		return ".."
	case TOKEN_AND:
		return "&&"
	case TOKEN_OR:
		return "||"
	case TOKEN_QUESTION:
		return "?"
	case TOKEN_COLON:
		return ":"
	case TOKEN_PERCENT:
		return "%"
	case TOKEN_INCLUDE_C:
		return "includeC"
	default:
		return "UNKNOWN"
	}
}
