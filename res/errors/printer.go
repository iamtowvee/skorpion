package errors

import (
	"fmt"
	"skrp/res/cli"
	"strings"
	"time"
)

func PrintError(err SkorpionError, sourceLines []string) {
	codeInfo := err.GetCodeInfo()

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
	fmt.Printf("   %s %s %s %s %d, %s %d\n",
		cli.Colors.Dim(">>>"),
		cli.Colors.Cyan(filePath),
		cli.Colors.Dim("|"),
		cli.Colors.Dim("L:"),
		err.Line,
		cli.Colors.Dim("C:"),
		err.Column)

	if len(sourceLines) > 0 && err.Line > 0 && err.Line <= len(sourceLines) {
		fmt.Printf("    |\n")

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

			lineNumStr := fmt.Sprintf("%2d", lineNum)
			if lineNum == err.Line {
				fmt.Printf(" %s | %s\n",
					cli.Colors.Bold(lineNumStr),
					line)

				if err.Column > 0 && err.Column <= len(line) {
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
				fmt.Printf(" %s | %s\n",
					cli.Colors.Dim(lineNumStr),
					line)
			}
		}
		fmt.Printf("    |\n")
	}

	if codeInfo.Tip != "" {
		fmt.Printf("   %s\n", cli.Colors.Yellow(">>> TIP"))
		fmt.Printf("    | %s\n", codeInfo.Tip)
		fmt.Printf("    |\n")
	}
}

func PrintWarning(warn SkorpionError, sourceLines []string) {
	codeInfo := warn.GetCodeInfo()

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
	fmt.Printf("   %s %s %s %s %d, %s %d\n",
		cli.Colors.Dim(">>>"),
		cli.Colors.Cyan(filePath),
		cli.Colors.Dim("|"),
		cli.Colors.Dim("L:"),
		warn.Line,
		cli.Colors.Dim("C:"),
		warn.Column)

	if len(sourceLines) > 0 && warn.Line > 0 && warn.Line <= len(sourceLines) {
		fmt.Printf("    |\n")
		start := warn.Line - 3
		if start < 0 {
			start = 0
		}
		end := warn.Line + 2
		if end > len(sourceLines) {
			end = len(sourceLines)
		}
		for i := start; i < end; i++ {
			lineNum := i + 1
			line := sourceLines[i]
			lineNumStr := fmt.Sprintf("%2d", lineNum)
			if lineNum == warn.Line {
				fmt.Printf(" %s | %s\n",
					cli.Colors.Bold(lineNumStr),
					line)
				if warn.Column > 0 && warn.Column <= len(line) {
					endCol := warn.Column
					for endCol < len(line) && line[endCol] != ' ' && line[endCol] != '"' {
						endCol++
					}
					if endCol == warn.Column {
						endCol = warn.Column + 1
					}
					pointer := strings.Repeat(" ", warn.Column-1) + strings.Repeat("~", endCol-warn.Column+1)
					fmt.Printf("    | %s\n", cli.Colors.Yellow(pointer))
				}
			} else {
				fmt.Printf(" %s | %s\n",
					cli.Colors.Dim(lineNumStr),
					line)
			}
		}
		fmt.Printf("    |\n")
	}

	if codeInfo.Tip != "" {
		fmt.Printf("   %s\n", cli.Colors.Yellow(">>> TIP"))
		fmt.Printf("    | %s\n", codeInfo.Tip)
		fmt.Printf("    |\n")
	}
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
