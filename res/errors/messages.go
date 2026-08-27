package errors

// GetErrorMessage возвращает сообщение для кода ошибки
func GetErrorMessage(code string) string {
	messages := map[string]string{
		ERR_UNEXPECTED_CHAR:     "Неожиданный символ",
		ERR_UNTERMINATED_STRING: "Незакрытая строка",
		ERR_INVALID_NUMBER:      "Некорректное число",

		ERR_UNEXPECTED_TOKEN:  "Неожиданный токен",
		ERR_MISSING_SEMICOLON: "Пропущена ;",
		ERR_MISSING_BRACE:     "Пропущена }",
		ERR_MISSING_PAREN:     "Пропущена )",
		ERR_INVALID_SYNTAX:    "Некорректный синтаксис",

		ERR_TYPE_MISMATCH:     "Несоответствие типов",
		ERR_UNDECLARED_VAR:    "Необъявленная переменная",
		ERR_REDECLARED_VAR:    "Повторное объявление переменной",
		ERR_INVALID_TYPE:      "Некорректный тип",
		ERR_INVALID_OPERATION: "Некорректная операция",

		ERR_TYPE_MISMATCH_ANY: "Type mismatch (Try to write wrong type into any)",

		ERR_MODULE_NOT_FOUND: "Модуль не найден",
		ERR_CIRCULAR_IMPORT:  "Циклический импорт",

		ERR_COMPILER_FAILED: "Ошибка компиляции C кода",
		ERR_LINKER_FAILED:   "Ошибка линковки",
	}

	if msg, ok := messages[code]; ok {
		return msg
	}
	return "Неизвестная ошибка"
}
