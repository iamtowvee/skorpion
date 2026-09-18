package cli

import (
	"fmt"
	"strings"
)

func HandleAddProfile(args []string, osName string) {
	name, path, err := parseAddProfileArgs(args)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	pm := NewProfileManager()
	if err := pm.AddProfile(osName, name, path); err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Profile '%s' added for %s")+" (path: %s)\n", name, osName, path)
}

func HandleEditProfile(args []string, osName string) {
	name, newPath, err := parseAddProfileArgs(args)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	pm := NewProfileManager()
	if err := pm.EditProfile(osName, name, newPath); err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Profile '%s' updated for %s")+" (new path: %s)\n", name, osName, newPath)
}

func HandleSetProfile(args []string, osName string) {
	if len(args) < 2 {
		fmt.Println(Colors.Warning("Usage: skorpion set-" + osName + "-profile NAME"))
		return
	}

	name := args[1]
	pm := NewProfileManager()
	if err := pm.SetProfile(osName, name); err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Current %s profile set to '%s'\n"), osName, name)
}

func HandleDeleteProfile(args []string, osName string) {
	if len(args) < 2 {
		fmt.Println(Colors.Warning("Usage: skorpion del-" + osName + "-profile NAME"))
		return
	}

	name := args[1]
	pm := NewProfileManager()
	if err := pm.DeleteProfile(osName, name); err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Profile '%s' deleted for %s\n"), name, osName)
}

func HandleProfileList(osName string) {
	pm := NewProfileManager()
	fmt.Printf(Colors.Bold("Profiles for %s:\n"), osName)
	pm.ListProfiles(osName)
}

func HandleCurrentProfile(osName string) {
	pm := NewProfileManager()
	pm.ShowCurrentProfile(osName)
}

func parseAddProfileArgs(args []string) (string, string, error) {
	if len(args) < 4 {
		return "", "", fmt.Errorf("usage: NAME : PATH")
	}

	sep := -1
	for i, arg := range args {
		if arg == ":" {
			sep = i
			break
		}
	}

	if sep == -1 || sep == 0 || sep == len(args)-1 {
		return "", "", fmt.Errorf("invalid format, use: NAME : PATH")
	}

	name := strings.Join(args[1:sep], " ")
	path := strings.Join(args[sep+1:], " ")

	return name, path, nil
}

// ============ Старые функции (color, updates) ============

type ColorConfig struct {
	Enabled bool
}

type UpdateConfig struct {
	Enabled bool
}

var (
	ColorSettings  = ColorConfig{Enabled: true}
	UpdateSettings = UpdateConfig{Enabled: true}
)

func HandleColor(args []string) {
	if len(args) < 2 {
		fmt.Println(Colors.Warning("Usage: skorpion color [true|false|toggle]"))
		return
	}

	switch args[1] {
	case "true":
		ColorSettings.Enabled = true
		Colors.Enable()
		fmt.Println(Colors.Success("Colors enabled"))
	case "false":
		ColorSettings.Enabled = false
		Colors.Disable()
		fmt.Println("Colors disabled")
	case "toggle":
		ColorSettings.Enabled = !ColorSettings.Enabled
		if ColorSettings.Enabled {
			Colors.Enable()
			fmt.Println(Colors.Success("Colors enabled"))
		} else {
			Colors.Disable()
			fmt.Println("Colors disabled")
		}
	default:
		fmt.Println(Colors.Error("Invalid value. Use: true, false, or toggle"))
	}
}

func HandleUpdates(args []string) {
	if len(args) < 2 {
		fmt.Println(Colors.Warning("Usage: skorpion updates [true|false|toggle]"))
		return
	}

	switch args[1] {
	case "true":
		UpdateSettings.Enabled = true
		fmt.Println(Colors.Success("Updates enabled"))
	case "false":
		UpdateSettings.Enabled = false
		fmt.Println("Updates disabled")
	case "toggle":
		UpdateSettings.Enabled = !UpdateSettings.Enabled
		if UpdateSettings.Enabled {
			fmt.Println(Colors.Success("Updates enabled"))
		} else {
			fmt.Println("Updates disabled")
		}
	default:
		fmt.Println(Colors.Error("Invalid value. Use: true, false, or toggle"))
	}
}

func HandleShowColors() {
	fmt.Printf("Colors: %s\n",
		map[bool]string{true: Colors.Green("true"), false: "false"}[ColorSettings.Enabled])
}

func HandleShowUpdates() {
	fmt.Printf("Updates: %s\n",
		map[bool]string{true: Colors.Green("true"), false: "false"}[UpdateSettings.Enabled])
}
