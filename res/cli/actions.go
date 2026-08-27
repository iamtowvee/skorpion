package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"skrp/res/backend"
	"skrp/res/front"
	"skrp/res/midlevel"
)

// Config структура для хранения настроек
type Config struct {
	CurrentProfile string
	Colors         bool
	Updates        bool
	Profiles       map[string]string
}

var config *Config

func loadConfig() *Config {
	if config != nil {
		return config
	}

	config = &Config{
		CurrentProfile: "auto",
		Colors:         true,
		Updates:        true,
		Profiles: map[string]string{
			"auto": "auto",
		},
	}

	// TODO: Загружать из файла ~/.skorpion/config.json
	return config
}

// HandleActions обрабатывает команды
func HandleActions(args []string) error {
	if len(args) == 0 {
		return nil
	}

	command := args[0]

	switch command {
	case "build":
		return handleBuild(args)
	case "test":
		return handleTest(args)
	case "add-profile":
		return handleAddProfile(args)
	case "edit-profile":
		return handleEditProfile(args)
	case "set-profile":
		return handleSetProfile(args)
	case "del-profile":
		return handleDelProfile(args)
	case "color":
		return handleColor(args)
	case "updates":
		return handleUpdates(args)
	default:
		return fmt.Errorf("неизвестная команда: %s", command)
	}
}

// handleBuild - сборка проекта
func handleBuild(args []string) error {
	path := "."

	// Проверяем флаг --path
	for i := 0; i < len(args); i++ {
		if args[i] == "--path" && i+1 < len(args) {
			path = args[i+1]
			i++
		}
	}

	fmt.Printf("🔨 Сборка проекта из: %s\n", path)

	// Ищем manifest.spc
	manifestPath := filepath.Join(path, "manifest.spc")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("файл manifest.spc не найден в %s", path)
	}

	// 1. Парсим конфиг
	configData, err := front.ParseConfig(manifestPath)
	if err != nil {
		return err
	}

	// 2. Находим main файл
	mainFile := configData.Main
	if mainFile == "" {
		mainFile = "./main.sk"
	}

	// 3. Читаем исходный код
	code, err := os.ReadFile(filepath.Join(path, mainFile))
	if err != nil {
		return fmt.Errorf("не удалось прочитать %s: %v", mainFile, err)
	}

	// 4. Лексический анализ
	tokens, err := front.InitLexer(string(code))
	if err != nil {
		return err
	}

	// 5. Синтаксический анализ
	ast, err := front.InitParser(tokens)
	if err != nil {
		return err
	}

	// 6. Семантический анализ
	if err := midlevel.SemCheck(ast); err != nil {
		return err
	}

	// 7. Оптимизация IR
	ir := midlevel.OptimizeIR(ast)

	// 8. Генерация C кода
	cCode, err := backend.GenCFromIR(ir)
	if err != nil {
		return err
	}

	// 9. Сохраняем временный C файл
	tmpFile := filepath.Join(path, "temp_build.c")
	if err := os.WriteFile(tmpFile, []byte(cCode), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	// 10. Компилируем C код
	outName := configData.BuildOutName
	if outName == "" {
		outName = configData.Name + "_" + configData.Version
	}
	outPath := filepath.Join(path, configData.BuildOutPath, outName)

	if err := backend.Builder(tmpFile, outPath, GetCurrentProfile()); err != nil {
		return err
	}

	fmt.Printf("✅ Сборка завершена: %s\n", outPath)
	return nil
}

// handleTest - тестовая сборка
func handleTest(args []string) error {
	fmt.Println("🧪 Тестовая сборка проекта...")
	// Аналогично build, но с другими настройками
	return nil
}

// handleAddProfile - добавление профиля
func handleAddProfile(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("использование: skorpion add-profile NAME : PATH")
	}

	// Парсим "NAME : PATH"
	name := args[1]
	if args[2] != ":" {
		return fmt.Errorf("ожидается ':', получено %s", args[2])
	}
	if len(args) < 4 {
		return fmt.Errorf("путь не указан")
	}
	path := args[3]

	// Проверяем имя (только A-Za-z)
	for _, ch := range name {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) {
			return fmt.Errorf("имя профиля может содержать только A-Za-z")
		}
	}

	// Проверяем существование файла
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("файл не существует: %s", path)
	}

	config := loadConfig()
	config.Profiles[name] = path

	fmt.Printf("✅ Профиль '%s' добавлен: %s\n", name, path)
	return nil
}

// handleEditProfile - изменение профиля
func handleEditProfile(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("использование: skorpion edit-profile NAME : NEW_PATH")
	}

	name := args[1]
	if args[2] != ":" {
		return fmt.Errorf("ожидается ':', получено %s", args[2])
	}
	newPath := args[3]

	if name == "auto" {
		return fmt.Errorf("профиль 'auto' нельзя изменить")
	}

	config := loadConfig()
	if _, exists := config.Profiles[name]; !exists {
		return fmt.Errorf("профиль '%s' не найден", name)
	}

	config.Profiles[name] = newPath
	fmt.Printf("✅ Профиль '%s' обновлен: %s\n", name, newPath)
	return nil
}

// handleSetProfile - назначение профиля
func handleSetProfile(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("использование: skorpion set-profile NAME")
	}

	name := args[1]
	config := loadConfig()

	if _, exists := config.Profiles[name]; !exists {
		return fmt.Errorf("профиль '%s' не найден", name)
	}

	config.CurrentProfile = name
	fmt.Printf("✅ Текущий профиль: %s\n", name)
	return nil
}

// handleDelProfile - удаление профиля
func handleDelProfile(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("использование: skorpion del-profile NAME")
	}

	name := args[1]
	if name == "auto" {
		return fmt.Errorf("профиль 'auto' нельзя удалить")
	}

	config := loadConfig()
	if _, exists := config.Profiles[name]; !exists {
		return fmt.Errorf("профиль '%s' не найден", name)
	}

	delete(config.Profiles, name)
	fmt.Printf("✅ Профиль '%s' удален\n", name)
	return nil
}

// handleColor - настройка цветов
func handleColor(args []string) error {
	config := loadConfig()

	if len(args) == 1 {
		return fmt.Errorf("использование: skorpion color <true|false|toggle>")
	}

	if args[1] == "toggle" {
		config.Colors = !config.Colors
	} else if args[1] == "true" {
		config.Colors = true
	} else if args[1] == "false" {
		config.Colors = false
	} else {
		return fmt.Errorf("значение должно быть true, false или toggle")
	}

	fmt.Printf("✅ Цвета: %v\n", config.Colors)
	return nil
}

// handleUpdates - настройка обновлений
func handleUpdates(args []string) error {
	config := loadConfig()

	if len(args) == 1 {
		return fmt.Errorf("использование: skorpion updates <true|false|toggle>")
	}

	if args[1] == "toggle" {
		config.Updates = !config.Updates
	} else if args[1] == "true" {
		config.Updates = true
	} else if args[1] == "false" {
		config.Updates = false
	} else {
		return fmt.Errorf("значение должно быть true, false или toggle")
	}

	fmt.Printf("✅ Проверка обновлений: %v\n", config.Updates)
	return nil
}
