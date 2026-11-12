// Package debug — отладочный вывод Skorpion.
//
// Раньше `debug.Debug` импортировал `cli` только ради `Colorize` +
// ANSI-констант. Это создавало import-cycle: `cli/profiles.go` не мог
// использовать `debug.Debug`, потому что `cli` → `debug` → `cli`.
//
// Теперь `debug` самодостаточен: ANSI-константы — локальные, цветность
// управляется через `SetColored` (вызывается из `cli` при `--uncolored`
// / `skorpion color false` / `NO_COLOR`).
package debug

import (
	"fmt"
	"os"
	"skrp/res/log"
	"strings"
)

// ANSI-константы. Должны совпадать с cli.BRIGHT_BLACK / cli.BG_BLACK /
// cli.BG_DEFAULT — иначе debug-строки будут отличаться от остального вывода.
const (
	ansiReset       = "\033[0m"
	ansiBrightBlack = "\033[90m"
	ansiBgBlack     = "\033[40m"
	ansiBgDefault   = "\033[49m"
)

var (
	// IsDevMode — true, если включён --debug / SKORPION_DEBUG=true.
	IsDevMode = false

	// colored — рисовать ли ANSI-цвета в консоли. Синхронизируется с
	// cli.Colors.Enabled через SetColored (см. cli/actions.go, skorpion.go).
	colored = true
)

func init() {
	if os.Getenv("SKORPION_DEBUG") == "true" {
		IsDevMode = true
	}
	for _, arg := range os.Args {
		if arg == "--debug" || arg == "-d" {
			IsDevMode = true
		}
		if arg == "--uncolored" {
			colored = false
		}
	}
	if os.Getenv("NO_COLOR") != "" {
		colored = false
	}
	if IsDevMode {
		log.EnableDebug()
	}
}

// SetColored включает/выключает цвета в консольном debug-выводе.
// Вызывается из cli при переключении цветов.
func SetColored(v bool) {
	colored = v
}

// Debug пишет debug-сообщение:
//   - в ring лога — всегда (нужно для crash-репорта);
//   - в консоль — только если IsDevMode (флаг --debug или SKORPION_DEBUG=true).
//
// В консольном выводе формат совпадает с прежним cli.Colors.Colorize:
//
//	[90m[40m DEBUG [49m <msg>
func Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	// В ring + файл лога — всегда.
	log.Debug("%s", msg)

	if !IsDevMode {
		return
	}

	var prefix string
	if colored {
		prefix = ansiBrightBlack + ansiBgBlack + " DEBUG " + ansiBgDefault + " "
	} else {
		prefix = " DEBUG  "
	}

	// Гарантируем один \n в конце, чтобы не «слипалось».
	if strings.HasSuffix(msg, "\n") {
		fmt.Print(prefix + msg)
	} else {
		fmt.Println(prefix + msg)
	}
}
