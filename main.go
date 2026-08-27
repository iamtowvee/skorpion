package main

import (
	"os"
	skm "skrp/res"
)

func main() {
	skm.InitLang(os.Args[1:])
}
