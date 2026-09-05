package midlevel

import (
	"fmt"
)

type SymbolKind int

const (
	SYM_VARIABLE SymbolKind = iota
	SYM_FUNCTION
	SYM_CONST
	SYM_IMPORT
)

type Symbol struct {
	Name       string
	Kind       SymbolKind
	Type       string
	RealType   string
	IsExported bool
	IsConst    bool
	Value      interface{} // для констант
	Scope      *Scope
}

type Scope struct {
	Parent   *Scope
	Symbols  map[string]*Symbol
	IsGlobal bool
	Depth    int
}

func NewScope(parent *Scope, isGlobal bool) *Scope {
	depth := 0
	if parent != nil {
		depth = parent.Depth + 1
	}
	return &Scope{
		Parent:   parent,
		Symbols:  make(map[string]*Symbol),
		IsGlobal: isGlobal,
		Depth:    depth,
	}
}

func (s *Scope) Define(name string, kind SymbolKind, typ string, isExported bool) *Symbol {
	sym := &Symbol{
		Name:       name,
		Kind:       kind,
		Type:       typ,
		IsExported: isExported,
		Scope:      s,
	}
	s.Symbols[name] = sym
	return sym
}

func (s *Scope) DefineConst(name string, typ string, value interface{}) *Symbol {
	sym := &Symbol{
		Name:    name,
		Kind:    SYM_CONST,
		Type:    typ,
		IsConst: true,
		Value:   value,
		Scope:   s,
	}
	s.Symbols[name] = sym
	return sym
}

func (s *Scope) Resolve(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	if s.Parent != nil {
		return s.Parent.Resolve(name)
	}
	return nil
}

func (s *Scope) ResolveLocal(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	return nil
}

func (s *Scope) String() string {
	result := fmt.Sprintf("Scope(depth=%d, symbols=%d", s.Depth, len(s.Symbols))
	if s.Parent != nil {
		result += fmt.Sprintf(", parent_depth=%d", s.Parent.Depth)
	}
	result += ")"
	return result
}
