package cli

import (
	"fmt"
	"skrp/res/debug"
	"strings"
)

// handleProfileMutation — общая часть HandleAddProfile / HandleEditProfile.
//
// Раньше HandleAddProfile и HandleEditProfile дублировали друг друга
// на 90% — отличался только вызов pm.AddProfile/pm.EditProfile
// и слово в сообщении успеха. Здесь — единая точка.
func handleProfileMutation(args []string, osName, verb string) {
	name, path, err := parseProfileArgs(args, osName, verb)
	if err != nil {
		fmt.Println(Colors.Error("Error:"), err)
		return
	}

	pm := NewProfileManager()

	var opErr error
	switch verb {
	case "add":
		opErr = pm.AddProfile(osName, name, path)
	case "edit":
		opErr = pm.EditProfile(osName, name, path)
	default:
		fmt.Println(Colors.Error("Error:"), "unknown profile mutation '"+verb+"'")
		return
	}
	if opErr != nil {
		fmt.Println(Colors.Error("Error:"), opErr)
		return
	}

	fmt.Printf(
		Colors.Success(fmt.Sprintf("Profile '%%s' %sed for %%s", verb))+" (path: %s)\n",
		name, osName, path,
	)
}

func HandleAddProfile(args []string, osName string) {
	handleProfileMutation(args, osName, "add")
}

func HandleEditProfile(args []string, osName string) {
	handleProfileMutation(args, osName, "edit")
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

// parseProfileArgs разбирает "NAME : PATH" из args[1:].
// Раньше была копия в actions.go::parseAddProfileArgs
// и ещё две копии в args.go::ParseAddProfile/ParseEditProfile.
// Теперь — одна.
func parseProfileArgs(args []string, osName, verb string) (string, string, error) {
	if len(args) < 4 {
		return "", "", fmt.Errorf(
			"usage: skorpion %s-%s-profile NAME : PATH", verb, osName)
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
		debug.SetColored(true)
		fmt.Println(Colors.Success("Colors enabled"))
	case "false":
		ColorSettings.Enabled = false
		Colors.Disable()
		debug.SetColored(false)
		fmt.Println("Colors disabled")
	case "toggle":
		ColorSettings.Enabled = !ColorSettings.Enabled
		if ColorSettings.Enabled {
			Colors.Enable()
			debug.SetColored(true)
			fmt.Println(Colors.Success("Colors enabled"))
		} else {
			Colors.Disable()
			debug.SetColored(false)
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
