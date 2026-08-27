package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"skrp/res/front"
)

// GenCFromIR генерирует C код из IR
func GenCFromIR(ir *front.ASTNode) (string, error) {
	generator := NewCodeGenerator()
	return generator.Generate(ir)
}

// Builder компилирует C код в исполняемый файл
func Builder(cFile, outFile, profile string) error {
	// Определяем компилятор
	compiler := "tcc" // по умолчанию TCC

	// Проверяем профиль
	if profile != "auto" {
		// TODO: Загружать путь компилятора из профиля
		// Пока просто используем tcc
	}

	// Создаем директорию для выходного файла
	outDir := filepath.Dir(outFile)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию: %v", err)
	}

	// Компилируем
	cmd := exec.Command(compiler, cFile, "-o", outFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ошибка компиляции: %v", err)
	}

	return nil
}
