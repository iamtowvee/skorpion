package midlevel

import (
	"fmt"
	"skrp/res/errors"
	"skrp/res/front"
	"strings"
)

// SemanticChecker проверяет семантику
type SemanticChecker struct {
	variables map[string]string // name -> type
	functions map[string]string // name -> returnType
	scope     []map[string]string
}

// NewSemanticChecker создает новый семантический чекер
func NewSemanticChecker() *SemanticChecker {
	return &SemanticChecker{
		variables: make(map[string]string),
		functions: make(map[string]string),
		scope:     make([]map[string]string, 0),
	}
}

// Check проверяет AST
func (s *SemanticChecker) Check(ast *front.ASTNode) error {
	s.pushScope()

	for _, node := range ast.Children {
		if err := s.checkNode(node); err != nil {
			return err
		}
	}

	s.popScope()
	return nil
}

func (s *SemanticChecker) checkNode(node *front.ASTNode) error {
	switch node.Type {
	case "Function":
		return s.checkFunction(node)
	case "VariableDeclaration":
		return s.checkVariableDeclaration(node)
	case "Assignment":
		return s.checkAssignment(node)
	case "Call":
		return s.checkCall(node)
	case "Return":
		return s.checkReturn(node)
	case "If":
		return s.checkIf(node)
	case "While":
		return s.checkWhile(node)
	case "Block":
		return s.checkBlock(node)
	case "Use":
		// Проверяем, что модуль существует
		return s.checkUse(node)
	case "IncludeC":
		// includeC не требует проверки
		return nil
	default:
		// Для выражений просто проверяем детей
		for _, child := range node.Children {
			if err := s.checkNode(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SemanticChecker) checkFunction(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	name := data["name"].(string)
	returnType := data["returnType"].(string)

	// Проверяем, что функция не объявлена дважды
	if _, exists := s.functions[name]; exists {
		errors.NewErrorWithPosition(
			errors.ERR_REDECLARED_VAR,
			fmt.Sprintf("Функция '%s' уже объявлена", name),
			node.Line, node.Column, "",
		)
		return fmt.Errorf("function redeclared")
	}

	s.functions[name] = returnType
	s.pushScope()

	// Добавляем параметры в область видимости
	params := data["params"].([]map[string]string)
	for _, param := range params {
		paramType := param["type"]
		paramName := param["name"]

		// Проверяем тип
		if !s.isValidType(paramType) {
			errors.NewErrorWithPosition(
				errors.ERR_INVALID_TYPE,
				fmt.Sprintf("Некорректный тип: %s", paramType),
				node.Line, node.Column, "",
			)
			return fmt.Errorf("invalid type")
		}

		s.variables[paramName] = paramType
	}

	// Проверяем тело функции
	if len(node.Children) > 0 {
		body := node.Children[0]
		if err := s.checkBlock(body); err != nil {
			return err
		}
	}

	s.popScope()
	return nil
}

func (s *SemanticChecker) checkVariableDeclaration(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	varType := data["type"].(string)
	varName := data["name"].(string)

	// Проверяем тип
	if !s.isValidType(varType) {
		errors.NewErrorWithPosition(
			errors.ERR_INVALID_TYPE,
			fmt.Sprintf("Некорректный тип: %s", varType),
			node.Line, node.Column, "",
		)
		return fmt.Errorf("invalid type")
	}

	// Проверяем, что переменная не объявлена дважды
	if _, exists := s.variables[varName]; exists {
		errors.NewErrorWithPosition(
			errors.ERR_REDECLARED_VAR,
			fmt.Sprintf("Переменная '%s' уже объявлена", varName),
			node.Line, node.Column, "",
		)
		return fmt.Errorf("variable redeclared")
	}

	// Проверяем инициализацию
	if len(node.Children) > 0 {
		exprType, err := s.checkExpression(node.Children[0])
		if err != nil {
			return err
		}

		// Проверяем соответствие типов
		if varType != "any" && varType != exprType {
			errors.NewErrorWithPosition(
				errors.ERR_TYPE_MISMATCH,
				fmt.Sprintf("Тип '%s' не соответствует объявленному типу '%s'", exprType, varType),
				node.Line, node.Column, "",
			)
			return fmt.Errorf("type mismatch")
		}
	}

	s.variables[varName] = varType
	return nil
}

func (s *SemanticChecker) checkAssignment(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	varName := data["name"].(string)

	// Проверяем, что переменная объявлена
	varType, exists := s.variables[varName]
	if !exists {
		errors.NewErrorWithPosition(
			errors.ERR_UNDECLARED_VAR,
			fmt.Sprintf("Переменная '%s' не объявлена", varName),
			node.Line, node.Column, "",
		)
		return fmt.Errorf("undeclared variable")
	}

	// Проверяем выражение
	if len(node.Children) > 0 {
		exprType, err := s.checkExpression(node.Children[0])
		if err != nil {
			return err
		}

		// Проверяем соответствие типов
		if varType != "any" && varType != exprType {
			errors.NewErrorWithPosition(
				errors.ERR_TYPE_MISMATCH,
				fmt.Sprintf("Тип '%s' не соответствует типу переменной '%s'", exprType, varType),
				node.Line, node.Column, "",
			)
			return fmt.Errorf("type mismatch")
		}
	}

	return nil
}

func (s *SemanticChecker) checkCall(node *front.ASTNode) error {
	data := node.Value.(map[string]interface{})
	name := data["name"].(string)

	// Проверяем, что функция объявлена
	if _, exists := s.functions[name]; !exists {
		errors.NewErrorWithPosition(
			errors.ERR_UNDECLARED_VAR,
			fmt.Sprintf("Функция '%s' не объявлена", name),
			node.Line, node.Column, "",
		)
		return fmt.Errorf("undeclared function")
	}

	// Проверяем аргументы
	for _, arg := range node.Children {
		if _, err := s.checkExpression(arg); err != nil {
			return err
		}
	}

	return nil
}

func (s *SemanticChecker) checkReturn(node *front.ASTNode) error {
	if len(node.Children) > 0 {
		if _, err := s.checkExpression(node.Children[0]); err != nil {
			return err
		}
	}
	return nil
}

func (s *SemanticChecker) checkIf(node *front.ASTNode) error {
	if len(node.Children) < 2 {
		return fmt.Errorf("if without condition or body")
	}

	// Проверяем условие
	if _, err := s.checkExpression(node.Children[0]); err != nil {
		return err
	}

	// Проверяем блок then
	if err := s.checkNode(node.Children[1]); err != nil {
		return err
	}

	// Проверяем else, если есть
	if len(node.Children) > 2 {
		if err := s.checkNode(node.Children[2]); err != nil {
			return err
		}
	}

	return nil
}

func (s *SemanticChecker) checkWhile(node *front.ASTNode) error {
	if len(node.Children) < 2 {
		return fmt.Errorf("while without condition or body")
	}

	// Проверяем условие
	if _, err := s.checkExpression(node.Children[0]); err != nil {
		return err
	}

	// Проверяем тело
	if err := s.checkNode(node.Children[1]); err != nil {
		return err
	}

	return nil
}

func (s *SemanticChecker) checkBlock(node *front.ASTNode) error {
	s.pushScope()
	for _, child := range node.Children {
		if err := s.checkNode(child); err != nil {
			return err
		}
	}
	s.popScope()
	return nil
}

func (s *SemanticChecker) checkUse(node *front.ASTNode) error {
	// TODO: Проверка существования модуля
	return nil
}

func (s *SemanticChecker) checkExpression(node *front.ASTNode) (string, error) {
	switch node.Type {
	case "NumberLiteral":
		// Определяем int или float
		val := node.Value.(string)
		if strings.Contains(val, ".") {
			return "float", nil
		}
		return "int", nil
	case "StringLiteral":
		return "string", nil
	case "CharLiteral":
		return "char", nil
	case "BoolLiteral":
		return "bool", nil
	case "Identifier":
		name := node.Value.(string)
		if typ, exists := s.variables[name]; exists {
			return typ, nil
		}
		return "", fmt.Errorf("variable not found: %s", name)
	case "BinaryOp":
		if len(node.Children) != 2 {
			return "", fmt.Errorf("binary op requires 2 operands")
		}
		leftType, err := s.checkExpression(node.Children[0])
		if err != nil {
			return "", err
		}
		rightType, err := s.checkExpression(node.Children[1])
		if err != nil {
			return "", err
		}

		// Проверяем, что типы совместимы для операции
		if leftType != rightType && leftType != "any" && rightType != "any" {
			errors.NewErrorWithPosition(
				errors.ERR_INVALID_OPERATION,
				fmt.Sprintf("Операция не поддерживается между типами '%s' и '%s'", leftType, rightType),
				node.Line, node.Column, "",
			)
			return "", fmt.Errorf("invalid operation")
		}
		return leftType, nil
	case "Call":
		// Проверяем, что функция возвращает значение
		data := node.Value.(map[string]interface{})
		name := data["name"].(string)
		if retType, exists := s.functions[name]; exists {
			return retType, nil
		}
		return "", fmt.Errorf("function not found: %s", name)
	default:
		return "", fmt.Errorf("unknown expression type: %s", node.Type)
	}
}

func (s *SemanticChecker) isValidType(typ string) bool {
	validTypes := map[string]bool{
		"int": true, "char": true, "string": true,
		"arr": true, "dict": true, "float": true,
		"double": true, "bool": true, "void": true,
		"any": true, "T": true,
	}
	return validTypes[typ]
}

func (s *SemanticChecker) pushScope() {
	s.scope = append(s.scope, make(map[string]string))
}

func (s *SemanticChecker) popScope() {
	if len(s.scope) > 0 {
		s.scope = s.scope[:len(s.scope)-1]
	}
}
