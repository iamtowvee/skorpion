package front

import "strings"

type NodeType string

const (
	NODE_PROGRAM   NodeType = "Program"
	NODE_FUNCTION  NodeType = "Function"
	NODE_VAR_DECL  NodeType = "VarDecl"
	NODE_ASSIGN    NodeType = "Assign"
	NODE_BINARY    NodeType = "Binary"
	NODE_UNARY     NodeType = "Unary"
	NODE_CALL      NodeType = "Call"
	NODE_RETURN    NodeType = "Return"
	NODE_BLOCK     NodeType = "Block"
	NODE_IDENT     NodeType = "Ident"
	NODE_NUMBER    NodeType = "Number"
	NODE_STRING    NodeType = "String"
	NODE_IF        NodeType = "If"
	NODE_ELSIF     NodeType = "Elsif"
	NODE_WHILE     NodeType = "While"
	NODE_FOR       NodeType = "For"
	NODE_IMPORT    NodeType = "Import"
	NODE_INCLUDE_C NodeType = "IncludeC"
)

type Node interface {
	GetType() NodeType
}

type TypedNode interface {
	Node
	GetTypeString() string
}

type Program struct {
	Imports   []*Import
	Functions []*Function
}

func (p *Program) GetType() NodeType { return NODE_PROGRAM }

type Import struct {
	Path  string
	Alias string
	All   bool
}

func (i *Import) GetType() NodeType { return NODE_IMPORT }

type Function struct {
	Name       string
	ReturnType string
	Params     []*Param
	Body       *Block
	IsExport   bool
}

func (f *Function) GetType() NodeType { return NODE_FUNCTION }

type Param struct {
	Name         string
	Type         string
	DefaultValue Node
}

type Block struct {
	Statements []Node
}

func (b *Block) GetType() NodeType { return NODE_BLOCK }

type VarDecl struct {
	Name string
	Type string
	Expr Node
}

func (v *VarDecl) GetType() NodeType     { return NODE_VAR_DECL }
func (v *VarDecl) GetTypeString() string { return v.Type }

type Assign struct {
	Name string
	Expr Node
}

func (a *Assign) GetType() NodeType { return NODE_ASSIGN }

type BinaryExpr struct {
	Left  Node
	Op    string
	Right Node
}

func (b *BinaryExpr) GetType() NodeType { return NODE_BINARY }

type UnaryExpr struct {
	Op   string
	Expr Node
}

func (u *UnaryExpr) GetType() NodeType { return NODE_UNARY }
func (u *UnaryExpr) GetTypeString() string {
	// $ преобразует в строку
	if u.Op == "$" {
		return "string"
	}
	return "int"
}

type CallExpr struct {
	Name string
	Args []Node
}

func (c *CallExpr) GetType() NodeType { return NODE_CALL }

type ReturnStmt struct {
	Expr Node
}

func (r *ReturnStmt) GetType() NodeType { return NODE_RETURN }

type Ident struct {
	Name string
}

func (i *Ident) GetType() NodeType { return NODE_IDENT }

type Number struct {
	Value string
}

func (n *Number) GetType() NodeType { return NODE_NUMBER }
func (n *Number) GetTypeString() string {
	if strings.Contains(n.Value, ".") {
		return "float"
	}
	return "int"
}

type String struct {
	Value string
}

func (s *String) GetType() NodeType     { return NODE_STRING }
func (s *String) GetTypeString() string { return "string" }

type IfStmt struct {
	Condition Node
	Then      *Block
	Elsifs    []*Elsif
	Else      *Block
}

type Elsif struct {
	Condition Node
	Then      *Block
}

func (i *IfStmt) GetType() NodeType { return NODE_IF }
func (e *Elsif) GetType() NodeType  { return NODE_ELSIF }

type WhileStmt struct {
	Condition Node
	Body      *Block
}

func (w *WhileStmt) GetType() NodeType { return NODE_WHILE }

type ForStmt struct {
	Init Node
	Cond Node
	Post Node
	Body *Block
}

func (f *ForStmt) GetType() NodeType { return NODE_FOR }

type IncludeC struct {
	Code string
}

func (i *IncludeC) GetType() NodeType { return NODE_INCLUDE_C }
