package errors

import (
	"fmt"
	"skrp/res/cli"
	"strings"
	"time"
)

// printTip выводит блок TIP: >>> TIP (салатовый), | текст, | (пустая).
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

// printDiag — единая отрисовка Error и Warning.
//
// Раньше PrintError и PrintWarning были на 95% идентичны:
// отличались только цвет и префикс ("Err+" vs "Warn+").
// Параметр isWarn выбирает цветовую схему.
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

	// width — для выравнивания контекста и >>>.
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

	// Шапка: >>> выровнен с | контекста.
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

func PrintErrorReport(report ErrorReport) {
	// Сначала warnings (с дедупликацией).
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

	// Если ошибок нет — только warnings.
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

	// Убираем дубли ошибок.
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
