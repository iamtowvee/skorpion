package stdlib

import _ "embed"

//go:embed io.sk
var IO_SK string

//go:embed ref.sk
var IO_REF string

var StdLib = map[string]string{
	"std/io":  IO_SK,
	"std/ref": IO_REF,
}

func GetModule(path string) (string, bool) {
	content, ok := StdLib[path]
	return content, ok
}

func HasModule(path string) bool {
	_, ok := StdLib[path]
	return ok
}
