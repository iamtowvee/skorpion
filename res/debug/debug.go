package debug

import (
	"fmt"
	"os"
	"skrp/res/cli"
	"strings"
)

// Единственный флаг дебага
var IsDevMode = false

func init() {
	// Проверяем переменную окружения
	if os.Getenv("SKORPION_DEBUG") == "true" {
		IsDevMode = true
	}
	// Проверяем аргументы командной строки
	for _, arg := range os.Args {
		if arg == "--debug" || arg == "-d" {
			IsDevMode = true
			break
		}
	}
}

// Единая функция для дебаг-вывода
func Debug(format string, args ...interface{}) {
	if !IsDevMode {
		return
	}
	msg := fmt.Sprintf(format, args...)
	colorized := cli.Colors.Colorize(cli.BRIGHT_BLACK, "[DEBUG] "+msg)

	if !strings.HasSuffix(msg, "\n") {
		colorized += "\n"
	}
	fmt.Print(colorized)
}

// Или просто проверка
func IsDebug() bool {
	return IsDevMode
}
