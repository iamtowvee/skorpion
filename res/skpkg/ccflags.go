package skpkg

import (
	"encoding/json"
	"os"
)

// CCFlagsFileName — имя файла с флагами компилятора.
const CCFlagsFileName = "ccflags.json"

// HasCCFlags возвращает true, если есть хотя бы один флаг.
func HasCCFlags(c CCFlagsConfig) bool {
	return len(c.Linux) > 0 || len(c.Windows) > 0 || len(c.Darwin) > 0
}

// WriteCCFlags записывает флаги в JSON-файл.
func WriteCCFlags(path string, c CCFlagsConfig) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadCCFlags читает флаги из JSON-файла.
func ReadCCFlags(path string) (CCFlagsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CCFlagsConfig{}, err
	}
	var c CCFlagsConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return CCFlagsConfig{}, err
	}
	return c, nil
}
