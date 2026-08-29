package cli

import (
	"fmt"
)

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

func HandleAddProfile(args []string) {
	name, path, err := ParseAddProfile(args)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	pm := NewProfileManager()
	err = pm.AddProfile(name, path)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Profile '%s' added successfully")+" (path: %s)\n", name, path)
}

func HandleEditProfile(args []string) {
	name, newPath, err := ParseEditProfile(args)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	pm := NewProfileManager()
	err = pm.EditProfile(name, newPath)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Profile '%s' updated successfully")+" (new path: %s)\n", name, newPath)
}

func HandleSetProfile(args []string) {
	if len(args) < 2 {
		fmt.Println(Colors.Warning("Usage: skorpion set-profile NAME"))
		return
	}

	name := args[1]
	pm := NewProfileManager()
	err := pm.SetProfile(name)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Current profile set to '%s'\n"), name)
}

func HandleDeleteProfile(args []string) {
	if len(args) < 2 {
		fmt.Println(Colors.Warning("Usage: skorpion del-profile NAME"))
		return
	}

	name := args[1]
	pm := NewProfileManager()
	err := pm.DeleteProfile(name)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	fmt.Printf(Colors.Success("Profile '%s' deleted successfully\n"), name)
}

func HandleProfileList() {
	pm := NewProfileManager()

	fmt.Println(Colors.Bold("ID\tName\tPath"))
	fmt.Println(Colors.Dim("--\t----\t----"))
	for _, p := range pm.Profiles {
		current := ""
		if p.Name == pm.GetCurrentProfile() {
			current = Colors.Green(" (current)")
		}

		// Показываем, найден ли компилятор в PATH
		status := Colors.Green("✓")
		if p.Name != "auto" {
			if !pm.CompilerExists(p.Path) {
				status = Colors.Red("✗")
			}
		}

		fmt.Printf("%d\t%s\t%s %s%s\n", p.ID, p.Name, p.Path, status, current)
	}
}

func HandleCurrentProfile() {
	pm := NewProfileManager()
	current := pm.GetCurrentProfile()
	fmt.Printf("Current profile: %s\n", Colors.Cyan(current))
}

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

func HandleUncolored() {
	Colors.Disable()
	ColorSettings.Enabled = false
	fmt.Println("Colors disabled")
}

func HandleSaveC() string {
	return "temp_skorpion.c"
}

func HandleAST() bool {
	return true
}

func HandleTokens() bool {
	return true
}
