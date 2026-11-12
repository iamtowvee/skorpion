package backend

import "skrp/res/front"

// ============================================================================
// IR — семантическое промежуточное представление Skorpion.
//
// КЛЮЧЕВОЙ ПРИНЦИП: pipeline НЕ ЗНАЕТ про C.
// Никаких имён C-функций (sk_int_add), никаких C-макросов (SK_NULL_int),
// никаких C-выражений ("t1.value") в IR-инструкциях не должно быть.
// Только:
//   - имена IR-переменных ("t1", "foo"),
//   - Skorpion-типы ("int", "string", "T<int,string>"),
//   - литералы ("5", "hello", true).
//
// Backend (codegen.go, llvm.go, ...) — единственный, кто знает про C.
// Чтобы перейти на LLVM — нужно написать только новый backend.go,
// pipeline.go и ir.go не трогаются.
//
// Три уровня:
//   - IRValue — переменная + её Skorpion-тип.
//   - IRInstruction — одна операция над IR-переменными.
//   - IRFunction / IRProgram — контейнеры.
// ============================================================================

// IRValue — значение в IR: имя переменной + её тип.
//
//	IRValue{Type: "int", Var: "t1"}  — int-переменная t1
//	IRValue{Type: "string", Var: "s"} — string-переменная s
//
// Для литералов и констант Var указывает на IR-переменную,
// в которую литерал был материализован (OpConstInt и т.п.).
type IRValue struct {
	Type string
	Var  string
}

// Opcode — семантическая операция IR.
//
// Имена opcodes стабильны; backend обязан обрабатывать ВСЕ.
// Порядок групп — по смыслу (арифметика, сравнения, ...).
type Opcode string

const (
	// --- Литералы / константы ---
	OpConstInt    Opcode = "const_int"    // Result = Arg1 (int)
	OpConstFloat  Opcode = "const_float"  // Result = Arg1 (float)
	OpConstDouble Opcode = "const_double" // Result = Arg1 (double)
	OpConstString Opcode = "const_string" // Result = Arg1 (string literal)
	OpConstChar   Opcode = "const_char"   // Result = Arg1 (char codepoint)
	OpConstBool   Opcode = "const_bool"   // Result = Arg1 ("true"/"false")
	OpConstNull   Opcode = "const_null"   // Result = null of ReturnType
	OpCopy        Opcode = "copy"         // Result = Arg1 (deep copy)

	// --- GC (compile-time free) ---
	OpFreeString Opcode = "free_string" // free(Arg1.value)
	OpFreeArray  Opcode = "free_array"  // sk_array_deep_free(Arg1.value)
	OpFreeDict   Opcode = "free_dict"   // sk_dict_deep_free(Arg1.value)

	// --- Присваивание ---
	OpAssign Opcode = "assign" // Result = Arg1

	// --- Арифметика (ReturnType решает, int/float/double/any) ---
	OpAdd Opcode = "add" // Result = Arg1 + Arg2
	OpSub Opcode = "sub"
	OpMul Opcode = "mul"
	OpDiv Opcode = "div"
	OpMod Opcode = "mod" // только int
	OpPow Opcode = "pow"
	OpNeg Opcode = "neg" // Result = -Arg1

	// --- Сравнения ---
	OpEq Opcode = "eq"
	OpNe Opcode = "ne"
	OpLt Opcode = "lt"
	OpLe Opcode = "le"
	OpGt Opcode = "gt"
	OpGe Opcode = "ge"

	// --- Логика ---
	OpAnd Opcode = "and" // bool
	OpOr  Opcode = "or"
	OpNot Opcode = "not"

	// --- Строки ---
	OpConcat    Opcode = "concat"     // string + string
	OpConcatCC  Opcode = "concat_cc"  // char + char → string
	OpConcatCS  Opcode = "concat_cs"  // char + string → string
	OpConcatSC  Opcode = "concat_sc"  // string + char → string
	OpConcatAll Opcode = "concat_all" // any_to_string(a) + any_to_string(b)

	// --- Конверсии (ReturnType — целевой тип) ---
	OpCast Opcode = "cast" // Result = (ReturnType) Arg1

	// --- any (упаковка/распаковка) ---
	OpWrapAny   Opcode = "wrap_any"   // Result = any(Arg1)
	OpUnwrapAny Opcode = "unwrap_any" // Result = (ReturnType) from any(Arg1)
	OpTypeOfAny Opcode = "typeof_any" // Result = string ("int", "string", ...)

	// --- Массивы ---
	OpArrayNew  Opcode = "array_new"  // Result = new arr[ElemType] (Arg1 = ElemType)
	OpArrayPush Opcode = "array_push" // Result.value[] = Arg2 (Arg1 = arr, Arg2 = elem)
	OpArrayGet  Opcode = "array_get"  // Result = Arg1[Arg2] (по ElemType)
	OpArraySet  Opcode = "array_set"  // Arg1[Arg2] = Arg3
	OpArrayLen  Opcode = "array_len"  // Result = len(Arg1)
	OpRangeNew  Opcode = "range_new"  // Result = arr[int] из [Arg1..Arg2]

	// --- Словари ---
	OpDictNew Opcode = "dict_new" // Result = new empty dict
	OpDictSet Opcode = "dict_set" // Arg1[Arg2] = Arg3
	OpDictGet Opcode = "dict_get" // Result = Arg1[Arg2] (always any)

	// --- Вызов функции ---
	OpCall Opcode = "call" // Result = Arg1(Arg2...) — Arg1 = имя, Arg2 = "a, b, c"

	// --- Управление потоком ---
	OpLabel Opcode = "label" // Result = имя метки
	OpGoto  Opcode = "goto"  // Arg1 = метка
	OpIf    Opcode = "if"    // Arg1 = then-label, Arg2 = else-label, Result = условие
	OpRet   Opcode = "ret"   // Arg1 = значение (или пусто)

	// --- Форматирование (^Nf / ^X...) ---
	OpFormatPrec  Opcode = "format_prec"  // Result = Arg1 с точностью Arg2 (int)
	OpFormatTrim  Opcode = "format_trim"  // Result = Arg1, обрезать Arg2 (TrimSet-строка)
	OpFormatTrunc Opcode = "format_trunc" // Result = int(Arg1)

	// --- IncludeC ---
	OpInlineC Opcode = "inline_c" // Arg1 = raw C code (ЕДИНСТВЕННОЕ место, где C протекает)

	// --- Ошибки / try ---
	OpThrow       Opcode = "throw"         // Arg1 = error instance
	OpErrorNew    Opcode = "error_new"     // Result = new ErrorType (Arg1 = полное имя)
	OpErrorType   Opcode = "error_type"    // Result = string (полное имя типа ошибки)
	OpFieldGet    Opcode = "field_get"     // Result = Arg1.Arg2
	OpFieldSet    Opcode = "field_set"     // Arg1.Arg2 = Arg3
	OpTryPush     Opcode = "try_push"      // Result = frame, Arg1 = catch-label
	OpTryPop      Opcode = "try_pop"       // Arg1 = frame
	OpTryGetError Opcode = "try_get_error" // Result = frame.error, Arg1 = frame
	OpTryRegister Opcode = "try_register"  // Arg1 = var, Arg2 = kind (0=string/any, 1=arr)
	OpErrorMatch  Opcode = "error_match"   // Result = bool, Arg1 = err, Arg2 = typeName
	OpErrorCast   Opcode = "error_cast"    // Result = (Type*)Arg1
	OpErrorFree   Opcode = "error_free"    // free(Arg1)

	// --- Async / Promise ---
	OpPromiseNew        Opcode = "promise_new"         // Result = new promise (Arg1 = Skorpion-тип)
	OpPromiseSet        Opcode = "promise_set"         // Arg1.promise = Arg2 (start value)
	OpPromiseFinish     Opcode = "promise_finish"      // Arg1.promise = Arg2 (final value)
	OpPromiseFinishVoid Opcode = "promise_finish_void" // Arg1 = promise
	OpPromiseFail       Opcode = "promise_fail"        // Arg1.promise = Arg2 (error)
	OpPromiseRead       Opcode = "promise_read"        // Result = promise.value (Arg1 = promise)
	OpPromiseAwait      Opcode = "promise_await"       // Result = await Arg1 (skType = ReturnType)
	OpPromiseAwaitVoid  Opcode = "promise_await_void"  // await Arg1 (void)
	OpPromiseEnded      Opcode = "promise_ended"       // Result = bool (Arg1 = promise)
	OpPromiseRelease    Opcode = "promise_release"     // release Arg1
	OpAsyncSpawn        Opcode = "async_spawn"         // Arg1 = funcName, Arg2 = args, Result = promise
)

// IRInstruction — одна инструкция IR.
//
// Соглашения:
//   - Result — имя переменной, куда положить результат ("" если нет результата).
//   - Arg1/Arg2/Arg3 — имена переменных ИЛИ литералы (для OpConst*).
//   - ReturnType — Skorpion-тип результата (для OpCast — целевой тип).
//   - Metadata — для редких случаев (TrimSet для OpFormatTrim, ElemType для OpArrayNew).
type IRInstruction struct {
	Op         Opcode
	Result     string
	Arg1       string
	Arg2       string
	Arg3       string
	ReturnType string
	SrcType    string
	Line       int
	Column     int
	File       string

	// Metadata — доп. параметры, специфичные для opcode.
	// OpArrayNew: "int", "string", "any", "arr[int]" — тип элемента.
	// OpFormatTrim: набор символов для TrimSet.
	// OpTryRegister: "0" или "1" (kind).
	// OpPromiseNew: Skorpion-тип promise.
	Metadata string
}

// IRParam — параметр функции.
type IRParam struct {
	Name string
	Type string
}

// IRFunction — одна функция в IR.
type IRFunction struct {
	Name           string
	CName          string // legacy: сохраняется для codegen, но pipeline его не заполняет смысловым значением
	ReturnType     string
	IsExport       bool
	IsAsync        bool
	File           string
	Params         []IRParam
	Locals         []IRLocal
	ArrayElemTypes map[string]string            // имя arr-переменной → тип элемента
	VarTypes       map[string]string            // имя переменной → Skorpion-тип
	DictKeyTypes   map[string]map[string]string // имя dict-переменной → {key → type}
	Instructions   []IRInstruction
}

// IRLocal — локальная переменная: тип + имя.
type IRLocal struct {
	Type string
	Name string
}

// IRGlobal — глобальная константа.
//
// В C нет константных выражений для sk_*_new(...), поэтому глобалы
// разворачиваются в статические структуры + brace-init.
// IRGlobalInit — типизированное дерево, из которого backend рендерит
// prelude (static-объекты) и инициализатор.
type IRGlobal struct {
	Name    string
	Type    string
	Value   IRGlobalInit
	IsConst bool
}

// IRGlobalInit — инициализатор глобальной переменной.
type IRGlobalInit struct {
	Kind     GlobalInitKind
	IntVal   int64
	FloatVal string // строка, чтобы не терять точность
	StrVal   string
	BoolVal  bool
	CharVal  byte
	Null     bool

	// Для массивов.
	ElemType string
	Elements []IRGlobalInit

	// Для словарей.
	Entries []IRGlobalEntry

	// Для вложенных (arr[arr[...]] и т.п.) — уникальное имя prelude-объекта.
	PreludeName string
}

type GlobalInitKind int

const (
	GlobalInt GlobalInitKind = iota
	GlobalFloat
	GlobalDouble
	GlobalString
	GlobalBool
	GlobalChar
	GlobalNull
	GlobalArray
	GlobalDict
	GlobalAny
)

type IRGlobalEntry struct {
	Key   string
	Value IRGlobalInit
}

// IRProgram — вся программа в IR.
type IRProgram struct {
	Functions  []IRFunction
	Globals    []IRGlobal
	InlineC    string
	ErrorDecls []IRErrorDecl
	Config     *front.Config
}

type IRErrorDecl struct {
	Name   string
	Parent string
	Fields []IRErrorField
	Module string
}

type IRErrorField struct {
	Name         string
	Type         string
	DefaultValue string
}

// ============================================================================
// Хелперы: типизированные инструкции
// ============================================================================

// NewConstInt создаёт инструкцию «Result = N (int)».
func NewConstInt(result string, v int64) IRInstruction {
	return IRInstruction{Op: OpConstInt, Result: result, Arg1: itoa(v), ReturnType: "int"}
}

// NewConstFloat создаёт инструкцию «Result = V (float)».
func NewConstFloat(result, v string) IRInstruction {
	return IRInstruction{Op: OpConstFloat, Result: result, Arg1: v, ReturnType: "float"}
}

// NewConstDouble — то же для double.
func NewConstDouble(result, v string) IRInstruction {
	return IRInstruction{Op: OpConstDouble, Result: result, Arg1: v, ReturnType: "double"}
}

// NewConstString — «Result = "s" (string)».
func NewConstString(result, s string) IRInstruction {
	return IRInstruction{Op: OpConstString, Result: result, Arg1: s, ReturnType: "string"}
}

// NewConstChar — «Result = 'c' (char)».
func NewConstChar(result string, cp uint32) IRInstruction {
	return IRInstruction{Op: OpConstChar, Result: result, Arg1: uitoa(cp), ReturnType: "char"}
}

// NewConstBool — «Result = true/false (bool)».
func NewConstBool(result string, v bool) IRInstruction {
	lit := "false"
	if v {
		lit = "true"
	}
	return IRInstruction{Op: OpConstBool, Result: result, Arg1: lit, ReturnType: "bool"}
}

// NewConstNull — «Result = null (typ)».
func NewConstNull(result, typ string) IRInstruction {
	return IRInstruction{Op: OpConstNull, Result: result, ReturnType: typ}
}

// itoa — локальный (не тянем strconv в каждый файл).
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func uitoa(n uint32) string {
	return itoa(int64(n))
}
