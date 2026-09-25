package backend

import (
	"fmt"
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

// ============================================================================
// Основной анализ функции
// ============================================================================

func (gc *GCAnalyzer) analyzeFunction(fn *IRFunction) {
	// 1. Собираем все ссылочные переменные
	variables := gc.collectVariables(fn)
	if len(variables) == 0 {
		return
	}

	// 2. Определяем, какие переменные выделены через malloc/strdup
	allocated := gc.findAllocatedVariables(fn)

	// Фильтруем — оставляем только те, что реально аллоцируются
	refVars := []string{}
	for _, v := range variables {
		if allocated[v] {
			refVars = append(refVars, v)
		}
	}
	if len(refVars) == 0 {
		return
	}

	// 3. Строим CFG
	cfg := gc.buildCFG(fn)

	// 4. Анализ жизни через CFG
	lifetimes := gc.analyzeLifetimes(fn, cfg, refVars)

	// 5. Обнаруживаем циклические ссылки
	cycles := gc.detectCycles(fn, refVars)

	// 6. Вставляем free в правильных точках
	gc.insertFrees(fn, cfg, lifetimes, cycles, refVars)
}

// ============================================================================
// 1. Сбор переменных
// ============================================================================

func (gc *GCAnalyzer) collectVariables(fn *IRFunction) []string {
	variables := []string{}

	for _, local := range fn.Locals {
		// sk_string* — всегда malloc (strdup)
		// void* — может быть malloc (arr, dict)
		if strings.Contains(local, "sk_string") ||
			strings.Contains(local, "sk_array*") ||
			strings.Contains(local, "void*") {
			parts := strings.Fields(local)
			if len(parts) >= 2 {
				// parts[len-1] — имя переменной (может быть указателем)
				variables = append(variables, parts[len(parts)-1])
			}
		}
	}

	return variables
}

// ============================================================================
// 2. Определение аллоцированных переменных
// ============================================================================

func (gc *GCAnalyzer) findAllocatedVariables(fn *IRFunction) map[string]bool {
	allocated := make(map[string]bool)

	for _, ins := range fn.Instructions {
		// Прямое присваивание strdup/malloc
		if ins.Op == "=" {
			if gc.exprAllocates(ins.Arg1) {
				allocated[ins.Result] = true
			}
			// Литерал — не аллокация
			if strings.HasPrefix(ins.Arg1, "\"") && strings.HasSuffix(ins.Arg1, "\"") {
				allocated[ins.Result] = false
			}
		}

		// Функция возвращает strdup/malloc/input
		if ins.Op == "call" && ins.Result != "" {
			if gc.callAllocates(ins.Arg1) {
				allocated[ins.Result] = true
			}
		}
	}

	return allocated
}

func (gc *GCAnalyzer) exprAllocates(expr string) bool {
	// strdup/malloc явно
	if strings.Contains(expr, "strdup") ||
		strings.Contains(expr, "malloc") ||
		strings.Contains(expr, "calloc") ||
		strings.Contains(expr, "realloc") {
		return true
	}
	return false
}

func (gc *GCAnalyzer) callAllocates(funcName string) bool {
	// Функции, которые возвращают malloc'd память
	allocators := []string{
		"strdup",
		"malloc",
		"calloc",
		"realloc",
		"input",           // io.input() → strdup
		"input_prompt",    // io.input_prompt() → strdup
		"to_string",       // io.to_string() → strdup
		"to_string_float", // и т.д.
		"to_string_double",
		"concat",        // io.concat() → malloc
		"substr",        // io.substr() → malloc
		"sk_array_new",  // массив → malloc
		"sk_array_copy", // копия → malloc
		"any_to_string", // → strdup
	}
	for _, a := range allocators {
		if strings.Contains(funcName, a) {
			return true
		}
	}
	return false
}

// ============================================================================
// 3. Построение CFG (Control Flow Graph)
// ============================================================================

type BasicBlock struct {
	ID           int
	Start        int // индекс первой инструкции
	End          int // индекс последней инструкции (включительно)
	Successors   []int
	Predecessors []int
	IsReturn     bool
}

type CFG struct {
	Blocks     []*BasicBlock
	LabelToBlk map[string]int // label → block ID
	BlkToLabel map[int]string // block ID → label (для генерации)
}

func (gc *GCAnalyzer) buildCFG(fn *IRFunction) *CFG {
	cfg := &CFG{
		Blocks:     []*BasicBlock{},
		LabelToBlk: make(map[string]int),
		BlkToLabel: make(map[int]string),
	}

	// 1. Разбиваем инструкции на basic blocks
	//    Границы: labels, if/goto (начало нового блока), ret
	leaders := make(map[int]bool)
	leaders[0] = true

	for i, ins := range fn.Instructions {
		switch ins.Op {
		case "label":
			leaders[i] = true
			cfg.LabelToBlk[ins.Result] = -1 // заполним позже
		case "if":
			// Следующая инструкция — leader
			if i+1 < len(fn.Instructions) {
				leaders[i+1] = true
			}
			// Целевые labels — leaders
			leaders[gc.findLabel(fn, ins.Arg1)] = true
			leaders[gc.findLabel(fn, ins.Arg2)] = true
		case "goto":
			if i+1 < len(fn.Instructions) {
				leaders[i+1] = true
			}
			leaders[gc.findLabel(fn, ins.Result)] = true
		case "ret":
			if i+1 < len(fn.Instructions) {
				leaders[i+1] = true
			}
		}
	}

	// 2. Сортируем leaders
	sortedLeaders := []int{}
	for l := range leaders {
		sortedLeaders = append(sortedLeaders, l)
	}
	sort.Ints(sortedLeaders)

	// 3. Создаём blocks
	for i, start := range sortedLeaders {
		end := len(fn.Instructions) - 1
		if i+1 < len(sortedLeaders) {
			end = sortedLeaders[i+1] - 1
		}

		blk := &BasicBlock{
			ID:    i,
			Start: start,
			End:   end,
		}

		// Если заканчивается на ret — это return block
		if end >= start && fn.Instructions[end].Op == "ret" {
			blk.IsReturn = true
		}

		cfg.Blocks = append(cfg.Blocks, blk)

		// Если блок начинается с label — запоминаем
		if start < len(fn.Instructions) && fn.Instructions[start].Op == "label" {
			label := fn.Instructions[start].Result
			cfg.LabelToBlk[label] = i
			cfg.BlkToLabel[i] = label
		}
	}

	// 4. Связываем блоки
	for i, blk := range cfg.Blocks {
		lastIns := fn.Instructions[blk.End]

		switch lastIns.Op {
		case "ret":
			// Нет successors
		case "goto":
			// Безусловный переход
			if target, ok := cfg.LabelToBlk[lastIns.Result]; ok {
				cfg.addEdge(i, target)
			}
		case "if":
			// Условный переход: then + else
			if target, ok := cfg.LabelToBlk[lastIns.Arg1]; ok {
				cfg.addEdge(i, target)
			}
			if target, ok := cfg.LabelToBlk[lastIns.Arg2]; ok {
				cfg.addEdge(i, target)
			}
		default:
			// Падение на следующий блок
			if i+1 < len(cfg.Blocks) {
				cfg.addEdge(i, i+1)
			}
		}
	}

	return cfg
}

func (gc *GCAnalyzer) findLabel(fn *IRFunction, label string) int {
	for i, ins := range fn.Instructions {
		if ins.Op == "label" && ins.Result == label {
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

	// Проверяем, нет ли уже такого ребра
	for _, s := range cfg.Blocks[from].Successors {
		if s == to {
			return
		}
	}

	cfg.Blocks[from].Successors = append(cfg.Blocks[from].Successors, to)
	cfg.Blocks[to].Predecessors = append(cfg.Blocks[to].Predecessors, from)
}

// ============================================================================
// 4. Анализ жизни через CFG
// ============================================================================

type Lifetime struct {
	Var       string
	LastBlock int  // в каком блоке последнее использование
	LastInst  int  // индекс инструкции последнего использования
	InLoop    bool // используется внутри цикла
	InBranch  bool // используется только в одной ветке
	Returned  bool // возвращается через return
	NumUses   int
}

type gcInsertion struct {
	afterInst int
	varName   string
}

func (gc *GCAnalyzer) analyzeLifetimes(fn *IRFunction, cfg *CFG, vars []string) map[string]*Lifetime {
	// Инициализация
	lifetimes := make(map[string]*Lifetime)
	for _, v := range vars {
		lifetimes[v] = &Lifetime{
			Var:       v,
			LastBlock: -1,
			LastInst:  -1,
		}
	}

	// 1. Для каждой инструкции — какие переменные используются
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

	// 2. Помечаем использование в циклах и ветках
	for _, v := range vars {
		lt := lifetimes[v]
		if lt.LastBlock < 0 {
			continue
		}

		// Проверяем, есть ли путь от последнего блока назад к нему самому (цикл)
		if gc.isInLoop(cfg, lt.LastBlock) {
			lt.InLoop = true
		}

		// Проверяем, есть ли у последнего блока >1 предшественника (ветвление)
		if len(cfg.Blocks[lt.LastBlock].Predecessors) > 1 {
			lt.InBranch = true
		}
	}

	// 3. Помечаем переменные, которые возвращаются
	for _, ins := range fn.Instructions {
		if ins.Op == "ret" && ins.Arg1 != "" {
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

// isInLoop — есть ли путь от блока назад к нему самому
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
	// Проверяем вхождения как отдельные токены
	if gc.containsToken(ins.Arg1, varName) {
		return true
	}
	if gc.containsToken(ins.Arg2, varName) {
		return true
	}
	return false
}

func (gc *GCAnalyzer) containsToken(s, token string) bool {
	if s == "" || token == "" {
		return false
	}
	// Простая проверка — либо целиком, либо с границами токенов
	if s == token {
		return true
	}
	// Проверяем как подстроку с границами
	idx := 0
	for {
		i := strings.Index(s[idx:], token)
		if i < 0 {
			return false
		}
		i += idx
		// Проверяем границы: до и после — не буквы/цифры/подчёркивания
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
// 5. Обнаружение циклических ссылок
// ============================================================================

type Cycle struct {
	Vars []string // переменные, участвующие в цикле
}

func (gc *GCAnalyzer) detectCycles(fn *IRFunction, vars []string) []*Cycle {
	// Строим граф: A → B, если A присваивается из B (или содержит B)
	graph := make(map[string]map[string]bool)
	for _, v := range vars {
		graph[v] = make(map[string]bool)
	}

	for _, ins := range fn.Instructions {
		if ins.Op == "=" && ins.Result != "" {
			// A = B — A ссылается на B
			for _, v := range vars {
				if v == ins.Result {
					continue
				}
				if gc.containsToken(ins.Arg1, v) {
					graph[ins.Result][v] = true
				}
			}
		}
	}

	// Ищем SCC (Tarjan или простой DFS)
	cycles := []*Cycle{}
	visited := make(map[string]bool)
	stack := make(map[string]bool)
	path := []string{}

	var dfs func(v string)
	dfs = func(v string) {
		if stack[v] {
			// Нашли цикл — извлекаем из стека
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

// findLoopExit — находим инструкцию после выхода из цикла
func (gc *GCAnalyzer) findLoopExit(fn *IRFunction, cfg *CFG, blockID int) int {
	// Простая эвристика: ищем следующий goto, который выходит из цикла
	// или конец функции
	for i := cfg.Blocks[blockID].End + 1; i < len(fn.Instructions); i++ {
		if fn.Instructions[i].Op == "goto" {
			// Проверяем, что это переход назад (выход из цикла)
			target := gc.findLabel(fn, fn.Instructions[i].Result)
			if target < cfg.Blocks[blockID].Start {
				return i - 1
			}
		}
	}
	// Fallback — после последнего использования
	return lifetimesFallback(cfg, blockID, fn)
}

func lifetimesFallback(cfg *CFG, blockID int, fn *IRFunction) int {
	if blockID >= 0 && blockID < len(cfg.Blocks) {
		return cfg.Blocks[blockID].End
	}
	return len(fn.Instructions) - 1
}

// findJoinPoint — находим точку слияния веток
func (gc *GCAnalyzer) findJoinPoint(fn *IRFunction, cfg *CFG, blockID int) int {
	// Ищем блок, который достижим из всех веток
	// Простая эвристика: ищем следующий label после последнего использования
	for i := cfg.Blocks[blockID].End + 1; i < len(fn.Instructions); i++ {
		if fn.Instructions[i].Op == "label" {
			// Проверяем, является ли этот label точкой слияния
			blkID := cfg.LabelToBlk[fn.Instructions[i].Result]
			if blkID >= 0 && len(cfg.Blocks[blkID].Predecessors) > 1 {
				return i - 1
			}
		}
	}
	// Fallback
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
		newInstructions = append(newInstructions, ins)

		if vars, ok := insertAfter[i]; ok {
			for j := len(vars) - 1; j >= 0; j-- {
				v := vars[j]

				if gc.hasFreeAfter(fn, i, v) {
					continue
				}

				newInstructions = append(newInstructions, IRInstruction{
					Op:   "free",
					Arg1: v,
				})

				if cyclicVars[v] {
					newInstructions = append(newInstructions, IRInstruction{
						Op:     "=",
						Result: v,
						Arg1:   "NULL",
					})
				}
			}
		}
	}

	fn.Instructions = newInstructions
}

func (gc *GCAnalyzer) hasFreeAfter(fn *IRFunction, idx int, varName string) bool {
	for j := idx + 1; j < len(fn.Instructions); j++ {
		if fn.Instructions[j].Op == "free" && fn.Instructions[j].Arg1 == varName {
			return true
		}
	}
	return false
}

// ============================================================================
// Вспомогательное — вывод для отладки
// ============================================================================

func (gc *GCAnalyzer) DebugCFG(cfg *CFG) {
	fmt.Println("=== CFG ===")
	for _, blk := range cfg.Blocks {
		fmt.Printf("Block %d: [%d-%d] -> %v (pred: %v)\n",
			blk.ID, blk.Start, blk.End,
			blk.Successors, blk.Predecessors)
	}
}
