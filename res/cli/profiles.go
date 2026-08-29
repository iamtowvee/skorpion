package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Profile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type ProfileManager struct {
	Profiles   []Profile
	ConfigPath string
}

func NewProfileManager() *ProfileManager {
	// Путь к файлу конфигурации профилей
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".skorpion")
	configPath := filepath.Join(configDir, "profiles.json")

	pm := &ProfileManager{
		Profiles:   []Profile{},
		ConfigPath: configPath,
	}

	pm.Load()
	return pm
}

func (pm *ProfileManager) Load() {
	// Проверяем, существует ли файл
	if _, err := os.Stat(pm.ConfigPath); os.IsNotExist(err) {
		// Создаём дефолтные профили
		pm.Profiles = []Profile{
			{ID: 1, Name: "auto", Path: "auto"},
		}
		pm.Save()
		return
	}

	data, err := os.ReadFile(pm.ConfigPath)
	if err != nil {
		pm.Profiles = []Profile{
			{ID: 1, Name: "auto", Path: "auto"},
		}
		return
	}

	err = json.Unmarshal(data, &pm.Profiles)
	if err != nil {
		pm.Profiles = []Profile{
			{ID: 1, Name: "auto", Path: "auto"},
		}
	}
}

func (pm *ProfileManager) Save() {
	// Создаём директорию если её нет
	dir := filepath.Dir(pm.ConfigPath)
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(pm.Profiles, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(pm.ConfigPath, data, 0644)
}

func (pm *ProfileManager) AddProfile(name, path string) error {
	// Проверяем, что имя не пустое
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	// Проверяем, что имя содержит только A-Za-z
	for _, ch := range name {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) {
			return fmt.Errorf("profile name must contain only A-Za-z (got '%s')", name)
		}
	}

	// Проверяем, что профиль не существует
	for _, p := range pm.Profiles {
		if p.Name == name {
			return fmt.Errorf("profile '%s' already exists", name)
		}
	}

	// Проверяем, что компилятор существует (через PATH или по полному пути)
	if path != "auto" {
		found := false

		// 1. Проверяем как есть (полный путь или имя в PATH)
		if _, err := exec.LookPath(path); err == nil {
			found = true
		}

		// 2. Если не найден, пробуем добавить расширение на Windows
		if !found && runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
			if _, err := exec.LookPath(path + ".exe"); err == nil {
				found = true
				path = path + ".exe"
			}
		}

		// 3. Если не найден, проверяем как полный путь к файлу
		if !found {
			if _, err := os.Stat(path); err == nil {
				found = true
			}
		}

		if !found {
			return fmt.Errorf("compiler '%s' not found in PATH or at specified path", path)
		}
	}

	// Находим следующий ID
	maxID := 0
	for _, p := range pm.Profiles {
		if p.ID > maxID {
			maxID = p.ID
		}
	}

	pm.Profiles = append(pm.Profiles, Profile{
		ID:   maxID + 1,
		Name: name,
		Path: path,
	})

	pm.Save()
	return nil
}

func (pm *ProfileManager) EditProfile(name, newPath string) error {
	// Нельзя редактировать auto
	if name == "auto" {
		return fmt.Errorf("cannot edit 'auto' profile")
	}

	// Проверяем, что компилятор существует
	found := false

	// 1. Проверяем как есть (полный путь или имя в PATH)
	if _, err := exec.LookPath(newPath); err == nil {
		found = true
	}

	// 2. Если не найден, пробуем добавить расширение на Windows
	if !found && runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(newPath), ".exe") {
		if _, err := exec.LookPath(newPath + ".exe"); err == nil {
			found = true
			newPath = newPath + ".exe"
		}
	}

	// 3. Если не найден, проверяем как полный путь к файлу
	if !found {
		if _, err := os.Stat(newPath); err == nil {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("compiler '%s' not found in PATH or at specified path", newPath)
	}

	for i, p := range pm.Profiles {
		if p.Name == name {
			pm.Profiles[i].Path = newPath
			pm.Save()
			return nil
		}
	}

	return fmt.Errorf("profile '%s' not found", name)
}

func (pm *ProfileManager) SetProfile(name string) error {
	// Проверяем, что профиль существует
	for _, p := range pm.Profiles {
		if p.Name == name {
			// Сохраняем выбранный профиль в отдельный файл
			homeDir, _ := os.UserHomeDir()
			configDir := filepath.Join(homeDir, ".skorpion")
			currentPath := filepath.Join(configDir, "current_profile.txt")
			os.WriteFile(currentPath, []byte(name), 0644)
			return nil
		}
	}

	return fmt.Errorf("profile '%s' not found", name)
}

func (pm *ProfileManager) GetCurrentProfile() string {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".skorpion")
	currentPath := filepath.Join(configDir, "current_profile.txt")

	data, err := os.ReadFile(currentPath)
	if err != nil {
		return "auto"
	}

	name := strings.TrimSpace(string(data))

	// Проверяем, что профиль существует
	for _, p := range pm.Profiles {
		if p.Name == name {
			return name
		}
	}

	return "auto"
}

func (pm *ProfileManager) DeleteProfile(name string) error {
	// Нельзя удалить auto
	if name == "auto" {
		return fmt.Errorf("cannot delete 'auto' profile")
	}

	for i, p := range pm.Profiles {
		if p.Name == name {
			pm.Profiles = append(pm.Profiles[:i], pm.Profiles[i+1:]...)
			pm.Save()
			return nil
		}
	}

	return fmt.Errorf("profile '%s' not found", name)
}

func (pm *ProfileManager) ListProfiles() {
	fmt.Println("ID\tName\tPath")
	fmt.Println("--\t----\t----")
	for _, p := range pm.Profiles {
		current := ""
		if p.Name == pm.GetCurrentProfile() {
			current = " (current)"
		}

		// Показываем, найден ли компилятор в PATH
		status := "✓"
		if p.Name != "auto" {
			if !pm.CompilerExists(p.Path) {
				status = "✗"
			}
		}

		fmt.Printf("%d\t%s\t%s %s%s\n", p.ID, p.Name, p.Path, status, current)
	}
}

func (pm *ProfileManager) GetProfilePath(name string) string {
	for _, p := range pm.Profiles {
		if p.Name == name {
			return p.Path
		}
	}
	return ""
}

func (pm *ProfileManager) CompilerExists(path string) bool {
	if path == "auto" {
		// Для auto проверяем наличие любого компилятора
		compilers := []string{"gcc", "clang", "tcc"}
		for _, c := range compilers {
			if _, err := exec.LookPath(c); err == nil {
				return true
			}
			// Windows
			if runtime.GOOS == "windows" {
				if _, err := exec.LookPath(c + ".exe"); err == nil {
					return true
				}
			}
		}
		return false
	}

	// Проверяем конкретный компилятор
	if _, err := exec.LookPath(path); err == nil {
		return true
	}

	// Windows с .exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		if _, err := exec.LookPath(path + ".exe"); err == nil {
			return true
		}
	}

	// Проверяем как полный путь
	if _, err := os.Stat(path); err == nil {
		return true
	}

	return false
}

func (pm *ProfileManager) FindCompiler(profileName string) string {
	// Если auto, ищем любой доступный компилятор
	if profileName == "auto" {
		// Приоритет: tcc, gcc, clang
		compilers := []string{"tcc", "gcc", "clang"}

		for _, c := range compilers {
			if path, err := exec.LookPath(c); err == nil {
				return path
			}
			// Windows
			if runtime.GOOS == "windows" {
				if path, err := exec.LookPath(c + ".exe"); err == nil {
					return path
				}
			}
		}
		return ""
	}

	// Ищем конкретный профиль
	path := pm.GetProfilePath(profileName)
	if path == "" {
		return ""
	}

	// Проверяем в PATH
	if path, err := exec.LookPath(path); err == nil {
		return path
	}

	// Windows с .exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		if path, err := exec.LookPath(path + ".exe"); err == nil {
			return path
		}
	}

	// Проверяем как полный путь
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return ""
}
