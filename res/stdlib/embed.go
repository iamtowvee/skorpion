package stdlib

import _ "embed"

//go:embed io.sk
var IO_SK string

//go:embed ref.sk
var REF_SK string

//go:embed cfg.sk
var CFG_SK string

var StdLib = map[string]string{
	"std/io":  IO_SK,
	"std/ref": REF_SK,
 "std/cfg": CFG_SK,
}

func GetModule(path string) (string, bool) {
	content, ok := StdLib[path]
	return content, ok
}

func HasModule(path string) bool {
	_, ok := StdLib[path]
	return ok
}
