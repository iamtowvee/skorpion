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

type ProfileStore struct {
	Windows []Profile         `json:"windows"`
	Linux   []Profile         `json:"linux"`
	Current map[string]string `json:"current"`
}

type ProfileManager struct {
	Store      *ProfileStore
	ConfigPath string
}

func NewProfileManager() *ProfileManager {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".skorpion")
	configPath := filepath.Join(configDir, "profiles.json")

	pm := &ProfileManager{
		Store:      &ProfileStore{},
		ConfigPath: configPath,
	}
	pm.Load()
	return pm
}

func (pm *ProfileManager) Load() {
	if _, err := os.Stat(pm.ConfigPath); os.IsNotExist(err) {
		pm.Store = defaultStore()
		pm.Save()
		return
	}

	data, err := os.ReadFile(pm.ConfigPath)
	if err != nil {
		pm.Store = defaultStore()
		return
	}

	if err := json.Unmarshal(data, pm.Store); err != nil {
		pm.Store = defaultStore()
	}

	if pm.Store.Current == nil {
		pm.Store.Current = map[string]string{"windows": "auto", "linux": "auto"}
	}
	if pm.Store.Windows == nil {
		pm.Store.Windows = []Profile{{ID: 1, Name: "auto", Path: "auto"}}
	}
	if pm.Store.Linux == nil {
		pm.Store.Linux = []Profile{{ID: 1, Name: "auto", Path: "auto"}}
	}
}

func defaultStore() *ProfileStore {
	return &ProfileStore{
		Windows: []Profile{{ID: 1, Name: "auto", Path: "auto"}},
		Linux:   []Profile{{ID: 1, Name: "auto", Path: "auto"}},
		Current: map[string]string{"windows": "auto", "linux": "auto"},
	}
}

func (pm *ProfileManager) Save() {
	dir := filepath.Dir(pm.ConfigPath)
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(pm.Store, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(pm.ConfigPath, data, 0644)
}

// ============ Добавление ============

func (pm *ProfileManager) AddProfile(osName, name, path string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	for _, ch := range name {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return fmt.Errorf("profile name must contain only A-Za-z0-9_- (got '%s')", name)
		}
	}

	if name == "auto" {
		return fmt.Errorf("'auto' is a reserved profile name")
	}

	profiles := pm.getProfilesByOS(osName)
	for _, p := range profiles {
		if p.Name == name {
			return fmt.Errorf("profile '%s' already exists for %s", name, osName)
		}
	}

	if path != "auto" {
		if !pm.compilerExists(path) {
			return fmt.Errorf("compiler '%s' not found", path)
		}
	}

	maxID := 0
	for _, p := range profiles {
		if p.ID > maxID {
			maxID = p.ID
		}
	}

	newProfile := Profile{ID: maxID + 1, Name: name, Path: path}
	pm.setProfilesByOS(osName, append(profiles, newProfile))
	pm.Save()
	return nil
}

// ============ Редактирование ============

func (pm *ProfileManager) EditProfile(osName, name, newPath string) error {
	if name == "auto" {
		return fmt.Errorf("cannot edit 'auto' profile")
	}

	if !pm.compilerExists(newPath) {
		return fmt.Errorf("compiler '%s' not found", newPath)
	}

	profiles := pm.getProfilesByOS(osName)
	for i, p := range profiles {
		if p.Name == name {
			profiles[i].Path = newPath
			pm.setProfilesByOS(osName, profiles)
			pm.Save()
			return nil
		}
	}

	return fmt.Errorf("profile '%s' not found for %s", name, osName)
}

// ============ Установка текущего ============

func (pm *ProfileManager) SetProfile(osName, name string) error {
	profiles := pm.getProfilesByOS(osName)
	for _, p := range profiles {
		if p.Name == name {
			pm.Store.Current[osName] = name
			pm.Save()
			return nil
		}
	}
	return fmt.Errorf("profile '%s' not found for %s", name, osName)
}

// ============ Удаление ============

func (pm *ProfileManager) DeleteProfile(osName, name string) error {
	if name == "auto" {
		return fmt.Errorf("cannot delete 'auto' profile")
	}

	profiles := pm.getProfilesByOS(osName)
	for i, p := range profiles {
		if p.Name == name {
			pm.setProfilesByOS(osName, append(profiles[:i], profiles[i+1:]...))
			pm.Save()
			return nil
		}
	}

	return fmt.Errorf("profile '%s' not found for %s", name, osName)
}

// ============ Списки ============

func (pm *ProfileManager) ListProfiles(osName string) {
	profiles := pm.getProfilesByOS(osName)
	current := pm.Store.Current[osName]

	fmt.Printf("%s\t%s\t%s\n", Colors.Bold("ID"), Colors.Bold("Name"), Colors.Bold("Path"))
	fmt.Println(Colors.Dim("--\t----\t----"))
	for _, p := range profiles {
		cur := ""
		if p.Name == current {
			cur = Colors.Green(" (current)")
		}
		status := Colors.Green("✓")
		if p.Name != "auto" && !pm.compilerExists(p.Path) {
			status = Colors.Red("✗")
		}
		fmt.Printf("%d\t%s\t%s %s%s\n", p.ID, p.Name, p.Path, status, cur)
	}
}

func (pm *ProfileManager) ShowCurrentProfile(osName string) {
	current := pm.Store.Current[osName]
	fmt.Printf("Current %s profile: %s\n", osName, Colors.Cyan(current))
}

// ============ Получение компилятора ============

// GetCompilerForOS возвращает путь к компилятору для конкретной ОС
func (pm *ProfileManager) GetCompilerForOS(osName string) string {
	normOS := osName
	switch osName {
	case "win", "windows":
		normOS = "windows"
	}
	current := pm.Store.Current[normOS]
	profiles := pm.getProfilesByOS(normOS)
	for _, p := range profiles {
		if p.Name == current {
			return p.Path
		}
	}
	return "auto"
}

// ============ Хелперы ============

func (pm *ProfileManager) getProfilesByOS(osName string) []Profile {
	switch osName {
	case "windows", "win":
		return pm.Store.Windows
	case "linux":
		return pm.Store.Linux
	}
	return nil
}

func (pm *ProfileManager) setProfilesByOS(osName string, profiles []Profile) {
	switch osName {
	case "windows", "win":
		pm.Store.Windows = profiles
	case "linux":
		pm.Store.Linux = profiles
	}
}

func (pm *ProfileManager) compilerExists(path string) bool {
	if path == "auto" {
		return true
	}

	fmt.Printf("[DEBUG] compilerExists: path=%q (len=%d)\n", path, len(path))

	// Проверяем каждый байт
	for i, b := range path {
		if b == ' ' || b == '"' || b == '\'' {
			fmt.Printf("[DEBUG]   byte[%d] = %q (0x%02x)\n", i, b, b)
		}
	}

	// 1. Единый путь
	if _, err := os.Stat(path); err == nil {
		fmt.Println("[DEBUG] os.Stat(path) OK")
		return true
	} else {
		fmt.Printf("[DEBUG] os.Stat(path) FAILED: %v\n", err)
	}

	// + .exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		if _, err := os.Stat(path + ".exe"); err == nil {
			fmt.Println("[DEBUG] os.Stat(path+.exe) OK")
			return true
		}
	}

	// 2. LookPath
	if _, err := exec.LookPath(path); err == nil {
		fmt.Println("[DEBUG] exec.LookPath(path) OK")
		return true
	}

	// 3. По частям
	parts := strings.Fields(path)
	fmt.Printf("[DEBUG] parts: %#v\n", parts)
	if len(parts) > 0 {
		if _, err := os.Stat(parts[0]); err == nil {
			fmt.Println("[DEBUG] os.Stat(parts[0]) OK")
			return true
		}
		if _, err := exec.LookPath(parts[0]); err == nil {
			fmt.Println("[DEBUG] exec.LookPath(parts[0]) OK")
			return true
		}
	}

	fmt.Println("[DEBUG] compilerExists: all checks failed")
	return false
}
