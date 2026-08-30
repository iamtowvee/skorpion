package backend

import (
	"skrp/res/front"
	"strings"
)

type GCAnalyzer struct {
	Program *front.Program
	IR      *IRProgram
}

func NewGCAnalyzer(prog *front.Program) *GCAnalyzer {
	return &GCAnalyzer{
		Program: prog,
	}
}

func (gc *GCAnalyzer) Analyze(ir *IRProgram) *IRProgram {
	for i := range ir.Functions {
		gc.analyzeFunction(&ir.Functions[i])
	}
	return ir
}

func (gc *GCAnalyzer) analyzeFunction(fn *IRFunction) {
	// Собираем переменные, которые нужно освободить
	variables := gc.collectVariables(fn)

	if len(variables) == 0 {
		return
	}

	// Определяем, какие переменные выделены через malloc/strdup
	allocated := gc.findAllocatedVariables(fn)

	// Для каждой переменной находим последнее использование
	lastUseMap := gc.findLastUse(fn, variables)

	// Вставляем free() только для выделенных переменных
	gc.insertFree(fn, lastUseMap, allocated)
}

func (gc *GCAnalyzer) collectVariables(fn *IRFunction) []string {
	variables := []string{}

	for _, local := range fn.Locals {
		if strings.Contains(local, "sk_string") ||
			strings.Contains(local, "void*") {
			parts := strings.Fields(local)
			if len(parts) >= 2 {
				variables = append(variables, parts[1])
			}
		}
	}

	return variables
}

func (gc *GCAnalyzer) findAllocatedVariables(fn *IRFunction) map[string]bool {
	allocated := make(map[string]bool)

	for _, ins := range fn.Instructions {
		// Проверяем присваивание с strdup или malloc
		if ins.Op == "=" {
			// Если Arg1 содержит strdup или malloc — память выделена
			if strings.Contains(ins.Arg1, "strdup") ||
				strings.Contains(ins.Arg1, "malloc") {
				allocated[ins.Result] = true
			}

			// Если Arg1 — это строковый литерал в кавычках — НЕ выделена
			if strings.HasPrefix(ins.Arg1, "\"") && strings.HasSuffix(ins.Arg1, "\"") {
				allocated[ins.Result] = false
			}
		}

		// Если результат функции — strdup/malloc (из processCall)
		if ins.Op == "call" && ins.Result != "" {
			if strings.Contains(ins.Arg1, "strdup") ||
				strings.Contains(ins.Arg1, "malloc") ||
				strings.Contains(ins.Arg1, "input") { // input возвращает strdup
				allocated[ins.Result] = true
			}
		}
	}

	return allocated
}

func (gc *GCAnalyzer) findLastUse(fn *IRFunction, variables []string) map[string]int {
	lastUse := make(map[string]int)

	for _, v := range variables {
		lastUse[v] = -1
	}

	for idx, ins := range fn.Instructions {
		for _, v := range variables {
			if gc.usesVariable(&ins, v) {
				lastUse[v] = idx
			}
		}
	}

	return lastUse
}

func (gc *GCAnalyzer) usesVariable(ins *IRInstruction, varName string) bool {
	if ins.Result == varName {
		return true
	}
	if ins.Arg1 == varName {
		return true
	}
	if ins.Arg2 == varName {
		return true
	}
	if strings.Contains(ins.Arg1, varName) {
		return true
	}
	return false
}

func (gc *GCAnalyzer) insertFree(fn *IRFunction, lastUseMap map[string]int, allocated map[string]bool) {
	type varLastUse struct {
		name string
		idx  int
	}

	vars := []varLastUse{}
	for name, idx := range lastUseMap {
		// Только для выделенных переменных и если есть использование
		if idx >= 0 && allocated[name] {
			vars = append(vars, varLastUse{name: name, idx: idx})
		}
	}

	// Сортируем по убыванию индекса
	for i := 0; i < len(vars); i++ {
		for j := i + 1; j < len(vars); j++ {
			if vars[i].idx < vars[j].idx {
				vars[i], vars[j] = vars[j], vars[i]
			}
		}
	}

	// Вставляем free()
	for _, v := range vars {
		idx := v.idx

		// Проверяем, не находится ли уже free
		hasFreeAfter := false
		for j := idx + 1; j < len(fn.Instructions); j++ {
			if fn.Instructions[j].Op == "free" && fn.Instructions[j].Arg1 == v.name {
				hasFreeAfter = true
				break
			}
		}

		if !hasFreeAfter {
			// Вставляем free ПОСЛЕ последнего использования
			newIns := IRInstruction{
				Op:   "free",
				Arg1: v.name,
			}

			if idx < len(fn.Instructions)-1 {
				fn.Instructions = append(fn.Instructions[:idx+1], append([]IRInstruction{newIns}, fn.Instructions[idx+1:]...)...)
			} else {
				fn.Instructions = append(fn.Instructions, newIns)
			}
		}
	}
}
