package cli

import (
	"fmt"
)

const skV = "v2026.08-b01"

// HandleChecks обрабатывает проверочные флаги (--help, --version, и т.д.)
func HandleChecks(args []string) bool {
	if len(args) == 0 {
		return false
	}

	switch args[0] {
	case "--help", "-h":
		ShowHelp()
		return true

	case "--version", "-v":
		ShowVersion()
		return true

	case "--colors":
		fmt.Printf("Цвета: %v\n", GetColors())
		return true

	case "--updates":
		fmt.Printf("Проверка обновлений: %v\n", GetUpdates())
		return true

	case "--current-profile":
		fmt.Printf("Текущий профиль: %s\n", GetCurrentProfile())
		return true

	case "--profile-list":
		ShowProfileList()
		return true
	}

	return false
}

// ShowHelp выводит справку
func ShowHelp() {
	fmt.Println(`Skorpion Compiler - Help

Использование:
  skorpion <команда> [параметры]
  skorpion <флаг>

Команды:
  build                    Собрать проект из текущей директории
  build --path="PATH"      Собрать проект из указанной директории
  test                     Тестовая сборка проекта
  
  add-profile NAME : PATH  Добавить профиль компилятора
  edit-profile NAME : PATH Изменить профиль компилятора
  set-profile NAME         Назначить профиль
  del-profile NAME         Удалить профиль
  
  color <true|false|toggle> Настройка цветов
  updates <true|false|toggle> Настройка проверки обновлений

Флаги:
  --help, -h               Показать эту справку
  --version, -v            Показать версию
  --colors                 Показать настройку цветов
  --updates                Показать настройку проверки обновлений
  --current-profile        Показать текущий профиль
  --profile-list           Показать список профилей

Примеры:
  skorpion build
  skorpion build --path="./my_project"
  skorpion add-profile mygcc : /usr/bin/gcc
  skorpion set-profile mygcc
  skorpion color true
`)
}

// ShowVersion выводит версию
func ShowVersion() {
	fmt.Println(`Skorpion Compiler v%s
Copyrights. (c) 2026 iamtowvee
Distributed under MIT License`, skV)
}

// ShowProfileList выводит список профилей
func ShowProfileList() {
	config := loadConfig()

	fmt.Println("ID | Name | Path")
	fmt.Println("-------------------")
	fmt.Printf("0  | auto | auto (default)\n")

	id := 1
	for name, path := range config.Profiles {
		if name != "auto" {
			fmt.Printf("%d  | %s | %s\n", id, name, path)
			id++
		}
	}
}
