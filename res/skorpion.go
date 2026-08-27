package res

import (
	"skrp/res/cli"
	"skrp/res/errors"
)

// InitLang - главная точка входа
func InitLang(args []string) {
	// Инициализация системы ошибок
	errors.InitErrors()
	defer errors.CloseErrors()

	// Если нет аргументов, показываем help
	if len(args) == 0 {
		cli.ShowHelp()
		return
	}

	// Проверяем флаги (--help, --version, и т.д.)
	if cli.HandleChecks(args) {
		return
	}

	// Обрабатываем команды (build, test, и т.д.)
	if err := cli.HandleActions(args); err != nil {
		errors.NewError("1001", err.Error())
	}
}
