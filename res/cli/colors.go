package cli

import (
	"os"
	"strings"
)

type Color struct {
	Enabled bool
}

var Colors = &Color{Enabled: true}

// ANSI color codes
const (
	RESET  = "\033[0m"
	BOLD   = "\033[1m"
	DIM    = "\033[2m"
	ITALIC = "\033[3m"
	UNDER  = "\033[4m"

	BLACK   = "\033[30m"
	RED     = "\033[31m"
	GREEN   = "\033[32m"
	YELLOW  = "\033[33m"
	BLUE    = "\033[34m"
	MAGENTA = "\033[35m"
	CYAN    = "\033[36m"
	WHITE   = "\033[37m"

	BRIGHT_BLACK   = "\033[90m"
	BRIGHT_RED     = "\033[91m"
	BRIGHT_GREEN   = "\033[92m"
	BRIGHT_YELLOW  = "\033[93m"
	BRIGHT_BLUE    = "\033[94m"
	BRIGHT_MAGENTA = "\033[95m"
	BRIGHT_CYAN    = "\033[96m"
	BRIGHT_WHITE   = "\033[97m"

	BG_BLACK   = "\033[40m"
	BG_RED     = "\033[41m"
	BG_GREEN   = "\033[42m"
	BG_YELLOW  = "\033[43m"
	BG_BLUE    = "\033[44m"
	BG_MAGENTA = "\033[45m"
	BG_CYAN    = "\033[46m"
	BG_WHITE   = "\033[47m"
)

// ColorString возвращает цветной текст, если цвета включены
func (c *Color) Colorize(code, text string) string {
	if !c.Enabled {
		return text
	}
	return code + text + RESET
}

// Colorizef форматирует и возвращает цветной текст
func (c *Color) Colorizef(code, format string, args ...interface{}) string {
	text := format
	if len(args) > 0 {
		text = strings.Replace(format, "{}", "%v", -1)
	}
	if !c.Enabled {
		return text
	}
	return code + text + RESET
}

// Сокращения для удобства
func (c *Color) Red(text string) string       { return c.Colorize(RED, text) }
func (c *Color) Green(text string) string     { return c.Colorize(GREEN, text) }
func (c *Color) Yellow(text string) string    { return c.Colorize(YELLOW, text) }
func (c *Color) Blue(text string) string      { return c.Colorize(BLUE, text) }
func (c *Color) Cyan(text string) string      { return c.Colorize(CYAN, text) }
func (c *Color) Magenta(text string) string   { return c.Colorize(MAGENTA, text) }
func (c *Color) White(text string) string     { return c.Colorize(WHITE, text) }
func (c *Color) Bold(text string) string      { return c.Colorize(BOLD, text) }
func (c *Color) Dim(text string) string       { return c.Colorize(DIM, text) }
func (c *Color) Underline(text string) string { return c.Colorize(UNDER, text) }

// Цветные ошибки
func (c *Color) Error(text string) string {
	return c.Colorize(RED, text)
}

func (c *Color) Warning(text string) string {
	return c.Colorize(YELLOW, text)
}

func (c *Color) Success(text string) string {
	return c.Colorize(GREEN, text)
}

func (c *Color) Info(text string) string {
	return c.Colorize(CYAN, text)
}

func (c *Color) Highlight(text string) string {
	return c.Colorize(BOLD+WHITE, text)
}

// Disable отключает цвета
func (c *Color) Disable() {
	c.Enabled = false
}

// Enable включает цвета
func (c *Color) Enable() {
	c.Enabled = true
}

// Toggle переключает цвета
func (c *Color) Toggle() {
	c.Enabled = !c.Enabled
}

// IsEnabled возвращает состояние цветов
func (c *Color) IsEnabled() bool {
	return c.Enabled
}

// ShouldUseColors определяет, нужно ли использовать цвета
func ShouldUseColors() bool {
	// Проверяем, что вывод не перенаправлен в файл
	info, _ := os.Stdout.Stat()
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return false
	}

	// Проверяем переменную окружения NO_COLOR
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	return Colors.Enabled
}
