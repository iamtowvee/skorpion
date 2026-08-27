package errors

// Коды ошибок
const (
	// Лексические ошибки
	ERR_UNEXPECTED_CHAR     = "1000"
	ERR_UNTERMINATED_STRING = "1001"
	ERR_INVALID_NUMBER      = "1002"

	// Синтаксические ошибки
	ERR_UNEXPECTED_TOKEN  = "2000"
	ERR_MISSING_SEMICOLON = "2001"
	ERR_MISSING_BRACE     = "2002"
	ERR_MISSING_PAREN     = "2003"
	ERR_INVALID_SYNTAX    = "2004"

	// Семантические ошибки
	ERR_TYPE_MISMATCH     = "3000"
	ERR_UNDECLARED_VAR    = "3001"
	ERR_REDECLARED_VAR    = "3002"
	ERR_INVALID_TYPE      = "3003"
	ERR_INVALID_OPERATION = "3004"

	// Ошибки типов (специфичные для Skorpion)
	ERR_TYPE_MISMATCH_ANY = "1054" // как в документации

	// Ошибки импорта
	ERR_MODULE_NOT_FOUND = "4000"
	ERR_CIRCULAR_IMPORT  = "4001"

	// Ошибки компиляции
	ERR_COMPILER_FAILED = "5000"
	ERR_LINKER_FAILED   = "5001"
)
