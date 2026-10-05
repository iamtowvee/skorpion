package errors

import (
	"fmt"
	"skrp/res/cli"
	"strings"
	"time"
)

// printTip выводит блок TIP: >>> TIP (салатовый), | текст, | (пустая)
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

func PrintError(err SkorpionError, sourceLines []string) {
	codeInfo := err.GetCodeInfo()

	// Всегда считаем width — если нет контекста, width = 1
	width := 1
	hasContext := len(sourceLines) > 0 && err.Line > 0 && err.Line <= len(sourceLines)
	var ctxStart, ctxEnd int
	if hasContext {
		ctxStart = err.Line - 3
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxEnd = err.Line + 2
		if ctxEnd > len(sourceLines) {
			ctxEnd = len(sourceLines)
		}
		width = len(fmt.Sprintf("%d", ctxEnd))
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05")
	fmt.Printf("%s %s%s%s %s\n",
		cli.Colors.Dim("["+timestamp+" >"),
		cli.Colors.Red("Err+"+err.Code),
		cli.Colors.Dim("]"),
		cli.Colors.Red(":"),
		cli.Colors.Bold(err.Message))

	filePath := err.File
	if filePath == "" {
		filePath = "<unknown>"
	}

	// Шапка: >>> выровнен с | контекста
	arrowPrefix := strings.Repeat(" ", width+1) + ">>>"
	fmt.Printf("%s %s %s %s %d, %s %d\n",
		cli.Colors.Dim(arrowPrefix),
		cli.Colors.Cyan(filePath),
		cli.Colors.Dim("|"),
		cli.Colors.Dim("L:"),
		err.Line,
		cli.Colors.Dim("C:"),
		err.Column)

	// Пустой префикс для | (на 1 больше, чем для >>>)
	pipePrefix := strings.Repeat(" ", width+2) + "|"

	if hasContext {
		fmt.Printf("%s\n", cli.Colors.Dim(pipePrefix))

		for i := ctxStart; i < ctxEnd; i++ {
			lineNum := i + 1
			line := sourceLines[i]
			lineNumStr := fmt.Sprintf("%*d", width, lineNum)

			if lineNum == err.Line {
				fmt.Printf(" %s %s %s\n",
					cli.Colors.Bold(lineNumStr),
					cli.Colors.Dim("|"),
					line)

				if err.Column > 0 {
					startCol := err.Column
					endCol := err.EndColumn
					if endCol <= startCol {
						endCol = startCol + 1 // 1 символ по умолчанию
					}
					pointerIndent := width + 4 + (startCol - 1)
					pointer := strings.Repeat(" ", pointerIndent) + strings.Repeat("~", endCol-startCol)
					fmt.Printf("%s\n", cli.Colors.Red(pointer))
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

func PrintWarning(warn SkorpionError, sourceLines []string) {
	codeInfo := warn.GetCodeInfo()

	// Всегда считаем width — если нет контекста, width = 1
	width := 1
	hasContext := len(sourceLines) > 0 && warn.Line > 0 && warn.Line <= len(sourceLines)
	var ctxStart, ctxEnd int
	if hasContext {
		ctxStart = warn.Line - 3
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxEnd = warn.Line + 2
		if ctxEnd > len(sourceLines) {
			ctxEnd = len(sourceLines)
		}
		width = len(fmt.Sprintf("%d", ctxEnd))
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05")
	fmt.Printf("%s %s%s%s %s\n",
		cli.Colors.Dim("["+timestamp+" >"),
		cli.Colors.Yellow("Warn+"+warn.Code),
		cli.Colors.Dim("]"),
		cli.Colors.Yellow(":"),
		cli.Colors.Bold(warn.Message))

	filePath := warn.File
	if filePath == "" {
		filePath = "<unknown>"
	}

	// Шапка: >>> выровнен с | контекста
	arrowPrefix := strings.Repeat(" ", width+1) + ">>>"
	fmt.Printf("%s %s %s %s %d, %s %d\n",
		cli.Colors.Dim(arrowPrefix),
		cli.Colors.Cyan(filePath),
		cli.Colors.Dim("|"),
		cli.Colors.Dim("L:"),
		warn.Line,
		cli.Colors.Dim("C:"),
		warn.Column)

	// Пустой префикс для |
	pipePrefix := strings.Repeat(" ", width+2) + "|"

	if hasContext {
		fmt.Printf("%s\n", cli.Colors.Dim(pipePrefix))

		for i := ctxStart; i < ctxEnd; i++ {
			lineNum := i + 1
			line := sourceLines[i]
			lineNumStr := fmt.Sprintf("%*d", width, lineNum)

			if lineNum == warn.Line {
				fmt.Printf(" %s %s %s\n",
					cli.Colors.Bold(lineNumStr),
					cli.Colors.Dim("|"),
					line)

				if warn.Column > 0 {
					startCol := warn.Column
					endCol := warn.EndColumn
					if endCol <= startCol {
						endCol = startCol + 1 // 1 символ по умолчанию
					}
					pointerIndent := width + 4 + (startCol - 1)
					pointer := strings.Repeat(" ", pointerIndent) + strings.Repeat("~", endCol-startCol)
					fmt.Printf("%s\n", cli.Colors.Yellow(pointer))
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

func PrintErrorReport(report ErrorReport) {
	// Сначала warnings
	if len(report.Warnings) > 0 {
		uniqueWarnings := []SkorpionError{}
		seen := make(map[string]bool)
		for _, w := range report.Warnings {
			key := fmt.Sprintf("%s:%d:%d", w.Code, w.Line, w.Column)
			if !seen[key] {
				seen[key] = true
				uniqueWarnings = append(uniqueWarnings, w)
			}
		}
		for _, w := range uniqueWarnings {
			PrintWarning(w, report.SourceCode)
			fmt.Println()
		}
	}

	// Если ошибок нет — только warnings
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

	// Убираем дубли ошибок
	uniqueErrors := []SkorpionError{}
	seen := make(map[string]bool)
	for _, err := range report.Errors {
		key := fmt.Sprintf("%s:%d:%d", err.Code, err.Line, err.Column)
		if !seen[key] {
			seen[key] = true
			uniqueErrors = append(uniqueErrors, err)
		}
	}

	for _, err := range uniqueErrors {
		PrintError(err, report.SourceCode)
		fmt.Println()
	}

	fmt.Printf("%s in %s (%dms) with %s (count %d).\n",
		cli.Colors.Red("Failed"),
		report.FilePath,
		report.TotalTime.Milliseconds(),
		cli.Colors.Red("errors"),
		len(uniqueErrors))
}

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
