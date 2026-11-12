package backend

import (
	"sort"
	"strings"

	"skrp/res/front"
)

// ============================================================================
// GC Analyzer — умный compile-time GC на основе free()
// ============================================================================

type GCAnalyzer struct {
	Program *front.Program
	IR      *IRProgram
}

func NewGCAnalyzer(prog *front.Program) *GCAnalyzer {
	return &GCAnalyzer{Program: prog}
}

func (gc *GCAnalyzer) Analyze(ir *IRProgram) *IRProgram {
	for i := range ir.Functions {
		gc.analyzeFunction(&ir.Functions[i])
	}
	return ir
}

// ============================================================================
// Основной анализ функции
// ============================================================================

func (gc *GCAnalyzer) analyzeFunction(fn *IRFunction) {
	variables := gc.collectVariables(fn)
	if len(variables) == 0 {
		return
	}

	owners := gc.findOwners(fn, variables)

	refVars := []string{}
	for _, v := range variables {
		if owners[v] {
			refVars = append(refVars, v)
		}
	}
	if len(refVars) == 0 {
		return
	}

	cfg := gc.buildCFG(fn)
	lifetimes := gc.analyzeLifetimes(fn, cfg, refVars)
	cycles := gc.detectCycles(fn, refVars)
	gc.insertFrees(fn, cfg, lifetimes, cycles, refVars)
}

// ============================================================================
// 1. Сбор переменных (по IRLocal.Type)
// ============================================================================

func (gc *GCAnalyzer) collectVariables(fn *IRFunction) []string {
	variables := []string{}

	for _, local := range fn.Locals {
		if gc.isHeapType(local.Type) {
			variables = append(variables, local.Name)
		}
	}

	return variables
}

// isHeapType — какие Skorpion-типы владеют heap-памятью.
func (gc *GCAnalyzer) isHeapType(t string) bool {
	if strings.Contains(t, "string") {
		return true
	}
	if strings.Contains(t, "arr") {
		return true
	}
	if strings.Contains(t, "dict") {
		return true
	}
	if strings.Contains(t, "promise") {
		return true
	}
	if strings.Contains(t, "error_ptr") {
		return true
	}
	return false
}

// ============================================================================
// 2. Определение владельцев памяти
// ============================================================================

func (gc *GCAnalyzer) findOwners(fn *IRFunction, variables []string) map[string]bool {
	allocated := make(map[string]bool)
	for _, ins := range fn.Instructions {
		// Литералы — не аллокации (C-константы).
		// Реальные аллокации:
		//   OpConstString, OpConstNull, OpArrayNew, OpDictNew,
		//   OpConcat*, OpCall (если callAllocates), OpPromiseNew,
		//   OpCopy (глубокая копия — новая память), OpAsyncSpawn
		if ins.Result == "" {
			continue
		}
		switch ins.Op {
		case OpConstString, OpConstNull, OpArrayNew, OpDictNew,
			OpConcat, OpConcatCC, OpConcatCS, OpConcatSC, OpConcatAll,
			OpPromiseNew, OpCopy, OpAsyncSpawn:
			allocated[ins.Result] = true
		case OpCall:
			if gc.callAllocates(ins.Arg1) {
				allocated[ins.Result] = true
			}
		case OpCast:
			// Cast в string — новая память (char_to_string, int_to_string, ...).
			if ins.ReturnType == "string" {
				allocated[ins.Result] = true
			}
		case OpWrapAny:
			// sk_any владеет памятью, если это string/arr/dict.
			if t := ins.Metadata; t == "string" || strings.Contains(t, "arr") || strings.Contains(t, "dict") {
				allocated[ins.Result] = true
			}
		}
	}

	// Распространение владения через OpAssign / OpCopy.
	changed := true
	for changed {
		changed = false
		for _, ins := range fn.Instructions {
			if ins.Op != OpAssign && ins.Op != OpCopy {
				continue
			}
			if ins.Arg1 == "" || ins.Result == "" {
				continue
			}
			if allocated[ins.Arg1] && !allocated[ins.Result] {
				allocated[ins.Result] = true
				changed = true
			}
		}
	}

	// Снимаем владение с тех, кто передал дальше.
	moved := make(map[string]bool)
	for _, ins := range fn.Instructions {
		if (ins.Op == OpAssign || ins.Op == OpCopy) &&
			ins.Arg1 != "" && ins.Result != "" && ins.Result != ins.Arg1 {
			moved[ins.Arg1] = true
		}
		if ins.Op == OpFieldSet && ins.Arg3 != "" {
			moved[ins.Arg3] = true
		}
	}

	// Передача владения в Promise / return.
	for _, ins := range fn.Instructions {
		switch ins.Op {
		case OpPromiseSet, OpPromiseFinish:
			if ins.Arg2 != "" {
				moved[ins.Arg2] = true
			}
		case OpRet:
			if ins.Arg1 != "" {
				moved[ins.Arg1] = true
			}
		}
	}

	owners := make(map[string]bool)
	for _, v := range variables {
		if allocated[v] && !moved[v] {
			owners[v] = true
		}
	}

	return owners
}

func (gc *GCAnalyzer) callAllocates(funcName string) bool {
	allocators := []string{
		"strdup", "malloc", "calloc", "realloc",
		"input", "input_prompt", "to_string",
		"to_string_float", "to_string_double",
		"concat", "substr",
		"sk_array_new", "sk_array_copy", "sk_array_deep_copy",
		"any_to_string", "any_to_arr", "any_to_dict",
		"sk_int_to_string", "sk_float_to_string",
		"sk_double_to_string", "sk_bool_to_string",
		"sk_string_to_string", "sk_arr_to_string",
		"sk_array_to_string", "sk_string_concat",
		"sk_string_new", "sk_range_new",
		"sk_char_to_string", "sk_char_add_char",
		"sk_char_add_string", "sk_string_add_char",
		"__sk__utf8__change_case",
		"__sk__std_io_input", "__sk__input", "__sk__concat",
		"__sk__toLowerCase", "__sk__toUpperCase",
		"__sk__toCapitalCase", "__sk__toString",
		"__sk__toStringStrict",
	}
	for _, a := range allocators {
		if strings.Contains(funcName, a) {
			return true
		}
	}
	return false
}

// ============================================================================
// 3. Построение CFG
// ============================================================================

type BasicBlock struct {
	ID           int
	Start        int
	End          int
	Successors   []int
	Predecessors []int
	IsReturn     bool
}

type CFG struct {
	Blocks     []*BasicBlock
	LabelToBlk map[string]int
	BlkToLabel map[int]string
}

func (gc *GCAnalyzer) buildCFG(fn *IRFunction) *CFG {
	cfg := &CFG{
		Blocks:     []*BasicBlock{},
		LabelToBlk: make(map[string]int),
		BlkToLabel: make(map[int]string),
	}

	leaders := make(map[int]bool)
	leaders[0] = true

	for i, ins := range fn.Instructions {
		switch ins.Op {
		case OpLabel:
			leaders[i] = true
			cfg.LabelToBlk[ins.Result] = -1
		case OpIf:
			if i+1 < len(fn.Instructions) {
				leaders[i+1] = true
			}
			leaders[gc.findLabel(fn, ins.Arg1)] = true
			leaders[gc.findLabel(fn, ins.Arg2)] = true
		case OpGoto:
			if i+1 < len(fn.Instructions) {
				leaders[i+1] = true
			}
			leaders[gc.findLabel(fn, ins.Arg1)] = true
		case OpRet:
			if i+1 < len(fn.Instructions) {
				leaders[i+1] = true
			}
		}
	}

	sortedLeaders := []int{}
	for l := range leaders {
		sortedLeaders = append(sortedLeaders, l)
	}
	sort.Ints(sortedLeaders)

	for i, start := range sortedLeaders {
		end := len(fn.Instructions) - 1
		if i+1 < len(sortedLeaders) {
			end = sortedLeaders[i+1] - 1
		}

		blk := &BasicBlock{ID: i, Start: start, End: end}

		if end >= start && fn.Instructions[end].Op == OpRet {
			blk.IsReturn = true
		}

		cfg.Blocks = append(cfg.Blocks, blk)

		if start < len(fn.Instructions) && fn.Instructions[start].Op == OpLabel {
			label := fn.Instructions[start].Result
			cfg.LabelToBlk[label] = i
			cfg.BlkToLabel[i] = label
		}
	}

	for i, blk := range cfg.Blocks {
		lastIns := fn.Instructions[blk.End]

		switch lastIns.Op {
		case OpRet:
			// нет successors
		case OpGoto:
			if target, ok := cfg.LabelToBlk[lastIns.Arg1]; ok {
				cfg.addEdge(i, target)
			}
		case OpIf:
			if target, ok := cfg.LabelToBlk[lastIns.Arg1]; ok {
				cfg.addEdge(i, target)
			}
			if target, ok := cfg.LabelToBlk[lastIns.Arg2]; ok {
				cfg.addEdge(i, target)
			}
		default:
			if i+1 < len(cfg.Blocks) {
				cfg.addEdge(i, i+1)
			}
		}
	}

	return cfg
}

func (gc *GCAnalyzer) findLabel(fn *IRFunction, label string) int {
	for i, ins := range fn.Instructions {
		if ins.Op == OpLabel && ins.Result == label {
			return i
		}
	}
	return 0
}

func (cfg *CFG) addEdge(from, to int) {
	if from < 0 || from >= len(cfg.Blocks) {
		return
	}
	if to < 0 || to >= len(cfg.Blocks) {
		return
	}

	for _, s := range cfg.Blocks[from].Successors {
		if s == to {
			return
		}
	}

	cfg.Blocks[from].Successors = append(cfg.Blocks[from].Successors, to)
	cfg.Blocks[to].Predecessors = append(cfg.Blocks[to].Predecessors, from)
}

// ============================================================================
// 4. Анализ жизни
// ============================================================================

type Lifetime struct {
	Var       string
	LastBlock int
	LastInst  int
	InLoop    bool
	InBranch  bool
	Returned  bool
	NumUses   int
}

type gcInsertion struct {
	afterInst int
	varName   string
}

func (gc *GCAnalyzer) analyzeLifetimes(fn *IRFunction, cfg *CFG, vars []string) map[string]*Lifetime {
	lifetimes := make(map[string]*Lifetime)
	for _, v := range vars {
		lifetimes[v] = &Lifetime{Var: v, LastBlock: -1, LastInst: -1}
	}

	for i, ins := range fn.Instructions {
		for _, v := range vars {
			if gc.usesVariable(&ins, v) {
				lt := lifetimes[v]
				lt.NumUses++
				if i > lt.LastInst {
					lt.LastInst = i
					lt.LastBlock = gc.findBlockForInst(cfg, i)
				}
			}
		}
	}

	for _, v := range vars {
		lt := lifetimes[v]
		if lt.LastBlock < 0 {
			continue
		}
		if gc.isInLoop(cfg, lt.LastBlock) {
			lt.InLoop = true
		}
		if len(cfg.Blocks[lt.LastBlock].Predecessors) > 1 {
			lt.InBranch = true
		}
	}

	for _, ins := range fn.Instructions {
		if ins.Op == OpRet && ins.Arg1 != "" {
			for _, v := range vars {
				if ins.Arg1 == v {
					lifetimes[v].Returned = true
				}
			}
		}
	}

	return lifetimes
}

func (gc *GCAnalyzer) findBlockForInst(cfg *CFG, instIdx int) int {
	for _, blk := range cfg.Blocks {
		if instIdx >= blk.Start && instIdx <= blk.End {
			return blk.ID
		}
	}
	return -1
}

func (gc *GCAnalyzer) isInLoop(cfg *CFG, blockID int) bool {
	visited := make(map[int]bool)
	return gc.dfs(cfg, blockID, blockID, visited, false)
}

func (gc *GCAnalyzer) dfs(cfg *CFG, current, target int, visited map[int]bool, passedThrough bool) bool {
	if visited[current] {
		return false
	}
	visited[current] = true

	for _, succ := range cfg.Blocks[current].Successors {
		if succ == target {
			if passedThrough || current != target {
				return true
			}
		}
		if gc.dfs(cfg, succ, target, visited, true) {
			return true
		}
	}

	return false
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
	if ins.Arg3 == varName {
		return true
	}
	if gc.containsToken(ins.Arg1, varName) {
		return true
	}
	if gc.containsToken(ins.Arg2, varName) {
		return true
	}
	if gc.containsToken(ins.Arg3, varName) {
		return true
	}
	return false
}

func (gc *GCAnalyzer) containsToken(s, token string) bool {
	if s == "" || token == "" {
		return false
	}
	if s == token {
		return true
	}
	idx := 0
	for {
		i := strings.Index(s[idx:], token)
		if i < 0 {
			return false
		}
		i += idx
		leftOK := i == 0 || !isIdentChar(s[i-1])
		rightOK := i+len(token) >= len(s) || !isIdentChar(s[i+len(token)])
		if leftOK && rightOK {
			return true
		}
		idx = i + 1
	}
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}

// ============================================================================
// 5. Циклические ссылки
// ============================================================================

type Cycle struct {
	Vars []string
}

func (gc *GCAnalyzer) detectCycles(fn *IRFunction, vars []string) []*Cycle {
	graph := make(map[string]map[string]bool)
	for _, v := range vars {
		graph[v] = make(map[string]bool)
	}

	for _, ins := range fn.Instructions {
		if (ins.Op == OpAssign || ins.Op == OpCopy) && ins.Result != "" {
			for _, v := range vars {
				if v == ins.Result {
					continue
				}
				if gc.containsToken(ins.Arg1, v) {
					if graph[ins.Result] == nil {
						graph[ins.Result] = make(map[string]bool)
					}
					graph[ins.Result][v] = true
				}
			}
		}
	}

	cycles := []*Cycle{}
	visited := make(map[string]bool)
	stack := make(map[string]bool)
	path := []string{}

	var dfs func(v string)
	dfs = func(v string) {
		if stack[v] {
			cycleStart := -1
			for i, p := range path {
				if p == v {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := &Cycle{Vars: append([]string{}, path[cycleStart:]...)}
				cycles = append(cycles, cycle)
			}
			return
		}
		if visited[v] {
			return
		}

		visited[v] = true
		stack[v] = true
		path = append(path, v)

		for next := range graph[v] {
			dfs(next)
		}

		path = path[:len(path)-1]
		stack[v] = false
	}

	for _, v := range vars {
		dfs(v)
	}

	return cycles
}

// ============================================================================
// 6. Вставка free
// ============================================================================

func (gc *GCAnalyzer) insertFrees(fn *IRFunction, cfg *CFG, lifetimes map[string]*Lifetime, cycles []*Cycle, vars []string) {
	sortedVars := make([]string, 0, len(vars))
	for _, v := range vars {
		if lifetimes[v].LastInst >= 0 {
			sortedVars = append(sortedVars, v)
		}
	}

	cyclicVars := make(map[string]bool)
	for _, c := range cycles {
		for _, v := range c.Vars {
			cyclicVars[v] = true
		}
	}

	insertions := []gcInsertion{}

	for _, v := range sortedVars {
		lt := lifetimes[v]

		if lt.Returned {
			continue
		}

		if lt.InLoop {
			insertions = append(insertions, gcInsertion{
				afterInst: gc.findLoopExit(fn, cfg, lt.LastBlock),
				varName:   v,
			})
			continue
		}

		if lt.InBranch {
			insertions = append(insertions, gcInsertion{
				afterInst: gc.findJoinPoint(fn, cfg, lt.LastBlock),
				varName:   v,
			})
			continue
		}

		insertions = append(insertions, gcInsertion{
			afterInst: lt.LastInst,
			varName:   v,
		})
	}

	sort.Slice(insertions, func(i, j int) bool {
		return insertions[i].afterInst < insertions[j].afterInst
	})

	gc.applyInsertions(fn, insertions, cyclicVars)
}

func (gc *GCAnalyzer) findLoopExit(fn *IRFunction, cfg *CFG, blockID int) int {
	for i := cfg.Blocks[blockID].End + 1; i < len(fn.Instructions); i++ {
		if fn.Instructions[i].Op == OpGoto {
			target := gc.findLabel(fn, fn.Instructions[i].Arg1)
			if target < cfg.Blocks[blockID].Start {
				return i - 1
			}
		}
	}
	return lifetimesFallback(cfg, blockID, fn)
}

func lifetimesFallback(cfg *CFG, blockID int, fn *IRFunction) int {
	if blockID >= 0 && blockID < len(cfg.Blocks) {
		return cfg.Blocks[blockID].End
	}
	return len(fn.Instructions) - 1
}

func (gc *GCAnalyzer) findJoinPoint(fn *IRFunction, cfg *CFG, blockID int) int {
	for i := cfg.Blocks[blockID].End + 1; i < len(fn.Instructions); i++ {
		if fn.Instructions[i].Op == OpLabel {
			blkID := cfg.LabelToBlk[fn.Instructions[i].Result]
			if blkID >= 0 && len(cfg.Blocks[blkID].Predecessors) > 1 {
				return i - 1
			}
		}
	}
	return lifetimesFallback(cfg, blockID, fn)
}

func (gc *GCAnalyzer) applyInsertions(fn *IRFunction, insertions []gcInsertion, cyclicVars map[string]bool) {
	insertAfter := make(map[int][]string)
	for _, ins := range insertions {
		if ins.afterInst < 0 {
			ins.afterInst = 0
		}
		insertAfter[ins.afterInst] = append(insertAfter[ins.afterInst], ins.varName)
	}

	newInstructions := []IRInstruction{}

	for i, ins := range fn.Instructions {
		if vars, ok := insertAfter[i]; ok {
			// Если текущая — OpRet, сначала free, потом return.
			// Если что-то другое — сначала ins, потом free.
			if ins.Op == OpRet {
				for j := len(vars) - 1; j >= 0; j-- {
					v := vars[j]
					gc.emitFree(newInstructions, fn, i, v, cyclicVars)
				}
				newInstructions = append(newInstructions, ins)
				continue
			}
		}
		newInstructions = append(newInstructions, ins)

		if vars, ok := insertAfter[i]; ok {
			for j := len(vars) - 1; j >= 0; j-- {
				v := vars[j]
				gc.emitFree(newInstructions, fn, i, v, cyclicVars)
			}
		}
	}

	fn.Instructions = newInstructions
}

func (gc *GCAnalyzer) emitFree(out []IRInstruction, fn *IRFunction, i int, v string, cyclicVars map[string]bool) []IRInstruction {
	if gc.hasFreeAfter(fn, i, v) {
		return out
	}
	skType := gc.findVarSkType(fn, v)
	if strings.Contains(skType, "promise") {
		suffix := strings.TrimSuffix(strings.TrimPrefix(skType, "sk_promise_"), "*")
		out = append(out, IRInstruction{
			Op:         OpPromiseRelease,
			Arg1:       v,
			ReturnType: suffix,
		})
	} else {
		op := gc.freeOpFor(skType)
		out = append(out, IRInstruction{Op: op, Arg1: v})
	}
	if cyclicVars[v] {
		out = append(out, IRInstruction{Op: OpAssign, Result: v, Arg1: ""})
	}
	return out
}

// freeOpFor — какой opcode освобождения использовать для данного Skorpion-типа.
func (gc *GCAnalyzer) freeOpFor(skType string) Opcode {
	if strings.Contains(skType, "string") {
		return OpFreeString
	}
	if strings.Contains(skType, "dict") {
		return OpFreeDict
	}
	if strings.Contains(skType, "arr") {
		return OpFreeArray
	}
	return OpFreeString
}

func (gc *GCAnalyzer) findVarSkType(fn *IRFunction, name string) string {
	for _, local := range fn.Locals {
		if local.Name == name {
			return local.Type
		}
	}
	return ""
}

func (gc *GCAnalyzer) hasFreeAfter(fn *IRFunction, idx int, varName string) bool {
	for j := idx + 1; j < len(fn.Instructions); j++ {
		if fn.Instructions[j].Arg1 == varName {
			switch fn.Instructions[j].Op {
			case OpFreeString, OpFreeArray, OpFreeDict, OpPromiseRelease:
				return true
			}
		}
	}
	return false
}
