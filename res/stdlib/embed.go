package stdlib

import _ "embed"

//go:embed io.sk
var IO_SK string

//go:embed ref.sk
var REF_SK string

//go:embed cfg.sk
var CFG_SK string

//go:embed os.sk
var OS_SK string

//go:embed strings.sk
var STRINGS_SK string

//go:embed math.sk
var MATH_SK string

//go:embed arrays.sk
var ARRAYS_SK string

var StdLib = map[string]string{
	"std/io":      IO_SK,
	"std/os":      OS_SK,
	"std/ref":     REF_SK,
	"std/cfg":     CFG_SK,
	"std/strings": STRINGS_SK,
	"std/math":    MATH_SK,
	"std/arrays":  ARRAYS_SK,
}

func GetModule(path string) (string, bool) {
	content, ok := StdLib[path]
	return content, ok
}

func HasModule(path string) bool {
	_, ok := StdLib[path]
	return ok
}
