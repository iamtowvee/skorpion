package utils

// SanitizeCIdent приводит строку к валидному C-идентификатору:
// разрешены a-z, A-Z, 0-9, _. Остальные символы → '_'.
// Пустая строка → "v" (чтобы не получить невалидное "").
//
// Раньше существовала как:
//   - backend/codegen.go::sanitizeCName
//   - backend/pipeline.go::sanitizeGlobalName
func SanitizeCIdent(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "v"
	}
	return string(b)
}

// SanitizePath приводит строку к безопасному slug'у для путей:
// разрешены a-z, A-Z, 0-9, _, ., -. Остальные символы → '_'.
// Пустая строка → "" (в отличие от SanitizeCIdent — здесь пустой slug
// допустим: пути собираются через filepath.Join).
//
// Раньше существовала как log/paths.go::sanitizeSlug.
func SanitizePath(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '.', c == '-':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	return string(b)
}
