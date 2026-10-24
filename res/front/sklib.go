package front

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// .sklib — бинарный формат библиотек Skorpion.
//
// Формат спроектирован так, чтобы его можно было реализовать на любом языке
// без зависимости от encoding/gob. Единственные внешние примитивы:
// LEB128 varint (как в protobuf), UTF-8-строки и gzip (RFC 1952).
//
// ── Структура файла ─────────────────────────────────────────────────────────
//
//   "SKLB"                  4 байта магии
//   formatVersion (1 байт)  должно быть равно SKLibFormatVersion (= 2)
//   gzip(payload)           остальное содержимое, сжатое gzip
//
// ── Payload (после распаковки) ──────────────────────────────────────────────
//
//   formatVersion : uvarint   (должно совпадать с байтом из заголовка)
//   name          : string
//   version       : string
//   author        : string
//   dependencies  : []string
//   hasProgram    : u8        (0 = нет, 1 = есть; иное — ошибка)
//   program       : Node      (только если hasProgram == 1; тег = 1 = Program)
//
// ── Примитивы ───────────────────────────────────────────────────────────────
//
//   u8          один байт, беззнаковый.
//   bool        один байт: 0 = false, 1 = true. Иное — ошибка.
//   uvarint     беззнаковый LEB128: 7 бит на байт, старший бит — continuation.
//               Совпадает с Go encoding/binary.PutUvarint.
//   varint      знаковый zigzag-Leb128: (n << 1) ^ (n >> 63), затем uvarint.
//               Совпадает с Go encoding/binary.PutVarint.
//   string      uvarint (длина в байтах) + UTF-8 байты.
//   []T         uvarint (количество) + T × count.
//   Position    uvarint (line) + uvarint (column).
//   Node        u8 (тег) + payload конкретного типа (см. ниже).
//   ?Node       u8 0 = nil, u8 1 + Node = значение. Иное — ошибка.
//   ?Block      аналогично ?Node, но значение обязано быть Block.
//   ?RangeExpr  аналогично ?Node, но значение обязано быть RangeExpr.
//
// ── Теги узлов ──────────────────────────────────────────────────────────────
//
//   1  Program        Position, []Import, []Function, []ErrorDecl, []string
//   2  Import         Position, string Path, string Alias, bool All, bool IsLib
//   3  Function       Position, Position NamePos, string Name, string ReturnType,
//                     []string ReturnUnion, []Param, Block Body,
//                     bool IsExport, bool IsAsync, string File
//   4  Param          Position, string Name, string Type, string CurrentType,
//                     ?Node DefaultValue, bool IsVariadic
//   5  Block          Position, []Node Statements
//   6  TernaryExpr    Position, Node Cond, Node Then, Node Else
//   7  TypeOf         Position, Node Expr
//   8  ArrayLiteral   Position, []Node Elements
//   9  ArrayIndex     Position, string Name, Node Index
//  10  ArrayLength    Position, string Name
//  11  ArrayAdd       Position, string Name, Node Elem
//  12  DictLiteral    Position, []DictElement Elements
//  13  DictElement    Position, string Key, Node Value
//  14  DictIndex      Position, string Object, Node Key
//  15  VarDecl        Position, string Name, string Type, string CurrentType,
//                     string ElemType, ?Node Expr, bool IsArray, bool IsConst
//  16  Assign         Position, string Name, string Field, ?Node Index,
//                     ?Node Expr, string Op
//  17  BinaryExpr     Position, Node Left, string Op, Node Right
//  18  UnaryExpr      Position, string Op, Node Expr
//  19  RangeExpr      Position, Node Start, Node End
//  20  CallRangeExpr  Position, string Name, ?RangeExpr Range, []Node Extra
//  21  CallExpr       Position, string Name, []Node Args, string Receiver,
//                     ?Node ReceiverNode
//  22  ReturnStmt     Position, ?Node Expr
//  23  Ident          Position, string Name
//  24  Number         Position, string Value
//  25  FormatExpr     Position, Node Expr, string Mode, varint N, string TrimSet
//  26  String         Position, string Value
//  27  CharLiteral    Position, u8 Value
//  28  NullLiteral    Position
//  29  IfStmt         Position, Node Cond, Block Then, []Elsif Elsifs,
//                     ?Block Else
//  30  Elsif          Position, Node Cond, Block Then
//  31  CaseStmt       Position, Node Value, []CaseBranch Branches, ?Block Default
//  32  CaseBranch     Position, []Node Patterns, Block Body
//  33  WhileStmt      Position, Node Cond, Block Body
//  34  ForInStmt      Position, string VarType, string VarName,
//                     Node Iterable, Block Body
//  35  IncludeC       Position, string Code
//  36  IknowIdoBlock  Position, Block Body
//  37  UnicodeLiteral Position, uvarint Codepoint (uint32)
//  38  ErrorDecl      Position, string Name, []ErrorField Fields,
//                     string Parent, bool IsNew, string Module
//  39  ErrorField     Position, string Name, string Type, ?Node DefaultValue
//  40  ErrorInstance  Position, string TypeName,
//                     map<string,Node> Fields (отсортировано по ключу)
//  41  ThrowStmt      Position, Node Expr
//  42  FieldAccess    Position, string Object, string Field
//  43  TryStmt        Position, Block Body, []CatchClause Catches
//  44  CatchClause    Position, string TypeName, string VarName, Block Body
//  45  BreakStmt      Position
//  46  ContinueStmt   Position
//  47  DoWhileStmt    Position, Block Body, Node Cond
//  48  PostfixExpr    Position, string VarName, string Op
//  49  PromiseSet     Position, Node Value
//  50  AwaitExpr      Position, Node Expr
//  51  IsAsyncEnded   Position, string FuncName
//
// ── Замечания по реализации ────────────────────────────────────────────────
//
//  * Поле map<string,Node> кодируется как uvarint count + count пар
//    (string key, Node value), отсортированных по ключу лексикографически.
//    Это обеспечивает детерминизм вывода.
//  * Поле Program.AllFunctions НЕ сериализуется — оно восстанавливается
//    при загрузке (см. ImportManager.LoadMain).
//  * Читатель обязан отвергнуть файл, если после разбора payload остались
//    лишние байты (защита от рассинхронизации версий).
// ─────────────────────────────────────────────────────────────────────────────

// Версия формата .sklib.
//
//   - Версия формата .sklib.
//   - 1 — gob (не поддерживается)
//   - 2 — собственный бинарный формат (без GlobalVars)
//   - 3 — + Program.GlobalVars и VarDecl.IsExport
//
// Увеличивайте при любых несовместимых изменениях структуры.
const SKLibFormatVersion = 3

// sklibMagic — магические байты в начале .sklib.
var sklibMagic = []byte{'S', 'K', 'L', 'B'}

// sklibHeaderSize — 4 байта магии + 1 байт версии формата.
const sklibHeaderSize = 5

// Теги узлов AST. Стабильны в рамках SKLibFormatVersion.
const (
	tagProgram        byte = 1
	tagImport         byte = 2
	tagFunction       byte = 3
	tagParam          byte = 4
	tagBlock          byte = 5
	tagTernaryExpr    byte = 6
	tagTypeOf         byte = 7
	tagArrayLiteral   byte = 8
	tagArrayIndex     byte = 9
	tagArrayLength    byte = 10
	tagArrayAdd       byte = 11
	tagDictLiteral    byte = 12
	tagDictElement    byte = 13
	tagDictIndex      byte = 14
	tagVarDecl        byte = 15
	tagAssign         byte = 16
	tagBinaryExpr     byte = 17
	tagUnaryExpr      byte = 18
	tagRangeExpr      byte = 19
	tagCallRangeExpr  byte = 20
	tagCallExpr       byte = 21
	tagReturnStmt     byte = 22
	tagIdent          byte = 23
	tagNumber         byte = 24
	tagFormatExpr     byte = 25
	tagString         byte = 26
	tagCharLiteral    byte = 27
	tagNullLiteral    byte = 28
	tagIfStmt         byte = 29
	tagElsif          byte = 30
	tagCaseStmt       byte = 31
	tagCaseBranch     byte = 32
	tagWhileStmt      byte = 33
	tagForInStmt      byte = 34
	tagIncludeC       byte = 35
	tagIknowIdoBlock  byte = 36
	tagUnicodeLiteral byte = 37
	tagErrorDecl      byte = 38
	tagErrorField     byte = 39
	tagErrorInstance  byte = 40
	tagThrowStmt      byte = 41
	tagFieldAccess    byte = 42
	tagTryStmt        byte = 43
	tagCatchClause    byte = 44
	tagBreakStmt      byte = 45
	tagContinueStmt   byte = 46
	tagDoWhileStmt    byte = 47
	tagPostfixExpr    byte = 48
	tagPromiseSet     byte = 49
	tagAwaitExpr      byte = 50
	tagIsAsyncEnded   byte = 51
)

// ============================================================================
// Encoder
// ============================================================================

type binWriter struct {
	buf *bytes.Buffer
}

func newBinWriter() *binWriter {
	return &binWriter{buf: &bytes.Buffer{}}
}

func (w *binWriter) u8(b byte) {
	w.buf.WriteByte(b)
}

func (w *binWriter) bool(v bool) {
	if v {
		w.buf.WriteByte(1)
	} else {
		w.buf.WriteByte(0)
	}
}

func (w *binWriter) uvarint(v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	w.buf.Write(tmp[:n])
}

func (w *binWriter) varint(v int64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutVarint(tmp[:], v)
	w.buf.Write(tmp[:n])
}

func (w *binWriter) str(s string) {
	w.uvarint(uint64(len(s)))
	w.buf.WriteString(s)
}

func (w *binWriter) strs(ss []string) {
	w.uvarint(uint64(len(ss)))
	for _, s := range ss {
		w.str(s)
	}
}

func (w *binWriter) pos(p Position) {
	w.uvarint(uint64(p.Line))
	w.uvarint(uint64(p.Column))
}

// nullableNode пишет префикс присутствия (0 = nil, 1 = значение), затем Node.
func (w *binWriter) nullableNode(n Node) error {
	if n == nil {
		w.u8(0)
		return nil
	}
	w.u8(1)
	return w.node(n)
}

// nullableBlock — как nullableNode, но для конкретного *Block.
func (w *binWriter) nullableBlock(b *Block) error {
	if b == nil {
		w.u8(0)
		return nil
	}
	w.u8(1)
	return w.node(b)
}

// node — центральный диспетчер по конкретному Go-типу.
// Возвращает ошибку только для неизвестных типов (баг в новом узле без
// обновления этого switch'а).
func (w *binWriter) node(n Node) error {
	switch v := n.(type) {
	case *Program:
		w.u8(tagProgram)
		return w.program(v)
	case *Import:
		w.u8(tagImport)
		w.pos(v.Position)
		w.str(v.Path)
		w.str(v.Alias)
		w.bool(v.All)
		w.bool(v.IsLib)
		return nil
	case *Function:
		w.u8(tagFunction)
		w.pos(v.Position)
		w.pos(v.NamePos)
		w.str(v.Name)
		w.str(v.ReturnType)
		w.strs(v.ReturnUnion)
		w.uvarint(uint64(len(v.Params)))
		for _, p := range v.Params {
			if err := w.node(p); err != nil {
				return err
			}
		}
		if err := w.node(v.Body); err != nil {
			return err
		}
		w.bool(v.IsExport)
		w.bool(v.IsAsync)
		w.str(v.File)
		return nil
	case *Param:
		w.u8(tagParam)
		w.pos(v.Position)
		w.str(v.Name)
		w.str(v.Type)
		w.str(v.CurrentType)
		if err := w.nullableNode(v.DefaultValue); err != nil {
			return err
		}
		w.bool(v.IsVariadic)
		return nil
	case *Block:
		w.u8(tagBlock)
		w.pos(v.Position)
		w.uvarint(uint64(len(v.Statements)))
		for _, s := range v.Statements {
			if err := w.node(s); err != nil {
				return err
			}
		}
		return nil
	case *TernaryExpr:
		w.u8(tagTernaryExpr)
		w.pos(v.Position)
		if err := w.node(v.Condition); err != nil {
			return err
		}
		if err := w.node(v.Then); err != nil {
			return err
		}
		return w.node(v.Else)
	case *TypeOf:
		w.u8(tagTypeOf)
		w.pos(v.Position)
		return w.node(v.Expr)
	case *ArrayLiteral:
		w.u8(tagArrayLiteral)
		w.pos(v.Position)
		w.uvarint(uint64(len(v.Elements)))
		for _, e := range v.Elements {
			if err := w.node(e); err != nil {
				return err
			}
		}
		return nil
	case *ArrayIndex:
		w.u8(tagArrayIndex)
		w.pos(v.Position)
		w.str(v.Name)
		return w.node(v.Index)
	case *ArrayLength:
		w.u8(tagArrayLength)
		w.pos(v.Position)
		w.str(v.Name)
		return nil
	case *ArrayAdd:
		w.u8(tagArrayAdd)
		w.pos(v.Position)
		w.str(v.Name)
		return w.node(v.Elem)
	case *DictLiteral:
		w.u8(tagDictLiteral)
		w.pos(v.Position)
		w.uvarint(uint64(len(v.Elements)))
		for _, e := range v.Elements {
			if err := w.node(e); err != nil {
				return err
			}
		}
		return nil
	case *DictElement:
		w.u8(tagDictElement)
		w.pos(v.Position)
		w.str(v.Key)
		return w.node(v.Value)
	case *DictIndex:
		w.u8(tagDictIndex)
		w.pos(v.Position)
		w.str(v.Object)
		return w.node(v.Key)
	case *VarDecl:
		w.u8(tagVarDecl)
		w.pos(v.Position)
		w.str(v.Name)
		w.str(v.Type)
		w.str(v.CurrentType)
		w.str(v.ElemType)
		if err := w.nullableNode(v.Expr); err != nil {
			return err
		}
		w.bool(v.IsArray)
		w.bool(v.IsConst)
		w.bool(v.IsExport) // NEW (v3)
		return nil
	case *Assign:
		w.u8(tagAssign)
		w.pos(v.Position)
		w.str(v.Name)
		w.str(v.Field)
		if err := w.nullableNode(v.Index); err != nil {
			return err
		}
		if err := w.nullableNode(v.Expr); err != nil {
			return err
		}
		w.str(v.Op)
		return nil
	case *BinaryExpr:
		w.u8(tagBinaryExpr)
		w.pos(v.Position)
		if err := w.node(v.Left); err != nil {
			return err
		}
		w.str(v.Op)
		return w.node(v.Right)
	case *UnaryExpr:
		w.u8(tagUnaryExpr)
		w.pos(v.Position)
		w.str(v.Op)
		return w.node(v.Expr)
	case *RangeExpr:
		w.u8(tagRangeExpr)
		w.pos(v.Position)
		if err := w.node(v.Start); err != nil {
			return err
		}
		return w.node(v.End)
	case *CallRangeExpr:
		w.u8(tagCallRangeExpr)
		w.pos(v.Position)
		w.str(v.Name)
		if v.Range == nil {
			w.u8(0)
		} else {
			w.u8(1)
			if err := w.node(v.Range); err != nil {
				return err
			}
		}
		w.uvarint(uint64(len(v.Extra)))
		for _, e := range v.Extra {
			if err := w.node(e); err != nil {
				return err
			}
		}
		return nil
	case *CallExpr:
		w.u8(tagCallExpr)
		w.pos(v.Position)
		w.str(v.Name)
		w.uvarint(uint64(len(v.Args)))
		for _, a := range v.Args {
			if err := w.node(a); err != nil {
				return err
			}
		}
		w.str(v.Receiver)
		return w.nullableNode(v.ReceiverNode)
	case *ReturnStmt:
		w.u8(tagReturnStmt)
		w.pos(v.Position)
		return w.nullableNode(v.Expr)
	case *Ident:
		w.u8(tagIdent)
		w.pos(v.Position)
		w.str(v.Name)
		return nil
	case *Number:
		w.u8(tagNumber)
		w.pos(v.Position)
		w.str(v.Value)
		return nil
	case *FormatExpr:
		w.u8(tagFormatExpr)
		w.pos(v.Position)
		if err := w.node(v.Expr); err != nil {
			return err
		}
		w.str(v.Mode)
		w.varint(int64(v.N))
		w.str(v.TrimSet)
		return nil
	case *String:
		w.u8(tagString)
		w.pos(v.Position)
		w.str(v.Value)
		return nil
	case *CharLiteral:
		w.u8(tagCharLiteral)
		w.pos(v.Position)
		w.u8(v.Value)
		return nil
	case *NullLiteral:
		w.u8(tagNullLiteral)
		w.pos(v.Position)
		return nil
	case *IfStmt:
		w.u8(tagIfStmt)
		w.pos(v.Position)
		if err := w.node(v.Condition); err != nil {
			return err
		}
		if err := w.node(v.Then); err != nil {
			return err
		}
		w.uvarint(uint64(len(v.Elsifs)))
		for _, e := range v.Elsifs {
			if err := w.node(e); err != nil {
				return err
			}
		}
		return w.nullableBlock(v.Else)
	case *Elsif:
		w.u8(tagElsif)
		w.pos(v.Position)
		if err := w.node(v.Condition); err != nil {
			return err
		}
		return w.node(v.Then)
	case *CaseStmt:
		w.u8(tagCaseStmt)
		w.pos(v.Position)
		if err := w.node(v.Value); err != nil {
			return err
		}
		w.uvarint(uint64(len(v.Branches)))
		for _, b := range v.Branches {
			if err := w.node(b); err != nil {
				return err
			}
		}
		return w.nullableBlock(v.Default)
	case *CaseBranch:
		w.u8(tagCaseBranch)
		w.pos(v.Position)
		w.uvarint(uint64(len(v.Patterns)))
		for _, p := range v.Patterns {
			if err := w.node(p); err != nil {
				return err
			}
		}
		return w.node(v.Body)
	case *WhileStmt:
		w.u8(tagWhileStmt)
		w.pos(v.Position)
		if err := w.node(v.Condition); err != nil {
			return err
		}
		return w.node(v.Body)
	case *ForInStmt:
		w.u8(tagForInStmt)
		w.pos(v.Position)
		w.str(v.VarType)
		w.str(v.VarName)
		if err := w.node(v.Iterable); err != nil {
			return err
		}
		return w.node(v.Body)
	case *IncludeC:
		w.u8(tagIncludeC)
		w.pos(v.Position)
		w.str(v.Code)
		return nil
	case *IknowIdoBlock:
		w.u8(tagIknowIdoBlock)
		w.pos(v.Position)
		return w.node(v.Body)
	case *UnicodeLiteral:
		w.u8(tagUnicodeLiteral)
		w.pos(v.Position)
		w.uvarint(uint64(v.Codepoint))
		return nil
	case *ErrorDecl:
		w.u8(tagErrorDecl)
		w.pos(v.Position)
		w.str(v.Name)
		w.uvarint(uint64(len(v.Fields)))
		for _, f := range v.Fields {
			if err := w.node(f); err != nil {
				return err
			}
		}
		w.str(v.Parent)
		w.bool(v.IsNew)
		w.str(v.Module)
		return nil
	case *ErrorField:
		w.u8(tagErrorField)
		w.pos(v.Position)
		w.str(v.Name)
		w.str(v.Type)
		return w.nullableNode(v.DefaultValue)
	case *ErrorInstance:
		w.u8(tagErrorInstance)
		w.pos(v.Position)
		w.str(v.TypeName)
		keys := make([]string, 0, len(v.Fields))
		for k := range v.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.uvarint(uint64(len(keys)))
		for _, k := range keys {
			w.str(k)
			if err := w.node(v.Fields[k]); err != nil {
				return err
			}
		}
		return nil
	case *ThrowStmt:
		w.u8(tagThrowStmt)
		w.pos(v.Position)
		return w.node(v.Expr)
	case *FieldAccess:
		w.u8(tagFieldAccess)
		w.pos(v.Position)
		w.str(v.Object)
		w.str(v.Field)
		return nil
	case *TryStmt:
		w.u8(tagTryStmt)
		w.pos(v.Position)
		if err := w.node(v.Body); err != nil {
			return err
		}
		w.uvarint(uint64(len(v.Catches)))
		for _, c := range v.Catches {
			if err := w.node(c); err != nil {
				return err
			}
		}
		return nil
	case *CatchClause:
		w.u8(tagCatchClause)
		w.pos(v.Position)
		w.str(v.TypeName)
		w.str(v.VarName)
		return w.node(v.Body)
	case *BreakStmt:
		w.u8(tagBreakStmt)
		w.pos(v.Position)
		return nil
	case *ContinueStmt:
		w.u8(tagContinueStmt)
		w.pos(v.Position)
		return nil
	case *DoWhileStmt:
		w.u8(tagDoWhileStmt)
		w.pos(v.Position)
		if err := w.node(v.Body); err != nil {
			return err
		}
		return w.node(v.Condition)
	case *PostfixExpr:
		w.u8(tagPostfixExpr)
		w.pos(v.Position)
		w.str(v.VarName)
		w.str(v.Op)
		return nil
	case *PromiseSet:
		w.u8(tagPromiseSet)
		w.pos(v.Position)
		return w.node(v.Value)
	case *AwaitExpr:
		w.u8(tagAwaitExpr)
		w.pos(v.Position)
		return w.node(v.Expr)
	case *IsAsyncEnded:
		w.u8(tagIsAsyncEnded)
		w.pos(v.Position)
		w.str(v.FuncName)
		return nil
	default:
		return fmt.Errorf("sklib: cannot encode node of type %T", n)
	}
}

func (w *binWriter) program(p *Program) error {
	w.pos(p.Position)

	w.uvarint(uint64(len(p.Imports)))
	for _, imp := range p.Imports {
		if err := w.node(imp); err != nil {
			return err
		}
	}

	w.uvarint(uint64(len(p.Functions)))
	for _, fn := range p.Functions {
		if err := w.node(fn); err != nil {
			return err
		}
	}

	w.uvarint(uint64(len(p.ErrorDecls)))
	for _, d := range p.ErrorDecls {
		if err := w.node(d); err != nil {
			return err
		}
	}

	w.strs(p.GlobalIncludeC)

	// NEW (v3): GlobalVars — пишем в конец payload, чтобы v2-файлы
	// не были «случайно совместимы».
	w.uvarint(uint64(len(p.GlobalVars)))
	for _, gv := range p.GlobalVars {
		if err := w.node(gv); err != nil {
			return err
		}
	}

	return nil
}

// ============================================================================
// Decoder
// ============================================================================

type binReader struct {
	data []byte
	off  int
}

func newBinReader(data []byte) *binReader {
	return &binReader{data: data}
}

func (r *binReader) u8() (byte, error) {
	if r.off >= len(r.data) {
		return 0, fmt.Errorf("unexpected end of data at byte %d", r.off)
	}
	b := r.data[r.off]
	r.off++
	return b, nil
}

func (r *binReader) bool() (bool, error) {
	b, err := r.u8()
	if err != nil {
		return false, err
	}
	if b == 0 {
		return false, nil
	}
	if b == 1 {
		return true, nil
	}
	return false, fmt.Errorf("invalid bool byte 0x%02x at byte %d", b, r.off-1)
}

func (r *binReader) uvarint() (uint64, error) {
	v, n := binary.Uvarint(r.data[r.off:])
	if n == 0 {
		return 0, fmt.Errorf("truncated uvarint at byte %d", r.off)
	}
	if n < 0 {
		return 0, fmt.Errorf("uvarint overflow at byte %d", r.off)
	}
	r.off += n
	return v, nil
}

func (r *binReader) varint() (int64, error) {
	v, n := binary.Varint(r.data[r.off:])
	if n == 0 {
		return 0, fmt.Errorf("truncated varint at byte %d", r.off)
	}
	if n < 0 {
		return 0, fmt.Errorf("varint overflow at byte %d", r.off)
	}
	r.off += n
	return v, nil
}

func (r *binReader) str() (string, error) {
	n, err := r.uvarint()
	if err != nil {
		return "", err
	}
	if n > uint64(len(r.data)-r.off) {
		return "", fmt.Errorf(
			"string length %d exceeds remaining %d bytes at byte %d",
			n, len(r.data)-r.off, r.off)
	}
	s := string(r.data[r.off : r.off+int(n)])
	r.off += int(n)
	return s, nil
}

func (r *binReader) count() (int, error) {
	n, err := r.uvarint()
	if err != nil {
		return 0, err
	}
	if n > uint64(len(r.data)-r.off) {
		return 0, fmt.Errorf(
			"count %d exceeds remaining %d bytes at byte %d",
			n, len(r.data)-r.off, r.off)
	}
	return int(n), nil
}

func (r *binReader) strs() ([]string, error) {
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	ss := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s, err := r.str()
		if err != nil {
			return nil, err
		}
		ss = append(ss, s)
	}
	return ss, nil
}

func (r *binReader) readPos() (Position, error) {
	line, err := r.uvarint()
	if err != nil {
		return Position{}, err
	}
	col, err := r.uvarint()
	if err != nil {
		return Position{}, err
	}
	return Position{Line: int(line), Column: int(col)}, nil
}

func (r *binReader) nullableNode() (Node, error) {
	flag, err := r.u8()
	if err != nil {
		return nil, err
	}
	switch flag {
	case 0:
		return nil, nil
	case 1:
		return r.node()
	default:
		return nil, fmt.Errorf(
			"invalid nullable flag 0x%02x at byte %d", flag, r.off-1)
	}
}

func (r *binReader) nullableBlock() (*Block, error) {
	n, err := r.nullableNode()
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	b, ok := n.(*Block)
	if !ok {
		return nil, fmt.Errorf("expected *Block, got %T", n)
	}
	return b, nil
}

func (r *binReader) requireBlock() (*Block, error) {
	n, err := r.node()
	if err != nil {
		return nil, err
	}
	b, ok := n.(*Block)
	if !ok {
		return nil, fmt.Errorf("expected *Block, got %T", n)
	}
	return b, nil
}

func (r *binReader) requireRange() (*RangeExpr, error) {
	flag, err := r.u8()
	if err != nil {
		return nil, err
	}
	switch flag {
	case 0:
		return nil, nil
	case 1:
		n, err := r.node()
		if err != nil {
			return nil, err
		}
		re, ok := n.(*RangeExpr)
		if !ok {
			return nil, fmt.Errorf("expected *RangeExpr, got %T", n)
		}
		return re, nil
	default:
		return nil, fmt.Errorf(
			"invalid nullable flag 0x%02x at byte %d", flag, r.off-1)
	}
}

// node — диспетчер по тегу.
func (r *binReader) node() (Node, error) {
	tag, err := r.u8()
	if err != nil {
		return nil, err
	}
	switch tag {
	case tagProgram:
		return r.program()
	case tagImport:
		return r.importNode()
	case tagFunction:
		return r.functionNode()
	case tagParam:
		return r.paramNode()
	case tagBlock:
		return r.blockNode()
	case tagTernaryExpr:
		return r.ternaryExprNode()
	case tagTypeOf:
		return r.typeOfNode()
	case tagArrayLiteral:
		return r.arrayLiteralNode()
	case tagArrayIndex:
		return r.arrayIndexNode()
	case tagArrayLength:
		return r.arrayLengthNode()
	case tagArrayAdd:
		return r.arrayAddNode()
	case tagDictLiteral:
		return r.dictLiteralNode()
	case tagDictElement:
		return r.dictElementNode()
	case tagDictIndex:
		return r.dictIndexNode()
	case tagVarDecl:
		return r.varDeclNode()
	case tagAssign:
		return r.assignNode()
	case tagBinaryExpr:
		return r.binaryExprNode()
	case tagUnaryExpr:
		return r.unaryExprNode()
	case tagRangeExpr:
		return r.rangeExprNode()
	case tagCallRangeExpr:
		return r.callRangeExprNode()
	case tagCallExpr:
		return r.callExprNode()
	case tagReturnStmt:
		return r.returnStmtNode()
	case tagIdent:
		return r.identNode()
	case tagNumber:
		return r.numberNode()
	case tagFormatExpr:
		return r.formatExprNode()
	case tagString:
		return r.stringNode()
	case tagCharLiteral:
		return r.charLiteralNode()
	case tagNullLiteral:
		return r.nullLiteralNode()
	case tagIfStmt:
		return r.ifStmtNode()
	case tagElsif:
		return r.elsifNode()
	case tagCaseStmt:
		return r.caseStmtNode()
	case tagCaseBranch:
		return r.caseBranchNode()
	case tagWhileStmt:
		return r.whileStmtNode()
	case tagForInStmt:
		return r.forInStmtNode()
	case tagIncludeC:
		return r.includeCNode()
	case tagIknowIdoBlock:
		return r.iknowIdoBlockNode()
	case tagUnicodeLiteral:
		return r.unicodeLiteralNode()
	case tagErrorDecl:
		return r.errorDeclNode()
	case tagErrorField:
		return r.errorFieldNode()
	case tagErrorInstance:
		return r.errorInstanceNode()
	case tagThrowStmt:
		return r.throwStmtNode()
	case tagFieldAccess:
		return r.fieldAccessNode()
	case tagTryStmt:
		return r.tryStmtNode()
	case tagCatchClause:
		return r.catchClauseNode()
	case tagBreakStmt:
		return r.breakStmtNode()
	case tagContinueStmt:
		return r.continueStmtNode()
	case tagDoWhileStmt:
		return r.doWhileStmtNode()
	case tagPostfixExpr:
		return r.postfixExprNode()
	case tagPromiseSet:
		return r.promiseSetNode()
	case tagAwaitExpr:
		return r.awaitExprNode()
	case tagIsAsyncEnded:
		return r.isAsyncEndedNode()
	default:
		return nil, fmt.Errorf("unknown node tag %d at byte %d", tag, r.off-1)
	}
}

func (r *binReader) program() (*Program, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	p := &Program{Position: pos}

	nImports, err := r.count()
	if err != nil {
		return nil, err
	}
	p.Imports = make([]*Import, 0, nImports)
	for i := 0; i < nImports; i++ {
		n, err := r.node()
		if err != nil {
			return nil, err
		}
		imp, ok := n.(*Import)
		if !ok {
			return nil, fmt.Errorf("program: expected *Import, got %T", n)
		}
		p.Imports = append(p.Imports, imp)
	}

	nFuncs, err := r.count()
	if err != nil {
		return nil, err
	}
	p.Functions = make([]*Function, 0, nFuncs)
	for i := 0; i < nFuncs; i++ {
		n, err := r.node()
		if err != nil {
			return nil, err
		}
		fn, ok := n.(*Function)
		if !ok {
			return nil, fmt.Errorf("program: expected *Function, got %T", n)
		}
		p.Functions = append(p.Functions, fn)
	}

	nErrs, err := r.count()
	if err != nil {
		return nil, err
	}
	p.ErrorDecls = make([]*ErrorDecl, 0, nErrs)
	for i := 0; i < nErrs; i++ {
		n, err := r.node()
		if err != nil {
			return nil, err
		}
		ed, ok := n.(*ErrorDecl)
		if !ok {
			return nil, fmt.Errorf("program: expected *ErrorDecl, got %T", n)
		}
		p.ErrorDecls = append(p.ErrorDecls, ed)
	}

	p.GlobalIncludeC, err = r.strs()
	if err != nil {
		return nil, err
	}

	// NEW (v3): GlobalVars
	nVars, err := r.count()
	if err != nil {
		return nil, err
	}
	p.GlobalVars = make([]*VarDecl, 0, nVars)
	for i := 0; i < nVars; i++ {
		n, err := r.node()
		if err != nil {
			return nil, err
		}
		vd, ok := n.(*VarDecl)
		if !ok {
			return nil, fmt.Errorf("program: expected *VarDecl, got %T", n)
		}
		p.GlobalVars = append(p.GlobalVars, vd)
	}

	return p, nil
}

func (r *binReader) importNode() (*Import, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	v := &Import{Position: pos}
	if v.Path, err = r.str(); err != nil {
		return nil, err
	}
	if v.Alias, err = r.str(); err != nil {
		return nil, err
	}
	if v.All, err = r.bool(); err != nil {
		return nil, err
	}
	if v.IsLib, err = r.bool(); err != nil {
		return nil, err
	}
	return v, nil
}

func (r *binReader) functionNode() (*Function, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	namePos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	f := &Function{Position: pos, NamePos: namePos}
	if f.Name, err = r.str(); err != nil {
		return nil, err
	}
	if f.ReturnType, err = r.str(); err != nil {
		return nil, err
	}
	if f.ReturnUnion, err = r.strs(); err != nil {
		return nil, err
	}
	nParams, err := r.count()
	if err != nil {
		return nil, err
	}
	f.Params = make([]*Param, 0, nParams)
	for i := 0; i < nParams; i++ {
		n, err := r.node()
		if err != nil {
			return nil, err
		}
		p, ok := n.(*Param)
		if !ok {
			return nil, fmt.Errorf("function: expected *Param, got %T", n)
		}
		f.Params = append(f.Params, p)
	}
	if f.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	if f.IsExport, err = r.bool(); err != nil {
		return nil, err
	}
	if f.IsAsync, err = r.bool(); err != nil {
		return nil, err
	}
	if f.File, err = r.str(); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *binReader) paramNode() (*Param, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	p := &Param{Position: pos}
	if p.Name, err = r.str(); err != nil {
		return nil, err
	}
	if p.Type, err = r.str(); err != nil {
		return nil, err
	}
	if p.CurrentType, err = r.str(); err != nil {
		return nil, err
	}
	if p.DefaultValue, err = r.nullableNode(); err != nil {
		return nil, err
	}
	if p.IsVariadic, err = r.bool(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *binReader) blockNode() (*Block, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	b := &Block{Position: pos}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	b.Statements = make([]Node, 0, n)
	for i := 0; i < n; i++ {
		s, err := r.node()
		if err != nil {
			return nil, err
		}
		b.Statements = append(b.Statements, s)
	}
	return b, nil
}

func (r *binReader) ternaryExprNode() (*TernaryExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	t := &TernaryExpr{Position: pos}
	if t.Condition, err = r.node(); err != nil {
		return nil, err
	}
	if t.Then, err = r.node(); err != nil {
		return nil, err
	}
	if t.Else, err = r.node(); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *binReader) typeOfNode() (*TypeOf, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	t := &TypeOf{Position: pos}
	if t.Expr, err = r.node(); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *binReader) arrayLiteralNode() (*ArrayLiteral, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	a := &ArrayLiteral{Position: pos}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	a.Elements = make([]Node, 0, n)
	for i := 0; i < n; i++ {
		e, err := r.node()
		if err != nil {
			return nil, err
		}
		a.Elements = append(a.Elements, e)
	}
	return a, nil
}

func (r *binReader) arrayIndexNode() (*ArrayIndex, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	a := &ArrayIndex{Position: pos}
	if a.Name, err = r.str(); err != nil {
		return nil, err
	}
	if a.Index, err = r.node(); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *binReader) arrayLengthNode() (*ArrayLength, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	a := &ArrayLength{Position: pos}
	if a.Name, err = r.str(); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *binReader) arrayAddNode() (*ArrayAdd, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	a := &ArrayAdd{Position: pos}
	if a.Name, err = r.str(); err != nil {
		return nil, err
	}
	if a.Elem, err = r.node(); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *binReader) dictLiteralNode() (*DictLiteral, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	d := &DictLiteral{Position: pos}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	d.Elements = make([]*DictElement, 0, n)
	for i := 0; i < n; i++ {
		node, err := r.node()
		if err != nil {
			return nil, err
		}
		e, ok := node.(*DictElement)
		if !ok {
			return nil, fmt.Errorf("dict literal: expected *DictElement, got %T", node)
		}
		d.Elements = append(d.Elements, e)
	}
	return d, nil
}

func (r *binReader) dictElementNode() (*DictElement, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	d := &DictElement{Position: pos}
	if d.Key, err = r.str(); err != nil {
		return nil, err
	}
	if d.Value, err = r.node(); err != nil {
		return nil, err
	}
	return d, nil
}

func (r *binReader) dictIndexNode() (*DictIndex, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	d := &DictIndex{Position: pos}
	if d.Object, err = r.str(); err != nil {
		return nil, err
	}
	if d.Key, err = r.node(); err != nil {
		return nil, err
	}
	return d, nil
}

func (r *binReader) varDeclNode() (*VarDecl, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	v := &VarDecl{Position: pos}
	if v.Name, err = r.str(); err != nil {
		return nil, err
	}
	if v.Type, err = r.str(); err != nil {
		return nil, err
	}
	if v.CurrentType, err = r.str(); err != nil {
		return nil, err
	}
	if v.ElemType, err = r.str(); err != nil {
		return nil, err
	}
	if v.Expr, err = r.nullableNode(); err != nil {
		return nil, err
	}
	if v.IsArray, err = r.bool(); err != nil {
		return nil, err
	}
	if v.IsConst, err = r.bool(); err != nil {
		return nil, err
	}
	if v.IsExport, err = r.bool(); err != nil {
		return nil, err
	}
	return v, nil
}

func (r *binReader) assignNode() (*Assign, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	a := &Assign{Position: pos}
	if a.Name, err = r.str(); err != nil {
		return nil, err
	}
	if a.Field, err = r.str(); err != nil {
		return nil, err
	}
	if a.Index, err = r.nullableNode(); err != nil {
		return nil, err
	}
	if a.Expr, err = r.nullableNode(); err != nil {
		return nil, err
	}
	if a.Op, err = r.str(); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *binReader) binaryExprNode() (*BinaryExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	b := &BinaryExpr{Position: pos}
	if b.Left, err = r.node(); err != nil {
		return nil, err
	}
	if b.Op, err = r.str(); err != nil {
		return nil, err
	}
	if b.Right, err = r.node(); err != nil {
		return nil, err
	}
	return b, nil
}

func (r *binReader) unaryExprNode() (*UnaryExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	u := &UnaryExpr{Position: pos}
	if u.Op, err = r.str(); err != nil {
		return nil, err
	}
	if u.Expr, err = r.node(); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *binReader) rangeExprNode() (*RangeExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	re := &RangeExpr{Position: pos}
	if re.Start, err = r.node(); err != nil {
		return nil, err
	}
	if re.End, err = r.node(); err != nil {
		return nil, err
	}
	return re, nil
}

func (r *binReader) callRangeExprNode() (*CallRangeExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	c := &CallRangeExpr{Position: pos}
	if c.Name, err = r.str(); err != nil {
		return nil, err
	}
	if c.Range, err = r.requireRange(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	c.Extra = make([]Node, 0, n)
	for i := 0; i < n; i++ {
		e, err := r.node()
		if err != nil {
			return nil, err
		}
		c.Extra = append(c.Extra, e)
	}
	return c, nil
}

func (r *binReader) callExprNode() (*CallExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	c := &CallExpr{Position: pos}
	if c.Name, err = r.str(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	c.Args = make([]Node, 0, n)
	for i := 0; i < n; i++ {
		a, err := r.node()
		if err != nil {
			return nil, err
		}
		c.Args = append(c.Args, a)
	}
	if c.Receiver, err = r.str(); err != nil {
		return nil, err
	}
	if c.ReceiverNode, err = r.nullableNode(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *binReader) returnStmtNode() (*ReturnStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	s := &ReturnStmt{Position: pos}
	if s.Expr, err = r.nullableNode(); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *binReader) identNode() (*Ident, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	i := &Ident{Position: pos}
	if i.Name, err = r.str(); err != nil {
		return nil, err
	}
	return i, nil
}

func (r *binReader) numberNode() (*Number, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	n := &Number{Position: pos}
	if n.Value, err = r.str(); err != nil {
		return nil, err
	}
	return n, nil
}

func (r *binReader) formatExprNode() (*FormatExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	f := &FormatExpr{Position: pos}
	if f.Expr, err = r.node(); err != nil {
		return nil, err
	}
	if f.Mode, err = r.str(); err != nil {
		return nil, err
	}
	nVal, err := r.varint()
	if err != nil {
		return nil, err
	}
	f.N = int(nVal)
	if f.TrimSet, err = r.str(); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *binReader) stringNode() (*String, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	s := &String{Position: pos}
	if s.Value, err = r.str(); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *binReader) charLiteralNode() (*CharLiteral, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	c := &CharLiteral{Position: pos}
	if c.Value, err = r.u8(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *binReader) nullLiteralNode() (*NullLiteral, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	return &NullLiteral{Position: pos}, nil
}

func (r *binReader) ifStmtNode() (*IfStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	stmt := &IfStmt{Position: pos}
	if stmt.Condition, err = r.node(); err != nil {
		return nil, err
	}
	if stmt.Then, err = r.requireBlock(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	stmt.Elsifs = make([]*Elsif, 0, n)
	for i := 0; i < n; i++ {
		node, err := r.node()
		if err != nil {
			return nil, err
		}
		e, ok := node.(*Elsif)
		if !ok {
			return nil, fmt.Errorf("if: expected *Elsif, got %T", node)
		}
		stmt.Elsifs = append(stmt.Elsifs, e)
	}
	if stmt.Else, err = r.nullableBlock(); err != nil {
		return nil, err
	}
	return stmt, nil
}

func (r *binReader) elsifNode() (*Elsif, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	e := &Elsif{Position: pos}
	if e.Condition, err = r.node(); err != nil {
		return nil, err
	}
	if e.Then, err = r.requireBlock(); err != nil {
		return nil, err
	}
	return e, nil
}

func (r *binReader) caseStmtNode() (*CaseStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	c := &CaseStmt{Position: pos}
	if c.Value, err = r.node(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	c.Branches = make([]*CaseBranch, 0, n)
	for i := 0; i < n; i++ {
		node, err := r.node()
		if err != nil {
			return nil, err
		}
		br, ok := node.(*CaseBranch)
		if !ok {
			return nil, fmt.Errorf("case: expected *CaseBranch, got %T", node)
		}
		c.Branches = append(c.Branches, br)
	}
	if c.Default, err = r.nullableBlock(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *binReader) caseBranchNode() (*CaseBranch, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	c := &CaseBranch{Position: pos}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	c.Patterns = make([]Node, 0, n)
	for i := 0; i < n; i++ {
		p, err := r.node()
		if err != nil {
			return nil, err
		}
		c.Patterns = append(c.Patterns, p)
	}
	if c.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *binReader) whileStmtNode() (*WhileStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	w := &WhileStmt{Position: pos}
	if w.Condition, err = r.node(); err != nil {
		return nil, err
	}
	if w.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *binReader) forInStmtNode() (*ForInStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	f := &ForInStmt{Position: pos}
	if f.VarType, err = r.str(); err != nil {
		return nil, err
	}
	if f.VarName, err = r.str(); err != nil {
		return nil, err
	}
	if f.Iterable, err = r.node(); err != nil {
		return nil, err
	}
	if f.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *binReader) includeCNode() (*IncludeC, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	i := &IncludeC{Position: pos}
	if i.Code, err = r.str(); err != nil {
		return nil, err
	}
	return i, nil
}

func (r *binReader) iknowIdoBlockNode() (*IknowIdoBlock, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	b := &IknowIdoBlock{Position: pos}
	if b.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	return b, nil
}

func (r *binReader) unicodeLiteralNode() (*UnicodeLiteral, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	u := &UnicodeLiteral{Position: pos}
	cp, err := r.uvarint()
	if err != nil {
		return nil, err
	}
	if cp > 0x10FFFF {
		return nil, fmt.Errorf(
			"unicode codepoint out of range: 0x%X at byte %d", cp, r.off)
	}
	u.Codepoint = uint32(cp)
	return u, nil
}

func (r *binReader) errorDeclNode() (*ErrorDecl, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	d := &ErrorDecl{Position: pos}
	if d.Name, err = r.str(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	d.Fields = make([]*ErrorField, 0, n)
	for i := 0; i < n; i++ {
		node, err := r.node()
		if err != nil {
			return nil, err
		}
		f, ok := node.(*ErrorField)
		if !ok {
			return nil, fmt.Errorf("error decl: expected *ErrorField, got %T", node)
		}
		d.Fields = append(d.Fields, f)
	}
	if d.Parent, err = r.str(); err != nil {
		return nil, err
	}
	if d.IsNew, err = r.bool(); err != nil {
		return nil, err
	}
	if d.Module, err = r.str(); err != nil {
		return nil, err
	}
	return d, nil
}

func (r *binReader) errorFieldNode() (*ErrorField, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	f := &ErrorField{Position: pos}
	if f.Name, err = r.str(); err != nil {
		return nil, err
	}
	if f.Type, err = r.str(); err != nil {
		return nil, err
	}
	if f.DefaultValue, err = r.nullableNode(); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *binReader) errorInstanceNode() (*ErrorInstance, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	e := &ErrorInstance{Position: pos}
	if e.TypeName, err = r.str(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	e.Fields = make(map[string]Node, n)
	for i := 0; i < n; i++ {
		key, err := r.str()
		if err != nil {
			return nil, err
		}
		val, err := r.node()
		if err != nil {
			return nil, err
		}
		e.Fields[key] = val
	}
	return e, nil
}

func (r *binReader) throwStmtNode() (*ThrowStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	t := &ThrowStmt{Position: pos}
	if t.Expr, err = r.node(); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *binReader) fieldAccessNode() (*FieldAccess, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	f := &FieldAccess{Position: pos}
	if f.Object, err = r.str(); err != nil {
		return nil, err
	}
	if f.Field, err = r.str(); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *binReader) tryStmtNode() (*TryStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	t := &TryStmt{Position: pos}
	if t.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	n, err := r.count()
	if err != nil {
		return nil, err
	}
	t.Catches = make([]*CatchClause, 0, n)
	for i := 0; i < n; i++ {
		node, err := r.node()
		if err != nil {
			return nil, err
		}
		c, ok := node.(*CatchClause)
		if !ok {
			return nil, fmt.Errorf("try: expected *CatchClause, got %T", node)
		}
		t.Catches = append(t.Catches, c)
	}
	return t, nil
}

func (r *binReader) catchClauseNode() (*CatchClause, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	c := &CatchClause{Position: pos}
	if c.TypeName, err = r.str(); err != nil {
		return nil, err
	}
	if c.VarName, err = r.str(); err != nil {
		return nil, err
	}
	if c.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *binReader) breakStmtNode() (*BreakStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	return &BreakStmt{Position: pos}, nil
}

func (r *binReader) continueStmtNode() (*ContinueStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	return &ContinueStmt{Position: pos}, nil
}

func (r *binReader) doWhileStmtNode() (*DoWhileStmt, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	d := &DoWhileStmt{Position: pos}
	if d.Body, err = r.requireBlock(); err != nil {
		return nil, err
	}
	if d.Condition, err = r.node(); err != nil {
		return nil, err
	}
	return d, nil
}

func (r *binReader) postfixExprNode() (*PostfixExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	p := &PostfixExpr{Position: pos}
	if p.VarName, err = r.str(); err != nil {
		return nil, err
	}
	if p.Op, err = r.str(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *binReader) promiseSetNode() (*PromiseSet, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	p := &PromiseSet{Position: pos}
	if p.Value, err = r.node(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *binReader) awaitExprNode() (*AwaitExpr, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	a := &AwaitExpr{Position: pos}
	if a.Expr, err = r.node(); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *binReader) isAsyncEndedNode() (*IsAsyncEnded, error) {
	pos, err := r.readPos()
	if err != nil {
		return nil, err
	}
	i := &IsAsyncEnded{Position: pos}
	if i.FuncName, err = r.str(); err != nil {
		return nil, err
	}
	return i, nil
}

// ============================================================================
// Library — верхнеуровневая структура .sklib
// ============================================================================

// Library — содержимое .sklib.
type Library struct {
	FormatVersion int
	Name          string
	Version       string
	Author        string
	Dependencies  []string
	Program       *Program
}

// WriteLibrary сериализует Library в файл.
func WriteLibrary(path string, lib *Library) error {
	lib.FormatVersion = SKLibFormatVersion

	// AllFunctions восстанавливается при импорте — не пишем.
	if lib.Program != nil {
		lib.Program.AllFunctions = nil
	}

	w := newBinWriter()
	w.uvarint(uint64(lib.FormatVersion))
	w.str(lib.Name)
	w.str(lib.Version)
	w.str(lib.Author)
	w.strs(lib.Dependencies)
	if lib.Program == nil {
		w.u8(0)
	} else {
		w.u8(1)
		if err := w.node(lib.Program); err != nil {
			return fmt.Errorf("encode program: %w", err)
		}
	}

	// gzip
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(w.buf.Bytes()); err != nil {
		return fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}

	// Собираем файл: magic + version + gzipped payload
	var out bytes.Buffer
	out.Write(sklibMagic)
	out.WriteByte(byte(SKLibFormatVersion))
	out.Write(compressed.Bytes())

	if err := os.WriteFile(path, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// ReadLibrary читает Library из файла.
func ReadLibrary(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// 1. Заголовок
	if len(data) < sklibHeaderSize {
		return nil, fmt.Errorf("not a .sklib file: too short (%d bytes)", len(data))
	}
	if !bytes.Equal(data[:4], sklibMagic) {
		return nil, fmt.Errorf(
			"not a .sklib file: bad magic bytes (expected %q, got %q)",
			sklibMagic, data[:4])
	}

	headerVersion := int(data[4])
	if headerVersion != SKLibFormatVersion {
		return nil, fmt.Errorf(
			"unsupported .sklib format version: got %d, expected %d (recompile the library)",
			headerVersion, SKLibFormatVersion)
	}

	// 2. gzip
	gz, err := gzip.NewReader(bytes.NewReader(data[sklibHeaderSize:]))
	if err != nil {
		return nil, fmt.Errorf("gzip open: %w", err)
	}
	decompressed, err := io.ReadAll(gz)
	_ = gz.Close()
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}

	// 3. Payload
	r := newBinReader(decompressed)

	fv, err := r.uvarint()
	if err != nil {
		return nil, fmt.Errorf("read format version: %w", err)
	}
	if int(fv) != SKLibFormatVersion {
		return nil, fmt.Errorf(
			"internal version mismatch: header=%d, body=%d (file is corrupted)",
			headerVersion, fv)
	}

	lib := &Library{FormatVersion: int(fv)}
	if lib.Name, err = r.str(); err != nil {
		return nil, fmt.Errorf("read name: %w", err)
	}
	if lib.Version, err = r.str(); err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}
	if lib.Author, err = r.str(); err != nil {
		return nil, fmt.Errorf("read author: %w", err)
	}
	if lib.Dependencies, err = r.strs(); err != nil {
		return nil, fmt.Errorf("read dependencies: %w", err)
	}

	hasProg, err := r.u8()
	if err != nil {
		return nil, fmt.Errorf("read program flag: %w", err)
	}
	switch hasProg {
	case 0:
		// Program отсутствует (nil).
	case 1:
		n, err := r.node()
		if err != nil {
			return nil, fmt.Errorf("read program: %w", err)
		}
		prog, ok := n.(*Program)
		if !ok {
			return nil, fmt.Errorf("expected *Program, got %T", n)
		}
		lib.Program = prog
	default:
		return nil, fmt.Errorf(
			"invalid program flag 0x%02x at byte %d", hasProg, r.off-1)
	}

	if r.off != len(decompressed) {
		return nil, fmt.Errorf(
			"trailing garbage: %d bytes after valid payload",
			len(decompressed)-r.off)
	}

	return lib, nil
}

// ============================================================================
// LibSpec — разбор пути "lib:author/name@1.0.0"
// ============================================================================

// LibSpec — разобранный путь "lib:author/name@1.0.0".
type LibSpec struct {
	Author  string
	Name    string
	Version string // "" если не указана
}

// Raw возвращает исходную строку без "lib:".
func (s LibSpec) Raw() string {
	if s.Version == "" {
		return s.Author + "/" + s.Name
	}
	return s.Author + "/" + s.Name + "@" + s.Version
}

// ParseLibSpec разбирает "lib:author/name@1.0.0" (или без версии).
func ParseLibSpec(path string) (LibSpec, error) {
	spec := LibSpec{}

	p := strings.TrimPrefix(path, "lib:")
	if p == path {
		return spec, fmt.Errorf("library path must start with 'lib:'")
	}

	if idx := strings.LastIndex(p, "@"); idx >= 0 {
		spec.Version = p[idx+1:]
		p = p[:idx]
		if spec.Version == "" {
			return spec, fmt.Errorf("empty version in '%s'", path)
		}
	}

	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 {
		return spec, fmt.Errorf(
			"invalid library path '%s': expected 'lib:author/name[@version]'", path)
	}

	spec.Author = parts[0]
	spec.Name = parts[1]

	if spec.Author == "" || spec.Name == "" {
		return spec, fmt.Errorf("empty author or name in '%s'", path)
	}

	return spec, nil
}

// FindLibrary ищет .sklib в libsDir для данного spec
// и возвращает прочитанную Library вместе с путём к файлу.
//
// Правила:
//   - Папка: {libsDir}/{author}/{name}/
//   - Расширение: .sklib (обязательно)
//   - Если version указан — ищем файл, внутри которого Version == spec.Version
//   - Если version НЕ указан — в папке должен быть ровно 1 .sklib
func FindLibrary(libsDir string, spec LibSpec) (*Library, string, error) {
	dir := fmt.Sprintf("%s/%s/%s",
		strings.TrimRight(libsDir, "/"), spec.Author, spec.Name)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("library not found: %s/%s (dir: %s)",
			spec.Author, spec.Name, dir)
	}

	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".sklib") {
			continue
		}
		candidates = append(candidates, dir+"/"+e.Name())
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("library %s/%s not found in %s (no .sklib files)",
			spec.Author, spec.Name, dir)
	}

	// Без версии — ровно 1 файл
	if spec.Version == "" {
		if len(candidates) > 1 {
			return nil, "", fmt.Errorf(
				"multiple versions of library %s/%s found in %s; specify @version",
				spec.Author, spec.Name, dir)
		}
		lib, err := ReadLibrary(candidates[0])
		if err != nil {
			return nil, "", fmt.Errorf("cannot read %s: %w", candidates[0], err)
		}
		return lib, candidates[0], nil
	}

	// С версией — читаем все и ищем совпадение
	var matches []*Library
	var matchFiles []string
	for _, file := range candidates {
		lib, err := ReadLibrary(file)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read %s: %w", file, err)
		}
		if lib.Version == spec.Version {
			matches = append(matches, lib)
			matchFiles = append(matchFiles, file)
		}
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("library %s/%s@%s not found in %s",
			spec.Author, spec.Name, spec.Version, dir)
	}
	if len(matches) > 1 {
		return nil, "", fmt.Errorf(
			"multiple files with version %s for library %s/%s in %s",
			spec.Version, spec.Author, spec.Name, dir)
	}

	return matches[0], matchFiles[0], nil
}
