package cli

import (
	"fmt"
	"strings"
)

func ParseAddProfile(args []string) (string, string, error) {
	if len(args) < 4 {
		return "", "", fmt.Errorf("usage: skorpion add-profile NAME : PATH")
	}

	// Ищем разделитель ":"
	separatorIdx := -1
	for i, arg := range args {
		if arg == ":" {
			separatorIdx = i
			break
		}
	}

	if separatorIdx == -1 || separatorIdx == 0 || separatorIdx == len(args)-1 {
		return "", "", fmt.Errorf("invalid format. Usage: skorpion add-profile NAME : PATH")
	}

	// Имя профиля (все аргументы до ":")
	nameParts := args[1:separatorIdx]
	name := strings.Join(nameParts, " ")

	// Путь (все аргументы после ":")
	pathParts := args[separatorIdx+1:]
	path := strings.Join(pathParts, " ")

	return name, path, nil
}

func ParseEditProfile(args []string) (string, string, error) {
	if len(args) < 4 {
		return "", "", fmt.Errorf("usage: skorpion edit-profile NAME : NEW_PATH")
	}

	separatorIdx := -1
	for i, arg := range args {
		if arg == ":" {
			separatorIdx = i
			break
		}
	}

	if separatorIdx == -1 || separatorIdx == 0 || separatorIdx == len(args)-1 {
		return "", "", fmt.Errorf("invalid format. Usage: skorpion edit-profile NAME : NEW_PATH")
	}

	nameParts := args[1:separatorIdx]
	name := strings.Join(nameParts, " ")

	pathParts := args[separatorIdx+1:]
	path := strings.Join(pathParts, " ")

	return name, path, nil
}
