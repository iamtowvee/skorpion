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
	TOKEN_AMPERSAND
	TOKEN_HASH
	TOKEN_DOLLAR
	TOKEN_DOT
	TOKEN_INCLUDE_C
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

// Единственный конструктор
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

	// Многострочные комментарии
	if ch == '/' && l.peek() == '*' {
		return l.readMultilineComment()
	}

	// Однострочные комментарии
	if ch == '/' && l.peek() == '/' {
		l.readSingleLineComment()
		return l.NextToken()
	}

	// Бэктики для includeC — ДО ВСЕГО!
	if ch == '`' {
		return l.readBackticks()
	}

	// Числа
	if unicode.IsDigit(rune(ch)) || (ch == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1]))) {
		return l.readNumber()
	}

	// Строки
	if ch == '"' {
		return l.readString()
	}

	// Идентификаторы и ключевые слова
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.readIdent()
	}

	// Остальные символы...
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
	case '!':
		return l.makeToken(TOKEN_NOT, "!")
	case '&':
		return l.makeToken(TOKEN_AMPERSAND, "&")
	case '#':
		return l.makeToken(TOKEN_HASH, "#")
	case '$':
		return l.makeToken(TOKEN_DOLLAR, "$")
	case '.':
		return l.makeToken(TOKEN_DOT, ".")
	default:
		errors.NewError("0001", "Unknown character", l.line, l.col, "")
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
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
		l.pos++
	}
	literal := l.input[start:l.pos]
	tokType := TOKEN_IDENT
	// Ключевые слова
	keywords := map[string]TokenType{
		"use":      TOKEN_KEYWORD,
		"const":    TOKEN_KEYWORD,
		"func":     TOKEN_KEYWORD,
		"if":       TOKEN_KEYWORD,
		"elsif":    TOKEN_KEYWORD,
		"else":     TOKEN_KEYWORD,
		"for":      TOKEN_KEYWORD,
		"while":    TOKEN_KEYWORD,
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
	return Token{Type: tokType, Literal: literal, Line: l.line, Column: start - l.col + 1}
}

func (l *Lexer) readNumber() Token {
	start := l.pos
	if l.input[l.pos] == '-' {
		l.pos++
	}
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.pos++
	}
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		l.pos++
		for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
			l.pos++
		}
	}
	literal := l.input[start:l.pos]
	return Token{Type: TOKEN_NUMBER, Literal: literal, Line: l.line, Column: start - l.col + 1}
}

func (l *Lexer) readString() Token {
	start := l.pos
	l.pos++ // пропустить "
	var result strings.Builder

	for l.pos < len(l.input) && l.input[l.pos] != '"' {
		if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
			// Сохраняем экранирование как есть для includeC
			result.WriteByte('\\')
			l.pos++
			result.WriteByte(l.input[l.pos])
			l.pos++
		} else {
			result.WriteByte(l.input[l.pos])
			l.pos++
		}
	}

	if l.pos >= len(l.input) {
		errors.NewError("0002", "Unterminated string", l.line, l.col, "")
		return Token{Type: TOKEN_STRING, Literal: "", Line: l.line, Column: l.col}
	}

	l.pos++ // пропустить "
	return Token{Type: TOKEN_STRING, Literal: result.String(), Line: l.line, Column: start - l.col + 1}
}

func (l *Lexer) readSingleLineComment() {
	for l.pos < len(l.input) && l.input[l.pos] != '\n' {
		l.pos++
	}
}

func (l *Lexer) readMultilineComment() Token {
	l.pos += 2
	for l.pos < len(l.input)-1 && !(l.input[l.pos] == '*' && l.input[l.pos+1] == '/') {
		if l.input[l.pos] == '\n' {
			l.line++
			l.col = 1
		}
		l.pos++
	}
	if l.pos >= len(l.input)-1 {
		errors.NewError("0003", "Unterminated multi-line comment", l.line, l.col, "")
		return Token{Type: TOKEN_EOF, Literal: "", Line: l.line, Column: l.col}
	}
	l.pos += 2
	return l.NextToken()
}

func (l *Lexer) readBackticks() Token {
	start := l.pos
	l.pos++ // пропускаем первый `

	// Пропускаем следующие два ` (всего ```)
	if l.pos < len(l.input) && l.input[l.pos] == '`' {
		l.pos++
	}
	if l.pos < len(l.input) && l.input[l.pos] == '`' {
		l.pos++
	}

	// Собираем код до закрывающих ```
	var code strings.Builder
	for l.pos < len(l.input) {
		// Проверяем, не встретили ли закрывающие ```
		if l.pos+2 < len(l.input) && l.input[l.pos] == '`' && l.input[l.pos+1] == '`' && l.input[l.pos+2] == '`' {
			// Нашли закрывающие ```, выходим
			l.pos += 3 // пропускаем ```
			return Token{
				Type:    TOKEN_BACKTICK,
				Literal: strings.TrimSpace(code.String()),
				Line:    l.line,
				Column:  start - l.col + 1,
			}
		}
		if l.input[l.pos] == '\n' {
			l.line++
		}
		code.WriteByte(l.input[l.pos])
		l.pos++
	}

	errors.NewError("0002", "Unterminated backticks (```)", l.line, l.col, "")
	return Token{Type: TOKEN_BACKTICK, Literal: "", Line: l.line, Column: l.col}
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

// String возвращает строковое представление TokenType
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
	case TOKEN_INCLUDE_C:
		return "includeC"
	default:
		return "UNKNOWN"
	}
}
