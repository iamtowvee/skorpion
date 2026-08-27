package errors

import (
	"fmt"
)

// ErrorSystem - система ошибок
type ErrorSystem struct {
	errors []SkorpionError
	active bool
}

var system *ErrorSystem

// SkorpionError - структура ошибки
type SkorpionError struct {
	Code    string // Err+1054
	Message string
	Line    int
	Column  int
	File    string
}

// InitErrors инициализирует систему ошибок
func InitErrors() {
	system = &ErrorSystem{
		errors: make([]SkorpionError, 0),
		active: true,
	}
}

// CloseErrors закрывает систему ошибок
func CloseErrors() {
	if system != nil && len(system.errors) > 0 {
		fmt.Println("\n❌ Обнаружены ошибки:")
		for _, err := range system.errors {
			fmt.Printf("  %s: %s\n", err.Code, err.Message)
			if err.File != "" {
				fmt.Printf("    в файле: %s", err.File)
				if err.Line > 0 {
					fmt.Printf(" строка %d", err.Line)
					if err.Column > 0 {
						fmt.Printf(" колонка %d", err.Column)
					}
				}
				fmt.Println()
			}
		}
	}
	system = nil
}

// NewError создает новую ошибку
func NewError(shortCode string, message string) {
	if system == nil {
		return
	}

	err := SkorpionError{
		Code:    "Err+" + shortCode,
		Message: message,
	}

	system.errors = append(system.errors, err)
}

// NewErrorWithPosition создает ошибку с позицией
func NewErrorWithPosition(shortCode string, message string, line, col int, file string) {
	if system == nil {
		return
	}

	err := SkorpionError{
		Code:    "Err+" + shortCode,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
	}

	system.errors = append(system.errors, err)
}

// TakeErrorsList возвращает список ошибок
func TakeErrorsList() []SkorpionError {
	if system == nil {
		return nil
	}
	return system.errors
}

// HasErrors проверяет наличие ошибок
func HasErrors() bool {
	return system != nil && len(system.errors) > 0
}
