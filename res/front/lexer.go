package front

import (
	"fmt"
	"skrp/res/errors"
)

// TokenType тип токена
type TokenType string

const (
	// Ключевые слова
	TOKEN_USE       TokenType = "USE"
	TOKEN_MODULE    TokenType = "MODULE"
	TOKEN_ALIAS     TokenType = "ALIAS"
	TOKEN_INCLUDE_C TokenType = "INCLUDE_C"

	// Типы
	TOKEN_TYPE_VOID   TokenType = "VOID"
	TOKEN_TYPE_INT    TokenType = "INT"
	TOKEN_TYPE_CHAR   TokenType = "CHAR"
	TOKEN_TYPE_STRING TokenType = "STRING"
	TOKEN_TYPE_ARR    TokenType = "ARR"
	TOKEN_TYPE_DICT   TokenType = "DICT"
	TOKEN_TYPE_FLOAT  TokenType = "FLOAT"
	TOKEN_TYPE_DOUBLE TokenType = "DOUBLE"
	TOKEN_TYPE_BOOL   TokenType = "BOOL"
	TOKEN_TYPE_ANY    TokenType = "ANY"
	TOKEN_TYPE_T      TokenType = "T"

	// Управляющие конструкции
	TOKEN_IF     TokenType = "IF"
	TOKEN_ELSE   TokenType = "ELSE"
	TOKEN_WHILE  TokenType = "WHILE"
	TOKEN_FOR    TokenType = "FOR"
	TOKEN_RETURN TokenType = "RETURN"

	// Значения
	TOKEN_TRUE  TokenType = "TRUE"
	TOKEN_FALSE TokenType = "FALSE"

	// Идентификаторы и литералы
	TOKEN_IDENT  TokenType = "IDENT"
	TOKEN_NUMBER TokenType = "NUMBER"
	TOKEN_STRING TokenType = "STRING_LITERAL"
	TOKEN_CHAR   TokenType = "CHAR_LITERAL"

	// Разделители
	TOKEN_LBRACE   TokenType = "{"
	TOKEN_RBRACE   TokenType = "}"
	TOKEN_LPAREN   TokenType = "("
	TOKEN_RPAREN   TokenType = ")"
	TOKEN_LBRACKET TokenType = "["
	TOKEN_RBRACKET TokenType = "]"
	TOKEN_LANGLE   TokenType = "<"
	TOKEN_RANGLE   TokenType = ">"

	// Операторы
	TOKEN_ASSIGN   TokenType = "="
	TOKEN_PLUS     TokenType = "+"
	TOKEN_MINUS    TokenType = "-"
	TOKEN_MUL      TokenType = "*"
	TOKEN_DIV      TokenType = "/"
	TOKEN_MOD      TokenType = "%"
	TOKEN_EQ       TokenType = "=="
	TOKEN_NEQ      TokenType = "!="
	TOKEN_LT       TokenType = "<"
	TOKEN_GT       TokenType = ">"
	TOKEN_LE       TokenType = "<="
	TOKEN_GE       TokenType = ">="
	TOKEN_AND      TokenType = "&&"
	TOKEN_OR       TokenType = "||"
	TOKEN_NOT      TokenType = "!"
	TOKEN_INC      TokenType = "++"
	TOKEN_DEC      TokenType = "--"
	TOKEN_PLUS_EQ  TokenType = "+="
	TOKEN_MINUS_EQ TokenType = "-="
	TOKEN_MUL_EQ   TokenType = "*="
	TOKEN_DIV_EQ   TokenType = "/="

	// Специальные
	TOKEN_SEMICOLON TokenType = ";"
	TOKEN_COMMA     TokenType = ","
	TOKEN_DOT       TokenType = "."
	TOKEN_COLON     TokenType = ":"
	TOKEN_AMP       TokenType = "&"
	TOKEN_DOLLAR    TokenType = "$"
	TOKEN_ARROW     TokenType = "->"

	TOKEN_COMMENT TokenType = "COMMENT"
	TOKEN_EOF     TokenType = "EOF"
)

// Token структура токена
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// Lexer структура лексера
type Lexer struct {
	input  string
	pos    int
	line   int
	col    int
	ch     rune
	tokens []Token
}

// NewLexer создает новый лексер
func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		col:    0,
		tokens: make([]Token, 0),
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.pos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = rune(l.input[l.pos])
	}
	l.pos++
	if l.ch == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
}

func (l *Lexer) peekChar() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return rune(l.input[l.pos])
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// Tokenize разбивает входной код на токены
func (l *Lexer) Tokenize() ([]Token, error) {
	for l.ch != 0 {
		switch {
		case isWhitespace(l.ch):
			l.skipWhitespace()
		case l.ch == '/':
			l.handleComment()
		case isLetter(l.ch):
			l.handleIdentifier()
		case isDigit(l.ch):
			l.handleNumber()
		case l.ch == '"':
			l.handleString()
		case l.ch == '\'':
			l.handleChar()
		default:
			l.handleOperator()
		}
	}

	l.tokens = append(l.tokens, Token{TOKEN_EOF, "EOF", l.line, l.col})
	return l.tokens, nil
}

func (l *Lexer) handleComment() {
	l.readChar()
	if l.ch == '/' {
		// Однострочный комментарий
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	} else if l.ch == '*' {
		// Многострочный комментарий
		l.readChar()
		for !(l.ch == '*' && l.peekChar() == '/') && l.ch != 0 {
			l.readChar()
		}
		if l.ch == '*' {
			l.readChar()
			l.readChar() // пропускаем '/'
		}
	} else {
		// Это оператор деления
		l.tokens = append(l.tokens, Token{TOKEN_DIV, "/", l.line, l.col})
	}
}

func (l *Lexer) handleIdentifier() {
	startLine := l.line
	startCol := l.col
	var ident string

	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		ident += string(l.ch)
		l.readChar()
	}

	// Проверяем ключевые слова
	tokenType := l.lookupKeyword(ident)
	l.tokens = append(l.tokens, Token{tokenType, ident, startLine, startCol})
}

func (l *Lexer) lookupKeyword(ident string) TokenType {
	keywords := map[string]TokenType{
		"use":      TOKEN_USE,
		"module":   TOKEN_MODULE,
		"alias":    TOKEN_ALIAS,
		"includeC": TOKEN_INCLUDE_C,
		"void":     TOKEN_TYPE_VOID,
		"int":      TOKEN_TYPE_INT,
		"char":     TOKEN_TYPE_CHAR,
		"string":   TOKEN_TYPE_STRING,
		"arr":      TOKEN_TYPE_ARR,
		"dict":     TOKEN_TYPE_DICT,
		"float":    TOKEN_TYPE_FLOAT,
		"double":   TOKEN_TYPE_DOUBLE,
		"bool":     TOKEN_TYPE_BOOL,
		"any":      TOKEN_TYPE_ANY,
		"T":        TOKEN_TYPE_T,
		"if":       TOKEN_IF,
		"else":     TOKEN_ELSE,
		"while":    TOKEN_WHILE,
		"for":      TOKEN_FOR,
		"return":   TOKEN_RETURN,
		"true":     TOKEN_TRUE,
		"false":    TOKEN_FALSE,
	}

	if token, ok := keywords[ident]; ok {
		return token
	}
	return TOKEN_IDENT
}

func (l *Lexer) handleNumber() {
	startLine := l.line
	startCol := l.col
	num := ""
	isFloat := false

	for isDigit(l.ch) || l.ch == '.' {
		if l.ch == '.' {
			if isFloat {
				errors.NewErrorWithPosition(
					errors.ERR_INVALID_NUMBER,
					"Некорректное число: несколько точек",
					startLine, startCol, "",
				)
				return
			}
			isFloat = true
		}
		num += string(l.ch)
		l.readChar()
	}

	l.tokens = append(l.tokens, Token{TOKEN_NUMBER, num, startLine, startCol})
}

func (l *Lexer) handleString() {
	startLine := l.line
	startCol := l.col
	l.readChar() // пропускаем открывающую кавычку

	var str string
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				str += "\n"
			case 't':
				str += "\t"
			case '"':
				str += "\""
			case '\\':
				str += "\\"
			default:
				str += string(l.ch)
			}
		} else {
			str += string(l.ch)
		}
		l.readChar()
	}

	if l.ch == 0 {
		errors.NewErrorWithPosition(
			errors.ERR_UNTERMINATED_STRING,
			"Незакрытая строка",
			startLine, startCol, "",
		)
		return
	}

	l.readChar() // пропускаем закрывающую кавычку
	l.tokens = append(l.tokens, Token{TOKEN_STRING, str, startLine, startCol})
}

func (l *Lexer) handleChar() {
	startLine := l.line
	startCol := l.col
	l.readChar() // пропускаем открывающую кавычку

	if l.ch == '\'' {
		errors.NewErrorWithPosition(
			errors.ERR_INVALID_SYNTAX,
			"Пустой символ",
			startLine, startCol, "",
		)
		return
	}

	char := string(l.ch)
	l.readChar()

	if l.ch != '\'' {
		errors.NewErrorWithPosition(
			errors.ERR_INVALID_SYNTAX,
			"Некорректный символ",
			startLine, startCol, "",
		)
		return
	}

	l.readChar()
	l.tokens = append(l.tokens, Token{TOKEN_CHAR, char, startLine, startCol})
}

func (l *Lexer) handleOperator() {
	startLine := l.line
	startCol := l.col

	switch l.ch {
	case '{':
		l.tokens = append(l.tokens, Token{TOKEN_LBRACE, "{", startLine, startCol})
		l.readChar()
	case '}':
		l.tokens = append(l.tokens, Token{TOKEN_RBRACE, "}", startLine, startCol})
		l.readChar()
	case '(':
		l.tokens = append(l.tokens, Token{TOKEN_LPAREN, "(", startLine, startCol})
		l.readChar()
	case ')':
		l.tokens = append(l.tokens, Token{TOKEN_RPAREN, ")", startLine, startCol})
		l.readChar()
	case '[':
		l.tokens = append(l.tokens, Token{TOKEN_LBRACKET, "[", startLine, startCol})
		l.readChar()
	case ']':
		l.tokens = append(l.tokens, Token{TOKEN_RBRACKET, "]", startLine, startCol})
		l.readChar()
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_LE, "<=", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_LANGLE, "<", startLine, startCol})
		}
		l.readChar()
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_GE, ">=", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_RANGLE, ">", startLine, startCol})
		}
		l.readChar()
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_EQ, "==", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_ASSIGN, "=", startLine, startCol})
		}
		l.readChar()
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_NEQ, "!=", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_NOT, "!", startLine, startCol})
		}
		l.readChar()
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_PLUS_EQ, "+=", startLine, startCol})
		} else if l.peekChar() == '+' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_INC, "++", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_PLUS, "+", startLine, startCol})
		}
		l.readChar()
	case '-':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_MINUS_EQ, "-=", startLine, startCol})
		} else if l.peekChar() == '-' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_DEC, "--", startLine, startCol})
		} else if l.peekChar() == '>' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_ARROW, "->", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_MINUS, "-", startLine, startCol})
		}
		l.readChar()
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_MUL_EQ, "*=", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_MUL, "*", startLine, startCol})
		}
		l.readChar()
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_DIV_EQ, "/=", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_DIV, "/", startLine, startCol})
		}
		l.readChar()
	case '%':
		l.tokens = append(l.tokens, Token{TOKEN_MOD, "%", startLine, startCol})
		l.readChar()
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_AND, "&&", startLine, startCol})
		} else {
			l.tokens = append(l.tokens, Token{TOKEN_AMP, "&", startLine, startCol})
		}
		l.readChar()
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			l.tokens = append(l.tokens, Token{TOKEN_OR, "||", startLine, startCol})
		}
		l.readChar()
	case ';':
		l.tokens = append(l.tokens, Token{TOKEN_SEMICOLON, ";", startLine, startCol})
		l.readChar()
	case ',':
		l.tokens = append(l.tokens, Token{TOKEN_COMMA, ",", startLine, startCol})
		l.readChar()
	case '.':
		l.tokens = append(l.tokens, Token{TOKEN_DOT, ".", startLine, startCol})
		l.readChar()
	case ':':
		l.tokens = append(l.tokens, Token{TOKEN_COLON, ":", startLine, startCol})
		l.readChar()
	case '$':
		l.tokens = append(l.tokens, Token{TOKEN_DOLLAR, "$", startLine, startCol})
		l.readChar()
	default:
		errors.NewErrorWithPosition(
			errors.ERR_UNEXPECTED_CHAR,
			fmt.Sprintf("Неожиданный символ: '%c'", l.ch),
			startLine, startCol, "",
		)
		l.readChar()
	}
}

func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}
