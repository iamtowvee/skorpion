// Package utils — общие утилиты, разделяемые между front, midlevel и backend.
//
// Здесь собраны функции, которые раньше дублировались в 3-4 местах:
//   - front/parser.go       (parseArrayElemTypeFromFullType)
//   - midlevel/semantic.go  (isUnionType, parseUnionTypes, isTypeInUnion,
//     isArrayTypeSemantic, parseArrayElemTypeSemantic)
//   - backend/pipeline.go   (isUnionTypeP, isArrayType, parseArrayElemType)
//   - backend/codegen.go    (isUnionTypeC, isArrayTypeC)
//
// Все варианты имели одинаковую семантику — теперь единый источник истины.
package utils

import "strings"

// IsUnionType сообщает, является ли t union-типом "T<A,B,...>".
func IsUnionType(t string) bool {
	return strings.HasPrefix(t, "T<") && strings.HasSuffix(t, ">")
}

// ParseUnionTypes извлекает список типов из "T<A,B,C>".
// Для не-union возвращает nil.
func ParseUnionTypes(t string) []string {
	if !IsUnionType(t) {
		return nil
	}
	inner := t[2 : len(t)-1]
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// IsTypeInUnion сообщает, входит ли t в union.
func IsTypeInUnion(union, t string) bool {
	for _, u := range ParseUnionTypes(union) {
		if u == t {
			return true
		}
	}
	return false
}

// IsArrayType сообщает, является ли t типом массива:
// "arr", "arr[int]", "arr[arr[string]]" и т.п.
func IsArrayType(t string) bool {
	return t == "arr" || strings.HasPrefix(t, "arr[")
}

// ParseArrayElemType возвращает тип элемента массива.
//   "arr"             → ""
//   "arr[int]"        → "int"
//   "arr[arr[int]]"   → "arr[int]"
//   "int"             → "int"   (не-массив, возвращается как есть)
func ParseArrayElemType(t string) string {
	if t == "" {
		return ""
	}
	if t == "arr" {
		return ""
	}
	if !strings.HasPrefix(t, "arr[") {
		return t
	}
	return t[4 : len(t)-1]
}
