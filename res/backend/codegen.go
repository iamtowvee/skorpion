package backend

import (
	"fmt"
	"skrp/res/front"
	"strings"
)

// CodeGenerator генерирует C код
type CodeGenerator struct {
	code       strings.Builder
	indent     int
	globalCode string
	includes   map[string]bool
}

// NewCodeGenerator создает новый генератор
func NewCodeGenerator() *CodeGenerator {
	return &CodeGenerator{
		includes: make(map[string]bool),
	}
}

// Generate генерирует C код из AST
func (g *CodeGenerator) Generate(ast *front.ASTNode) (string, error) {
	g.code.Reset()
	g.globalCode = ""
	g.indent = 0
	g.includes = make(map[string]bool)

	// Добавляем стандартные include
	g.includes["stdio.h"] = true
	g.includes["stdlib.h"] = true
	g.includes["string.h"] = true

	// Генерируем код
	for _, node := range ast.Children {
		if err := g.generateNode(node); err != nil {
			return "", err
		}
	}

	// Собираем финальный код
	var finalCode strings.Builder

	// Добавляем includes
	for include := range g.includes {
		finalCode.WriteString(fmt.Sprintf("#include <%s>\n", include))
	}
	finalCode.WriteString("\n")

	// Добавляем глобальный код
	finalCode.WriteString(g.globalCode)
	finalCode.WriteString("\n")

	// Добавляем сгенерированный код
	finalCode.WriteString(g.code.String())

	return finalCode.String(), nil
}

func (g *CodeGenerator) generateNode(node *front.ASTNode) error {
	switch node.Type {
	case "IncludeC":
		// Просто вставляем C код
		g.globalCode += node.Value.(string) + "\n"
		return nil
	case "Function":
		return g.generateFunction(node)
	case "VariableDeclaration":
		return g.generateVariableDeclaration(node)
	case "Assignment":
		return g.generateAssignment(node)
	case "Call":
		return g.generateCall(node)
	case "Return":
		return g.generateReturn(node)
	case "If":
		return g.generateIf(node)
	case "While":
		return g.generateWhile(node)
	case "Block":
		return g.generateBlock(node)
	case "Use":
		// Импорты обрабатываются на уровне парсера
		return nil
	default:
		// Для остальных узлов просто обрабатываем детей
		for _, child := range node.Children {
			if err := g.generateNode(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *CodeGenerator) generateFunction(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	name := data["name"].(string)
	returnType := g.mapType(data["returnType"].(string))

	// Генерируем сигнатуру
	g.writeIndent()
	g.code.WriteString(fmt.Sprintf("%s %s(", returnType, name))

	params := data["params"].([]map[string]string)
	for i, param := range params {
		if i > 0 {
			g.code.WriteString(", ")
		}
		paramType := g.mapType(param["type"])
		g.code.WriteString(fmt.Sprintf("%s %s", paramType, param["name"]))
	}
	g.code.WriteString(") {\n")
	g.indent++

	// Генерируем тело
	if len(node.Children) > 0 {
		if err := g.generateNode(node.Children[0]); err != nil {
			return err
		}
	}

	g.indent--
	g.writeIndent()
	g.code.WriteString("}\n\n")

	return nil
}

func (g *CodeGenerator) generateVariableDeclaration(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	varType := g.mapType(data["type"].(string))
	varName := data["name"].(string)

	g.writeIndent()
	g.code.WriteString(fmt.Sprintf("%s %s", varType, varName))

	// Инициализация
	if len(node.Children) > 0 {
		g.code.WriteString(" = ")
		if err := g.generateExpression(node.Children[0]); err != nil {
			return err
		}
	} else {
		// Инициализируем значением по умолчанию
		g.code.WriteString(" = ")
		g.code.WriteString(g.defaultValue(data["type"].(string)))
	}

	g.code.WriteString(";\n")
	return nil
}

func (g *CodeGenerator) generateAssignment(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	varName := data["name"].(string)

	g.writeIndent()
	g.code.WriteString(fmt.Sprintf("%s = ", varName))

	if len(node.Children) > 0 {
		if err := g.generateExpression(node.Children[0]); err != nil {
			return err
		}
	}
	g.code.WriteString(";\n")

	return nil
}

func (g *CodeGenerator) generateCall(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	name := data["name"].(string)

	g.writeIndent()
	g.code.WriteString(fmt.Sprintf("%s(", name))

	for i, arg := range node.Children {
		if i > 0 {
			g.code.WriteString(", ")
		}
		if err := g.generateExpression(arg); err != nil {
			return err
		}
	}
	g.code.WriteString(");\n")

	return nil
}

func (g *CodeGenerator) generateReturn(node *front.ASTNode) error {
	g.writeIndent()
	g.code.WriteString("return")

	if len(node.Children) > 0 {
		g.code.WriteString(" ")
		if err := g.generateExpression(node.Children[0]); err != nil {
			return err
		}
	}
	g.code.WriteString(";\n")

	return nil
}

func (g *CodeGenerator) generateIf(node *front.ASTNode) error {
	if len(node.Children) < 2 {
		return nil
	}

	g.writeIndent()
	g.code.WriteString("if (")
	if err := g.generateExpression(node.Children[0]); err != nil {
		return err
	}
	g.code.WriteString(") {\n")
	g.indent++

	if err := g.generateNode(node.Children[1]); err != nil {
		return err
	}

	g.indent--
	g.writeIndent()
	g.code.WriteString("}")

	// Проверяем else
	if len(node.Children) > 2 && node.Children[2].Type == "Else" {
		g.code.WriteString(" else {\n")
		g.indent++
		if len(node.Children[2].Children) > 0 {
			if err := g.generateNode(node.Children[2].Children[0]); err != nil {
				return err
			}
		}
		g.indent--
		g.writeIndent()
		g.code.WriteString("}")
	}

	g.code.WriteString("\n")
	return nil
}

func (g *CodeGenerator) generateWhile(node *front.ASTNode) error {
	if len(node.Children) < 2 {
		return nil
	}

	g.writeIndent()
	g.code.WriteString("while (")
	if err := g.generateExpression(node.Children[0]); err != nil {
		return err
	}
	g.code.WriteString(") {\n")
	g.indent++

	if err := g.generateNode(node.Children[1]); err != nil {
		return err
	}

	g.indent--
	g.writeIndent()
	g.code.WriteString("}\n")

	return nil
}

func (g *CodeGenerator) generateBlock(node *front.ASTNode) error {
	for _, child := range node.Children {
		if err := g.generateNode(child); err != nil {
			return err
		}
	}
	return nil
}

func (g *CodeGenerator) generateExpression(node *front.ASTNode) error {
	switch node.Type {
	case "NumberLiteral":
		g.code.WriteString(node.Value.(string))
	case "StringLiteral":
		g.code.WriteString(fmt.Sprintf("\"%s\"", node.Value.(string)))
	case "CharLiteral":
		g.code.WriteString(fmt.Sprintf("'%c'", node.Value.(string)[0]))
	case "BoolLiteral":
		if node.Value.(bool) {
			g.code.WriteString("1")
		} else {
			g.code.WriteString("0")
		}
	case "Identifier":
		g.code.WriteString(node.Value.(string))
	case "BinaryOp":
		data := node.Value.(map[string]interface{})
		op := data["operator"].(string)

		if len(node.Children) != 2 {
			return fmt.Errorf("binary op needs 2 children")
		}

		left := node.Children[0]
		right := node.Children[1]

		g.code.WriteString("(")
		if err := g.generateExpression(left); err != nil {
			return err
		}
		g.code.WriteString(" ")
		g.code.WriteString(g.mapOperator(op))
		g.code.WriteString(" ")
		if err := g.generateExpression(right); err != nil {
			return err
		}
		g.code.WriteString(")")
	case "Call":
		data := node.Value.(map[string]interface{})
		name := data["name"].(string)

		g.code.WriteString(fmt.Sprintf("%s(", name))
		for i, arg := range node.Children {
			if i > 0 {
				g.code.WriteString(", ")
			}
			if err := g.generateExpression(arg); err != nil {
				return err
			}
		}
		g.code.WriteString(")")
	default:
		return fmt.Errorf("unknown expression type: %s", node.Type)
	}
	return nil
}

func (g *CodeGenerator) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.code.WriteString("    ")
	}
}

func (g *CodeGenerator) mapType(skorpionType string) string {
	mapping := map[string]string{
		"int":    "int",
		"char":   "char",
		"string": "char*",
		"arr":    "void*", // TODO: Реализовать массивы
		"dict":   "void*", // TODO: Реализовать словари
		"float":  "float",
		"double": "double",
		"bool":   "int",
		"void":   "void",
		"any":    "void*",
		"T":      "void*",
	}
	if cType, ok := mapping[skorpionType]; ok {
		return cType
	}
	return "void*"
}

func (g *CodeGenerator) defaultValue(skorpionType string) string {
	defaults := map[string]string{
		"int":    "0",
		"char":   "0",
		"string": "NULL",
		"arr":    "NULL",
		"dict":   "NULL",
		"float":  "0.0f",
		"double": "0.0",
		"bool":   "0",
		"void":   "",
		"any":    "NULL",
	}
	if val, ok := defaults[skorpionType]; ok {
		return val
	}
	return "0"
}

func (g *CodeGenerator) mapOperator(op string) string {
	mapping := map[string]string{
		"+":  "+",
		"-":  "-",
		"*":  "*",
		"/":  "/",
		"%":  "%",
		"==": "==",
		"!=": "!=",
		"<":  "<",
		">":  ">",
		"<=": "<=",
		">=": ">=",
		"&&": "&&",
		"||": "||",
		"!":  "!",
	}
	if cOp, ok := mapping[op]; ok {
		return cOp
	}
	return op
}
