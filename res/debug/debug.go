package debug

import (
	"fmt"
	"os"
	"skrp/res/cli"
	"skrp/res/log"
	"strings"
)

var IsDevMode = false

func init() {
	if os.Getenv("SKORPION_DEBUG") == "true" {
		IsDevMode = true
	}
	for _, arg := range os.Args {
		if arg == "--debug" || arg == "-d" {
			IsDevMode = true
			break
		}
	}
	if IsDevMode {
		log.EnableDebug()
	}
}

// Debug — как раньше, но с дублированием в лог (ring + файл).
func Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	// В ring + файл — всегда (ring нужен для crash-репорта).
	log.Debug("%s", msg)

	if !IsDevMode {
		return
	}
	colorized := cli.Colors.Colorize(
		cli.BRIGHT_BLACK+cli.BG_BLACK, " DEBUG "+cli.BG_DEFAULT+" "+msg)
	if !strings.HasSuffix(msg, "\n") {
		colorized += "\n"
	}
	fmt.Print(colorized)
}

func IsDebug() bool {
	return IsDevMode
}
