package main

import (
	"os"
	"skrp/res"
)

func main() {
	res.InitLang(os.Args[1:])
}
