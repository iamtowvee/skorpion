package errors

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Severity — уровень сообщения. Используется логом для префикса
// в секции ERRORS: Fatal / Err / Warn / Info.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
	SeverityFatal
)

var (
	mu       sync.Mutex
	errors   []SkorpionError
	warnings []SkorpionError
	all      []SkorpionError // errors + warnings в порядке поступления
	hasFatal bool
)

type SkorpionError struct {
	Code      string
	Message   string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	File      string
	Fatal     bool
	Warning   bool
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

// Severity возвращает уровень сообщения для лога.
func (e *SkorpionError) Severity() Severity {
	switch {
	case e.Fatal:
		return SeverityFatal
	case e.Warning:
		return SeverityWarn
	case e.Code == "":
		return SeverityInfo
	default:
		return SeverityError
	}
}

// SeverityString — "Fatal" / "Err" / "Warn" / "Info".
func (e *SkorpionError) SeverityString() string {
	switch e.Severity() {
	case SeverityFatal:
		return "Fatal"
	case SeverityWarn:
		return "Warn"
	case SeverityInfo:
		return "Info"
	default:
		return "Err"
	}
}

func InitErrors() {
	mu.Lock()
	defer mu.Unlock()
	errors = make([]SkorpionError, 0)
	warnings = make([]SkorpionError, 0)
	all = make([]SkorpionError, 0)
	hasFatal = false
}

func NewError(code, message string, line, col int, file string) {
	mu.Lock()
	defer mu.Unlock()
	e := SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
	}
	errors = append(errors, e)
	all = append(all, e)
}

func NewFatalError(code, message string, line, col int, file string) {
	mu.Lock()
	defer mu.Unlock()
	e := SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
		Fatal:   true,
	}
	errors = append(errors, e)
	all = append(all, e)
	hasFatal = true
}

func NewWarning(code, message string, line, col int, file string) {
	mu.Lock()
	defer mu.Unlock()
	e := SkorpionError{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  col,
		File:    file,
		Warning: true,
	}
	warnings = append(warnings, e)
	all = append(all, e)
}

func NewErrorSpan(code, message string, line, col, endLine, endCol int, file string) {
	mu.Lock()
	defer mu.Unlock()
	e := SkorpionError{
		Code:      code,
		Message:   message,
		Line:      line,
		Column:    col,
		EndLine:   endLine,
		EndColumn: endCol,
		File:      file,
	}
	errors = append(errors, e)
	all = append(all, e)
}

func NewWarningSpan(code, message string, line, col, endLine, endCol int, file string) {
	mu.Lock()
	defer mu.Unlock()
	e := SkorpionError{
		Code:      code,
		Message:   message,
		Line:      line,
		Column:    col,
		EndLine:   endLine,
		EndColumn: endCol,
		File:      file,
		Warning:   true,
	}
	warnings = append(warnings, e)
	all = append(all, e)
}

func TakeErrorsList() []SkorpionError {
	mu.Lock()
	defer mu.Unlock()
	return append([]SkorpionError{}, errors...)
}

func TakeWarningsList() []SkorpionError {
	mu.Lock()
	defer mu.Unlock()
	return append([]SkorpionError{}, warnings...)
}

// Count возвращает количество всех сообщений (errors + warnings).
func Count() int {
	mu.Lock()
	defer mu.Unlock()
	return len(all)
}

func CloseErrors() {
	mu.Lock()
	defer mu.Unlock()
	errors = nil
	warnings = nil
	all = nil
	hasFatal = false
}

func HasErrors() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(errors) > 0
}

func HasWarnings() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(warnings) > 0
}

func HasFatal() bool {
	mu.Lock()
	defer mu.Unlock()
	return hasFatal
}

// WriteAllTo пишет все накопленные сообщения в w (без цветов,
// без cli-зависимостей). Используется логом для секции ERRORS.
func WriteAllTo(w io.Writer) {
	mu.Lock()
	snapshot := append([]SkorpionError{}, all...)
	mu.Unlock()

	if len(snapshot) == 0 {
		return
	}

	for _, e := range snapshot {
		info := e.GetCodeInfo()

		fmt.Fprintf(w, "[%s+%s] %s\n", e.SeverityString(), e.Code, e.Message)

		file := e.File
		if file == "" {
			file = "<unknown>"
		}
		if e.Line > 0 || e.Column > 0 {
			fmt.Fprintf(w, "  >>> %s | L: %d, C: %d\n", file, e.Line, e.Column)
		} else {
			fmt.Fprintf(w, "  >>> %s\n", file)
		}

		if info.Tip != "" {
			fmt.Fprintf(w, "   >>> TIP\n")
			for _, line := range strings.Split(info.Tip, "\n") {
				fmt.Fprintf(w, "    | %s\n", line)
			}
		}
		fmt.Fprintln(w)
	}
}

type ErrorReport struct {
	Errors     []SkorpionError
	Warnings   []SkorpionError
	FilePath   string
	SourceCode []string
	TotalTime  time.Duration
}
