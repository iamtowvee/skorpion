package front

import "strings"

type NodeType string

const (
	NODE_PROGRAM       NodeType = "Program"
	NODE_FUNCTION      NodeType = "Function"
	NODE_VAR_DECL      NodeType = "VarDecl"
	NODE_ASSIGN        NodeType = "Assign"
	NODE_BINARY        NodeType = "Binary"
	NODE_UNARY         NodeType = "Unary"
	NODE_CALL          NodeType = "Call"
	NODE_RETURN        NodeType = "Return"
	NODE_BLOCK         NodeType = "Block"
	NODE_IDENT         NodeType = "Ident"
	NODE_NUMBER        NodeType = "Number"
	NODE_STRING        NodeType = "String"
	NODE_IF            NodeType = "If"
	NODE_ELSIF         NodeType = "Elsif"
	NODE_CASE          NodeType = "Case"
	NODE_CASE_BRANCH   NodeType = "CaseBranch"
	NODE_WHILE         NodeType = "While"
	NODE_FOR           NodeType = "For"
	NODE_TYPEOF        NodeType = "TypeOf"
	NODE_ARRAY_LITERAL NodeType = "ArrayLiteral"
	NODE_ARRAY_INDEX   NodeType = "ArrayIndex"
	NODE_ARRAY_LENGTH  NodeType = "ArrayLength"
	NODE_ARRAY_ADD     NodeType = "ArrayAdd"
	NODE_IMPORT        NodeType = "Import"
	NODE_INCLUDE_C     NodeType = "IncludeC"
	NODE_RANGE         NodeType = "Range"
	NODE_CALL_RANGE    NodeType = "CallRange"
	NODE_TERNARY       NodeType = "Ternary"
)

type Node interface {
	GetType() NodeType
	GetLine() int
	GetColumn() int
}

type TypedNode interface {
	Node
	GetTypeString() string
}

// ============================================================================
// Position — базовая структура для всех узлов
// ============================================================================

type Position struct {
	Line   int
	Column int
}

func (p Position) GetLine() int   { return p.Line }
func (p Position) GetColumn() int { return p.Column }

// ============================================================================
// Program
// ============================================================================

type Program struct {
	Position
	Imports      []*Import
	Functions    []*Function
	AllFunctions []*Function
}

func (p *Program) GetType() NodeType { return NODE_PROGRAM }

// ============================================================================
// Import
// ============================================================================

type Import struct {
	Position
	Path  string
	Alias string
	All   bool
}

func (i *Import) GetType() NodeType { return NODE_IMPORT }

// ============================================================================
// Function
// ============================================================================

type Function struct {
	Position
	Name       string
	ReturnType string
	Params     []*Param
	Body       *Block
	IsExport   bool
	File       string
}

func (f *Function) GetType() NodeType { return NODE_FUNCTION }

// ============================================================================
// Param
// ============================================================================

type Param struct {
	Position
	Name         string
	Type         string
	DefaultValue Node
}

func (p *Param) GetType() NodeType { return "Param" }

// ============================================================================
// Block
// ============================================================================

type Block struct {
	Position
	Statements []Node
}

func (b *Block) GetType() NodeType { return NODE_BLOCK }

// ============================================================================
// TernaryExpr
// ============================================================================

type TernaryExpr struct {
	Position
	Condition Node
	Then      Node
	Else      Node
}

func (t *TernaryExpr) GetType() NodeType { return NODE_TERNARY }

// ============================================================================
// TypeOf
// ============================================================================

type TypeOf struct {
	Position
	Expr Node
}

func (t *TypeOf) GetType() NodeType { return NODE_TYPEOF }

// ============================================================================
// ArrayLiteral
// ============================================================================

type ArrayLiteral struct {
	Position
	Elements []Node
}

func (a *ArrayLiteral) GetType() NodeType { return NODE_ARRAY_LITERAL }

// ============================================================================
// ArrayIndex
// ============================================================================

type ArrayIndex struct {
	Position
	Name  string
	Index Node
}

func (a *ArrayIndex) GetType() NodeType { return NODE_ARRAY_INDEX }

// ============================================================================
// ArrayLength
// ============================================================================

type ArrayLength struct {
	Position
	Name string
}

func (a *ArrayLength) GetType() NodeType { return NODE_ARRAY_LENGTH }

// ============================================================================
// ArrayAdd
// ============================================================================

type ArrayAdd struct {
	Position
	Name string
	Elem Node
}

func (a *ArrayAdd) GetType() NodeType { return NODE_ARRAY_ADD }

// ============================================================================
// VarDecl
// ============================================================================

type VarDecl struct {
	Position
	Name     string
	Type     string
	ElemType string
	Expr     Node
	IsArray  bool
}

func (v *VarDecl) GetType() NodeType     { return NODE_VAR_DECL }
func (v *VarDecl) GetTypeString() string { return v.Type }

// ============================================================================
// Assign
// ============================================================================

type Assign struct {
	Position
	Name string
	Expr Node
}

func (a *Assign) GetType() NodeType { return NODE_ASSIGN }

// ============================================================================
// BinaryExpr
// ============================================================================

type BinaryExpr struct {
	Position
	Left  Node
	Op    string
	Right Node
}

func (b *BinaryExpr) GetType() NodeType { return NODE_BINARY }

// ============================================================================
// UnaryExpr
// ============================================================================

type UnaryExpr struct {
	Position
	Op   string
	Expr Node
}

func (u *UnaryExpr) GetType() NodeType { return NODE_UNARY }
func (u *UnaryExpr) GetTypeString() string {
	if u.Op == "$" {
		return "string"
	}
	return "int"
}

// ============================================================================
// RangeExpr
// ============================================================================

type RangeExpr struct {
	Position
	Start Node
	End   Node
}

func (r *RangeExpr) GetType() NodeType { return NODE_RANGE }

// ============================================================================
// CallRangeExpr
// ============================================================================

type CallRangeExpr struct {
	Position
	Name  string
	Range *RangeExpr
	Extra []Node
}

func (c *CallRangeExpr) GetType() NodeType { return NODE_CALL_RANGE }

// ============================================================================
// CallExpr
// ============================================================================

type CallExpr struct {
	Position
	Name     string
	Args     []Node
	Receiver string
}

func (c *CallExpr) GetType() NodeType { return NODE_CALL }

// ============================================================================
// ReturnStmt
// ============================================================================

type ReturnStmt struct {
	Position
	Expr Node
}

func (r *ReturnStmt) GetType() NodeType { return NODE_RETURN }

// ============================================================================
// Ident
// ============================================================================

type Ident struct {
	Position
	Name string
}

func (i *Ident) GetType() NodeType { return NODE_IDENT }

// ============================================================================
// Number
// ============================================================================

type Number struct {
	Position
	Value string
}

func (n *Number) GetType() NodeType { return NODE_NUMBER }
func (n *Number) GetTypeString() string {
	if strings.Contains(n.Value, ".") {
		return "float"
	}
	return "int"
}

// ============================================================================
// String
// ============================================================================

type String struct {
	Position
	Value string
}

func (s *String) GetType() NodeType     { return NODE_STRING }
func (s *String) GetTypeString() string { return "string" }

// ============================================================================
// IfStmt
// ============================================================================

type IfStmt struct {
	Position
	Condition Node
	Then      *Block
	Elsifs    []*Elsif
	Else      *Block
}

type Elsif struct {
	Position
	Condition Node
	Then      *Block
}

func (i *IfStmt) GetType() NodeType { return NODE_IF }
func (e *Elsif) GetType() NodeType  { return NODE_ELSIF }

// ============================================================================
// CaseStmt
// ============================================================================

type CaseStmt struct {
	Position
	Value    Node
	Branches []*CaseBranch
	Default  *Block
}

type CaseBranch struct {
	Position
	Pattern Node
	Body    *Block
}

func (c *CaseStmt) GetType() NodeType   { return NODE_CASE }
func (c *CaseBranch) GetType() NodeType { return NODE_CASE_BRANCH }

// ============================================================================
// WhileStmt
// ============================================================================

type WhileStmt struct {
	Position
	Condition Node
	Body      *Block
}

func (w *WhileStmt) GetType() NodeType { return NODE_WHILE }

// ============================================================================
// ForStmt
// ============================================================================

type ForStmt struct {
	Position
	Init Node
	Cond Node
	Post Node
	Body *Block
}

func (f *ForStmt) GetType() NodeType { return NODE_FOR }

// ============================================================================
// IncludeC
// ============================================================================

type IncludeC struct {
	Position
	Code string
}

func (i *IncludeC) GetType() NodeType { return NODE_INCLUDE_C }
