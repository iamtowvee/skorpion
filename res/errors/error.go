package errors

import (
	"fmt"
	"skrp/res/cli"
	"sync"
	"time"
)

var (
	mu       sync.Mutex
	errors   []SkorpionError
	hasFatal bool
)

type SkorpionError struct {
	Code    string
	Message string
	Line    int
	Column  int
	File    string
	Fatal   bool
}

func (e *SkorpionError) GetCodeInfo() ErrorCode {
	if info, ok := ErrorCodes[e.Code]; ok {
		return info
	}
	return ErrorCode{
		Code:        e.Code,
		Message:     e.Message,
		Description: "Unknown error",
		Tip:         "Run 'skorpion --explain " + e.Code + "' for more information.",
	}
}

func InitErrors() {
	mu.Lock()
	defer mu.Unlock()
	errors = make([]SkorpionError, 0)
	hasFatal = false
}

func NewError(code, message string, line, col int, file string) {
	mu.Lock()
	defer mu.Unlock()
	errors = append(errors, SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
		Fatal:   false,
	})
}

func NewFatalError(code, message string, line, col int, file string) {
	mu.Lock()
	defer mu.Unlock()
	errors = append(errors, SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
		Fatal:   true,
	})
	hasFatal = true
}

func TakeErrorsList() []SkorpionError {
	mu.Lock()
	defer mu.Unlock()
	return append([]SkorpionError{}, errors...)
}

func CloseErrors() {
	mu.Lock()
	defer mu.Unlock()
	errors = nil
	hasFatal = false
}

func HasErrors() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(errors) > 0
}

func HasFatal() bool {
	mu.Lock()
	defer mu.Unlock()
	return hasFatal
}

func ClearErrors() {
	mu.Lock()
	defer mu.Unlock()
	errors = make([]SkorpionError, 0)
	hasFatal = false
}

func PrintErrors() {
	for _, e := range TakeErrorsList() {
		code := cli.Colors.Red("Err+" + e.Code)
		message := cli.Colors.Red(e.Message)
		location := cli.Colors.Yellow(e.File + ":" + fmt.Sprintf("%d:%d", e.Line, e.Column))
		if e.Fatal {
			code = cli.Colors.Bold(cli.Colors.Red("FATAL: " + e.Code))
		}
		fmt.Printf("%s: %s (at %s)\n", code, message, location)
	}
}

// ErrorReport для красивого вывода ошибок
type ErrorReport struct {
	Errors     []SkorpionError
	FilePath   string
	SourceCode []string
	TotalTime  time.Duration
}
