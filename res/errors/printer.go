package errors

import (
	"fmt"
	"os"
	"skrp/res/cli"
	"skrp/res/stdlib"
	"strings"
	"time"
)

// ============================================================================
// Source resolver — единый способ получить строки файла по err.File.
//
// По умолчанию:
//   - builtin:<...>  → stdlib.GetModule
//   - иначе          → os.ReadFile
//
// skorpion.go может переопределить (например, чтобы отдать уже прочитанный
// main.sk без повторного I/O).
// ============================================================================

var sourceResolver func(path string) []string

// SetSourceResolver — переопределить резолвер (nil = сбросить на дефолт).
func SetSourceResolver(fn func(path string) []string) {
	sourceResolver = fn
}

// resolveSource возвращает строки файла для err.File.
//
// Приоритет:
//  1. Если задан sourceResolver — его результат (если не nil).
//  2. builtin:<...>  → stdlib.GetModule.
//  3. иначе          → os.ReadFile.
//
// Возвращает nil, если файл не удалось получить.
func resolveSource(path string) []string {
	if path == "" {
		return nil
	}

	if sourceResolver != nil {
		if lines := sourceResolver(path); lines != nil {
			return lines
		}
	}

	if strings.HasPrefix(path, "<builtin:") && strings.HasSuffix(path, ">") {
		modulePath := strings.TrimSuffix(strings.TrimPrefix(path, "<builtin:"), ">")
		if content, ok := stdlib.GetModule(modulePath); ok {
			return strings.Split(content, "\n")
		}
		return nil
	}

	if strings.HasPrefix(path, "builtin:") {
		modulePath := strings.TrimPrefix(path, "builtin:")
		if content, ok := stdlib.GetModule(modulePath); ok {
			return strings.Split(content, "\n")
		}
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}

// ============================================================================
// printTip — блок TIP.
// ============================================================================

func printTip(codeInfo ErrorCode, width int) {
	if codeInfo.Tip == "" {
		return
	}
	tipArrowPrefix := strings.Repeat(" ", width+1) + ">>> TIP"
	fmt.Printf("%s\n", cli.Colors.Colorize(cli.BRIGHT_YELLOW, tipArrowPrefix))

	pipePrefix := strings.Repeat(" ", width+2) + "|"
	fmt.Printf("%s %s\n", cli.Colors.Dim(pipePrefix), codeInfo.Tip)
	fmt.Printf("%s\n", cli.Colors.Dim(pipePrefix))
}

// ============================================================================
// printDiag — единая отрисовка Error и Warning.
//
// sourceLines — строки файла, к которому относится ошибка (или nil).
// Резолвится вызывающим кодом (PrintErrorReport) через resolveSource.
// ============================================================================

func printDiag(d SkorpionError, sourceLines []string, isWarn bool) {
	codeInfo := d.GetCodeInfo()

	colorFn := cli.Colors.Red
	label := "Err+" + d.Code
	colonColor := cli.Colors.Red
	if isWarn {
		colorFn = cli.Colors.Yellow
		label = "Warn+" + d.Code
		colonColor = cli.Colors.Yellow
	}

	width := 1
	hasContext := len(sourceLines) > 0 && d.Line > 0 && d.Line <= len(sourceLines)
	var ctxStart, ctxEnd int
	if hasContext {
		ctxStart = d.Line - 3
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxEnd = d.Line + 2
		if ctxEnd > len(sourceLines) {
			ctxEnd = len(sourceLines)
		}
		width = len(fmt.Sprintf("%d", ctxEnd))
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05")
	fmt.Printf("%s %s%s%s %s\n",
		cli.Colors.Dim("["+timestamp+" >"),
		colorFn(label),
		cli.Colors.Dim("]"),
		colonColor(":"),
		cli.Colors.Bold(d.Message))

	filePath := d.File
	if filePath == "" {
		filePath = "<unknown>"
	}

	arrowPrefix := strings.Repeat(" ", width+1) + ">>>"
	fmt.Printf("%s %s %s %s %d, %s %d\n",
		cli.Colors.Dim(arrowPrefix),
		cli.Colors.Cyan(filePath),
		cli.Colors.Dim("|"),
		cli.Colors.Dim("L:"),
		d.Line,
		cli.Colors.Dim("C:"),
		d.Column)

	pipePrefix := strings.Repeat(" ", width+2) + "|"

	if hasContext {
		fmt.Printf("%s\n", cli.Colors.Dim(pipePrefix))

		for i := ctxStart; i < ctxEnd; i++ {
			lineNum := i + 1
			line := sourceLines[i]
			lineNumStr := fmt.Sprintf("%*d", width, lineNum)

			if lineNum == d.Line {
				fmt.Printf(" %s %s %s\n",
					cli.Colors.Bold(lineNumStr),
					cli.Colors.Dim("|"),
					line)

				if d.Column > 0 {
					startCol := d.Column
					endCol := d.EndColumn
					if endCol <= startCol {
						endCol = startCol + 1
					}
					pointerIndent := width + 4 + (startCol - 1)
					pointer := strings.Repeat(" ", pointerIndent) + strings.Repeat("~", endCol-startCol)
					fmt.Printf("%s\n", colorFn(pointer))
				}
			} else {
				fmt.Printf(" %s %s %s\n",
					cli.Colors.Dim(lineNumStr),
					cli.Colors.Dim("|"),
					line)
			}
		}

		fmt.Printf("%s\n", cli.Colors.Dim(pipePrefix))
	}

	printTip(codeInfo, width)
}

func PrintError(err SkorpionError, sourceLines []string) {
	printDiag(err, sourceLines, false)
}

func PrintWarning(warn SkorpionError, sourceLines []string) {
	printDiag(warn, sourceLines, true)
}

// ============================================================================
// PrintErrorReport — группирует сообщения по err.File и рисует
// каждое с контекстом СВОЕГО файла.
//
// report.FilePath и report.SourceCode используются как «главный» файл
// (main.sk) и fallback — если resolveSource(err.File) вернул nil,
// берём report.SourceCode, но только когда err.File == report.FilePath
// ИЛИ report.SourceCode явно просили использовать всегда.
//
// Правильное поведение: контекст берётся строго из err.File.
// report.SourceCode — fallback для случая, когда резолвер не может
// прочитать файл (например, main.sk передан как уже прочитанные строки).
// ============================================================================

func PrintErrorReport(report ErrorReport) {
	// Дедупликация warnings.
	if len(report.Warnings) > 0 {
		uniqueWarnings := []SkorpionError{}
		seen := make(map[string]bool)
		for _, w := range report.Warnings {
			key := fmt.Sprintf("%s:%s:%d:%d", w.File, w.Code, w.Line, w.Column)
			if !seen[key] {
				seen[key] = true
				uniqueWarnings = append(uniqueWarnings, w)
			}
		}

		cache := make(map[string][]string)
		for _, w := range uniqueWarnings {
			lines := resolveSourceCached(w.File, report, cache)
			PrintWarning(w, lines)
			fmt.Println()
		}
	}

	if len(report.Errors) == 0 {
		if len(report.Warnings) > 0 {
			fmt.Printf("%s in %s (%dms) with %s (count %d).\n",
				cli.Colors.Yellow("Succeeded"),
				report.FilePath,
				report.TotalTime.Milliseconds(),
				cli.Colors.Yellow("warnings"),
				len(report.Warnings))
		}
		return
	}

	// Дедупликация errors.
	uniqueErrors := []SkorpionError{}
	seen := make(map[string]bool)
	for _, err := range report.Errors {
		key := fmt.Sprintf("%s:%s:%d:%d", err.File, err.Code, err.Line, err.Column)
		if !seen[key] {
			seen[key] = true
			uniqueErrors = append(uniqueErrors, err)
		}
	}

	cache := make(map[string][]string)
	for _, err := range uniqueErrors {
		lines := resolveSourceCached(err.File, report, cache)
		PrintError(err, lines)
		fmt.Println()
	}

	fmt.Printf("%s in %s (%dms) with %s (count %d).\n",
		cli.Colors.Red("Failed"),
		report.FilePath,
		report.TotalTime.Milliseconds(),
		cli.Colors.Red("errors"),
		len(uniqueErrors))
}

// resolveSourceCached — resolveSource с кэшем по файлу.
//
// Fallback на report.SourceCode, только если:
//   - path == report.FilePath, ИЛИ
//   - resolveSource(path) вернул nil (файл недоступен).
//
// ВАЖНО: fallback на report.SourceCode при чужом path'е больше не применяется —
// лучше показать ошибку без контекста, чем строки из другого файла.
func resolveSourceCached(path string, report ErrorReport, cache map[string][]string) []string {
	if lines, ok := cache[path]; ok {
		return lines
	}

	lines := resolveSource(path)

	// Если резолвер не смог, и это «главный» файл — отдаём report.SourceCode.
	if lines == nil && path == report.FilePath && len(report.SourceCode) > 0 {
		lines = report.SourceCode
	}

	cache[path] = lines
	return lines
}

// ============================================================================
// ExplainError — без изменений.
// ============================================================================

func ExplainError(code string) {
	if len(code) > 5 && code[:5] == "Warn+" {
		code = code[5:]
	} else if len(code) > 4 && code[:4] == "Err+" {
		code = code[4:]
	}

	info, ok := ErrorCodes[code]
	if !ok {
		fmt.Printf("%s Unknown error code: %s\n", cli.Colors.Red("✗"), code)
		fmt.Println("Run 'skorpion --explain <code>' with a valid error code.")
		return
	}

	fmt.Printf("%s %s%s\n",
		cli.Colors.Bold("Code:"),
		cli.Colors.Red(info.Code),
		cli.Colors.Dim(" — "+info.Message))
	fmt.Println()

	fmt.Printf("%s\n", cli.Colors.Bold("Description:"))
	fmt.Printf("  %s\n", info.Description)
	fmt.Println()

	fmt.Printf("%s\n", cli.Colors.Bold("Tip:"))
	fmt.Printf("  %s\n", info.Tip)

	if info.Example != "" {
		fmt.Println()
		fmt.Printf("%s\n", cli.Colors.Bold("Example:"))
		for _, line := range strings.Split(info.Example, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Println()
	fmt.Printf("%s\n", cli.Colors.Dim("Run 'skorpion build' to see the error in context."))
}
