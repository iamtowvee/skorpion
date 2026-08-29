package backend

import (
	"fmt"
	"strings"
)

type IRInstruction struct {
	Op       string
	Result   string
	Arg1     string
	Arg2     string
	Metadata map[string]interface{}
}

type IRFunction struct {
	Name         string
	ReturnType   string
	Params       []IRParam
	Locals       []string
	Instructions []IRInstruction
	IsExport     bool
}

type IRParam struct {
	Name string
	Type string
}

type IRProgram struct {
	Functions []IRFunction
	Globals   []IRGlobal
	Imports   []IRImport
	InlineC   string // Сырой C код из includeC {}
}

type IRGlobal struct {
	Name    string
	Type    string
	Value   string
	IsConst bool
}

type IRImport struct {
	Path  string
	Alias string
	All   bool
}

func (ir *IRProgram) String() string {
	var sb strings.Builder
	sb.WriteString("IR Program:\n")

	for _, imp := range ir.Imports {
		sb.WriteString(fmt.Sprintf("  Import: %s (alias: %s, all: %v)\n", imp.Path, imp.Alias, imp.All))
	}

	for _, g := range ir.Globals {
		sb.WriteString(fmt.Sprintf("  Global: %s %s = %s\n", g.Type, g.Name, g.Value))
	}

	for _, fn := range ir.Functions {
		sb.WriteString(fmt.Sprintf("\n  Function: %s(%s) -> %s\n", fn.Name, fn.ReturnType, fn.ReturnType))
		for _, param := range fn.Params {
			sb.WriteString(fmt.Sprintf("    Param: %s %s\n", param.Type, param.Name))
		}
		for _, local := range fn.Locals {
			sb.WriteString(fmt.Sprintf("    Local: %s\n", local))
		}
		for _, ins := range fn.Instructions {
			sb.WriteString(fmt.Sprintf("    %s", ins.String()))
		}
	}

	if ir.InlineC != "" {
		sb.WriteString(fmt.Sprintf("\n  Inline C:\n%s\n", ir.InlineC))
	}

	return sb.String()
}

func (ins *IRInstruction) String() string {
	meta := ""
	if len(ins.Metadata) > 0 {
		meta = fmt.Sprintf(" [%v]", ins.Metadata)
	}

	switch ins.Op {
	case "label":
		return fmt.Sprintf("%s:%s\n", ins.Result, meta)
	case "ret":
		if ins.Arg1 != "" {
			return fmt.Sprintf("  return %s%s\n", ins.Arg1, meta)
		}
		return fmt.Sprintf("  return%s\n", meta)
	case "call":
		return fmt.Sprintf("  %s = call %s(%s)%s\n", ins.Result, ins.Arg1, ins.Arg2, meta)
	case "=", "+=", "-=", "*=", "/=":
		return fmt.Sprintf("  %s %s %s%s\n", ins.Result, ins.Op, ins.Arg1, meta)
	default:
		if ins.Result != "" && ins.Arg1 != "" && ins.Arg2 != "" {
			return fmt.Sprintf("  %s = %s %s %s%s\n", ins.Result, ins.Arg1, ins.Op, ins.Arg2, meta)
		}
		if ins.Result != "" && ins.Arg1 != "" {
			return fmt.Sprintf("  %s = %s %s%s\n", ins.Result, ins.Op, ins.Arg1, meta)
		}
		return fmt.Sprintf("  %s %s %s%s\n", ins.Op, ins.Result, ins.Arg1, meta)
	}
}
