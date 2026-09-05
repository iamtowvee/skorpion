package errors

import (
	"fmt"
	"skrp/res/cli"
	"strings"
	"time"
)

func PrintError(err SkorpionError, sourceLines []string) {
	codeInfo := err.GetCodeInfo()

	// [2026-09-05T15:21:43 > Err+0004]: Expected '}', got ',' (at 11:10)
	timestamp := time.Now().Format("2006-01-02T15:04:05")
	fmt.Printf("%s %s%s%s %s\n",
		cli.Colors.Dim("["+timestamp+" >"),
		cli.Colors.Red("Err+"+err.Code),
		cli.Colors.Dim("]"),
		cli.Colors.Red(":"),
		cli.Colors.Bold(err.Message))

	//    >>> F:\path\to\project\dir | L: 33, C: 15
	filePath := err.File
	if filePath == "" {
		filePath = "<unknown>"
	}
	fmt.Printf("   %s %s %s %s %d, %s %d\n",
		cli.Colors.Dim(">>>"),
		cli.Colors.Cyan(filePath),
		cli.Colors.Dim("|"),
		cli.Colors.Dim("L:"),
		err.Line,
		cli.Colors.Dim("C:"),
		err.Column)

	// Контекст: 2 строки до и 2 строки после
	if len(sourceLines) > 0 && err.Line > 0 && err.Line <= len(sourceLines) {
		fmt.Printf("    |\n")

		// Показываем 2 строки до
		start := err.Line - 3
		if start < 0 {
			start = 0
		}
		end := err.Line + 2
		if end > len(sourceLines) {
			end = len(sourceLines)
		}

		for i := start; i < end; i++ {
			lineNum := i + 1
			line := sourceLines[i]

			// Номер строки
			lineNumStr := fmt.Sprintf("%2d", lineNum)
			if lineNum == err.Line {
				// Текущая строка — жирным и с указателем
				fmt.Printf(" %s | %s\n",
					cli.Colors.Bold(lineNumStr),
					line)

				// Указатель на позицию ошибки
				if err.Column > 0 && err.Column <= len(line) {
					// Находим конец токена
					endCol := err.Column
					for endCol < len(line) && line[endCol] != ' ' && line[endCol] != ',' && line[endCol] != ';' && line[endCol] != ')' && line[endCol] != '(' {
						endCol++
					}
					if endCol == err.Column {
						endCol = err.Column + 1
					}
					pointer := strings.Repeat(" ", err.Column-1) + strings.Repeat("~", endCol-err.Column+1)
					fmt.Printf("    | %s\n", cli.Colors.Red(pointer))
				}
			} else {
				// Обычные строки
				fmt.Printf(" %s | %s\n",
					cli.Colors.Dim(lineNumStr),
					line)
			}
		}
		fmt.Printf("    |\n")
	}

	// TIP
	if codeInfo.Tip != "" {
		fmt.Printf("   %s\n", cli.Colors.Yellow(">>> TIP"))
		fmt.Printf("    | %s\n", codeInfo.Tip)
		fmt.Printf("    |\n")
	}
}

func PrintErrorReport(report ErrorReport) {
	if len(report.Errors) == 0 {
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

	// Итог
	if len(uniqueErrors) == 1 {
		fmt.Printf("%s in %s (%dms) with %s (count %d).\n",
			cli.Colors.Red("Failed"),
			report.FilePath,
			report.TotalTime.Milliseconds(),
			cli.Colors.Red("errors"),
			len(uniqueErrors))
	} else {
		fmt.Printf("%s in %s (%dms) with %s (count %d).\n",
			cli.Colors.Red("Failed"),
			report.FilePath,
			report.TotalTime.Milliseconds(),
			cli.Colors.Red("errors"),
			len(uniqueErrors))
	}
}

func ExplainError(code string) {
	if len(code) > 4 && code[:4] == "Err+" {
		code = code[4:]
	}

	info, ok := ErrorCodes[code]
	if !ok {
		fmt.Printf("%s Unknown error code: %s\n", cli.Colors.Red("✗"), code)
		fmt.Println("Run 'skorpion --explain <code>' with a valid error code.")
		return
	}

	fmt.Printf("%s %s%s\n",
		cli.Colors.Bold("Error:"),
		cli.Colors.Red("Err+"+info.Code),
		cli.Colors.Dim(" — "+info.Message))
	fmt.Println()

	fmt.Printf("%s\n", cli.Colors.Bold("Description:"))
	fmt.Printf("  %s\n", info.Description)
	fmt.Println()

	fmt.Printf("%s\n", cli.Colors.Bold("Tip:"))
	fmt.Printf("  %s\n", info.Tip)
	fmt.Println()

	fmt.Printf("%s\n", cli.Colors.Dim("Run 'skorpion build' to see the error in context."))
}
