package cli

import (
	"os"
)

// CLI аргументы
type Args struct {
	Help    bool
	Version bool
	Colors  bool
	Updates bool
	Path    string
	Profile string
}

// ParseArgs парсит аргументы командной строки
func ParseArgs() *Args {
	args := &Args{
		Colors:  true, // По умолчанию
		Updates: true, // По умолчанию
	}

	// Проверяем флаги
	for i := 0; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--help", "-h":
			args.Help = true
		case "--version", "-v":
			args.Version = true
		case "--colors":
			if i+1 < len(os.Args) {
				args.Colors = os.Args[i+1] == "true"
				i++
			}
		case "--updates":
			if i+1 < len(os.Args) {
				args.Updates = os.Args[i+1] == "true"
				i++
			}
		case "--path":
			if i+1 < len(os.Args) {
				args.Path = os.Args[i+1]
				i++
			}
		case "--current-profile", "--profile-list":
			// Обрабатываются в actions.go
		}
	}

	return args
}

// GetCurrentProfile возвращает текущий профиль компилятора
func GetCurrentProfile() string {
	// Читаем из конфига или возвращаем "auto"
	config := loadConfig()
	if config.CurrentProfile != "" {
		return config.CurrentProfile
	}
	return "auto"
}

// GetColors возвращает настройку цветов
func GetColors() bool {
	config := loadConfig()
	return config.Colors
}

// GetUpdates возвращает настройку проверки обновлений
func GetUpdates() bool {
	config := loadConfig()
	return config.Updates
}
