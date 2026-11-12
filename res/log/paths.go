package log

import (
	"os"
	"path/filepath"
	"runtime"

	"skrp/res/utils"
)

// CacheRoot возвращает базовый каталог кэша Skorpion.
//
//	Windows:     %LOCALAPPDATA%\Skorpion
//	Linux/macOS: $XDG_DATA_HOME/skorpion или ~/.local/share/skorpion
func CacheRoot() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("APPDATA")
		}
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "Skorpion")
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "skorpion")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "skorpion")
}

// LogsRoot — общий каталог логов: <CacheRoot>/logs
func LogsRoot() string {
	return filepath.Join(CacheRoot(), "logs")
}

// ProjectLogDir — каталог логов проекта:
// <CacheRoot>/logs/<name>__<version>/
//
// Если name == "" — <CacheRoot>/logs/_no_project__0.0.0/
func ProjectLogDir(name, version string) string {
	if name == "" {
		name = "_no_project"
	}
	if version == "" {
		version = "0.0.0"
	}
	return filepath.Join(LogsRoot(), utils.SanitizePath(name)+"__"+utils.SanitizePath(version))
}

// LocalLogDir — локальный каталог логов: <projectDir>/logs/
func LocalLogDir(projectDir string) string {
	return filepath.Join(projectDir, "logs")
}
