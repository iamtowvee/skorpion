package front

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfigData структура конфигурации проекта
type ConfigData struct {
	Name         string   // Имя проекта
	Version      string   // Версия
	Authors      []string // Авторы
	Target       []string // Целевые платформы
	Main         string   // Главный файл
	BuildOutName string   // Имя выходного файла сборки
	BuildOutPath string   // Путь для сборки
	TestOutName  string   // Имя выходного файла тестов
	TestOutPath  string   // Путь для тестов
}

// ConfigParser парсер конфигурации
type ConfigParser struct{}

// NewConfigParser создает новый парсер конфигурации
func NewConfigParser() *ConfigParser {
	return &ConfigParser{}
}

// Parse парсит manifest.spc
func (c *ConfigParser) Parse(path string) (*ConfigData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть %s: %v", path, err)
	}
	defer file.Close()

	config := &ConfigData{
		Authors:      make([]string, 0),
		Target:       make([]string, 0),
		BuildOutPath: "bin/",
		TestOutPath:  "test/",
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if err := c.parseLine(line, config); err != nil {
			return nil, err
		}
	}

	// Проверяем обязательные поля
	if config.Name == "" {
		return nil, fmt.Errorf("поле 'name' обязательно")
	}
	if config.Version == "" {
		return nil, fmt.Errorf("поле 'version' обязательно")
	}
	if len(config.Target) == 0 {
		return nil, fmt.Errorf("поле 'target' обязательно")
	}
	if config.Main == "" {
		return nil, fmt.Errorf("поле 'main' обязательно")
	}

	// Устанавливаем значения по умолчанию
	if config.BuildOutName == "" {
		config.BuildOutName = config.Name + "_" + config.Version
	}
	if config.TestOutName == "" {
		config.TestOutName = config.Name + "_" + config.Version
	}

	return config, nil
}

func (c *ConfigParser) parseLine(line string, config *ConfigData) error {
	// Проверяем блоки (например, block{...})
	if strings.Contains(line, "{") && strings.Contains(line, "}") {
		return c.parseBlock(line, config)
	}

	// Обычный ключ: key[value]
	parts := strings.SplitN(line, "[", 2)
	if len(parts) != 2 {
		return fmt.Errorf("некорректный синтаксис: %s", line)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSuffix(strings.TrimSpace(parts[1]), "]")

	switch key {
	case "name":
		config.Name = c.parseString(value)
	case "version":
		config.Version = c.parseString(value)
	case "authors":
		config.Authors = c.parseArray(value)
	case "target":
		config.Target = c.parseArray(value)
	case "main":
		config.Main = c.parseString(value)
	case "buildOutName":
		config.BuildOutName = c.parseString(value)
	case "buildOutPath":
		config.BuildOutPath = c.parseString(value)
	case "testOutName":
		config.TestOutName = c.parseString(value)
	case "testOutPath":
		config.TestOutPath = c.parseString(value)
	}

	return nil
}

func (c *ConfigParser) parseBlock(line string, config *ConfigData) error {
	// Парсим block{key[value],keyTwo[value],}
	parts := strings.SplitN(line, "{", 2)
	if len(parts) != 2 {
		return fmt.Errorf("некорректный блок: %s", line)
	}

	// blockName - имя блока (используем _ чтобы избежать ошибки неиспользуемой переменной)
	_ = strings.TrimSpace(parts[0]) // Игнорируем имя блока
	content := strings.TrimSuffix(parts[1], "}")

	// Разбираем содержимое блока
	items := strings.Split(content, ",")
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		c.parseLine(item, config)
	}

	return nil
}

func (c *ConfigParser) parseString(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value[1 : len(value)-1]
	}
	return value
}

func (c *ConfigParser) parseArray(value string) []string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		value = value[1 : len(value)-1]
		items := strings.Split(value, ",")
		result := make([]string, 0)
		for _, item := range items {
			item = strings.TrimSpace(item)
			if strings.HasPrefix(item, "\"") && strings.HasSuffix(item, "\"") {
				item = item[1 : len(item)-1]
			}
			result = append(result, item)
		}
		return result
	}
	return []string{}
}
