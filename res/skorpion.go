package res

import (
	"fmt"
	"os"
	"path/filepath"
	"skrp/res/backend"
	"skrp/res/cli"
	"skrp/res/errors"
	"skrp/res/front"
	"skrp/res/midlevel"
)

var (
	showTokens bool
	showAST    bool
	saveC      bool
	uncolored  bool
	buildPath  string
)

func InitLang(args []string) {
	errors.InitErrors()
	defer errors.CloseErrors()

	// Парсим флаги
	args = parseFlags(args)

	if len(args) < 1 {
		printHelp()
		return
	}

	switch args[0] {
	// Команды сборки
	case "build":
		buildProject()
	case "test":
		testProject()

	// Команды управления профилями
	case "add-profile":
		cli.HandleAddProfile(args)
	case "edit-profile":
		cli.HandleEditProfile(args)
	case "set-profile":
		cli.HandleSetProfile(args)
	case "del-profile":
		cli.HandleDeleteProfile(args)
	case "--profile-list":
		cli.HandleProfileList()
	case "--current-profile":
		cli.HandleCurrentProfile()

	// Команды настроек
	case "color":
		cli.HandleColor(args)
	case "updates":
		cli.HandleUpdates(args)
	case "--colors":
		cli.HandleShowColors()
	case "--updates":
		cli.HandleShowUpdates()

	// Справка и версия
	case "--help", "-h":
		printHelp()
	case "--version", "-v":
		printVersion()

	default:
		fmt.Printf(cli.Colors.Error("Unknown command: %s\n"), args[0])
		fmt.Println(cli.Colors.Warning("Run 'skorpion --help' for usage"))
	}
}

func parseFlags(args []string) []string {
	var result []string
	showTokens = false
	showAST = false
	saveC = false
	uncolored = false
	buildPath = "."

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "--tokens":
			showTokens = true
		case "--ast":
			showAST = true
		case "--save-c":
			saveC = true
		case "--uncolored":
			uncolored = true
			cli.Colors.Disable()
			cli.ColorSettings.Enabled = false
		default:
			// Проверяем --path=...
			if len(arg) > 7 && arg[:7] == "--path=" {
				buildPath = arg[7:]
				continue
			}
			result = append(result, arg)
		}
	}

	return result
}

func buildProject() {
	fmt.Println(cli.Colors.Bold(cli.Colors.Cyan("Building Skorpion project...")))

	projectPath := buildPath

	// Читаем конфиг
	configPath := filepath.Join(projectPath, "manifest.spc")
	cfg := front.ParseConfig(configPath)
	if cfg == nil {
		errors.NewError("1001", "Cannot read manifest.spc", 0, 0, "manifest.spc")
		errors.PrintErrors()
		return
	}

	// Определяем главный файл
	mainFile := cfg.Main
	if mainFile == "" {
		mainFile = "main.sk"
	}
	mainFile = filepath.Join(projectPath, mainFile)

	// Загружаем программу с импортами
	mainProg, err := front.LoadProgram(mainFile)
	if err != nil {
		errors.NewFatalError("1001", fmt.Sprintf("Import error: %v", err), 0, 0, mainFile)
		errors.PrintErrors()
		os.Exit(1)
		return
	}

	// Если нужно показать токены
	if showTokens {
		fmt.Println(cli.Colors.Bold(cli.Colors.Yellow("\n=== Tokens ===")))
		content, _ := os.ReadFile(mainFile)
		lexer := front.NewLexer(string(content))
		tok := lexer.NextToken()
		for tok.Type != front.TOKEN_EOF {
			fmt.Printf("%s: %q\n", cli.Colors.Cyan(tok.Type.String()), tok.Literal)
			tok = lexer.NextToken()
		}
		fmt.Println()

		if errors.HasFatal() {
			errors.PrintErrors()
			os.Exit(1)
			return
		}
	}

	// Создаём менеджер импортов
	im := front.NewImportManager(projectPath)
	im.LoadMain(mainFile)

	fmt.Printf(cli.Colors.Info("Parsed %d functions\n"), len(mainProg.Functions))

	// Если нужно показать AST
	if showAST {
		fmt.Println(cli.Colors.Bold(cli.Colors.Yellow("\n=== AST ===")))
		printAST(mainProg, 0)
		fmt.Println()
	}

	// СЕМАНТИЧЕСКИЙ АНАЛИЗ
	semantic := midlevel.NewSemanticAnalyzer(mainProg)
	semantic.SetImportManager(im)

	if !semantic.Analyze() {
		errors.PrintErrors()
		os.Exit(1)
		return
	}
	fmt.Println(cli.Colors.Success("Semantic analysis passed"))

	// ОПТИМИЗАЦИЯ — СОБИРАЕМ ВСЕ ФУНКЦИИ (main + импорты)
	allFunctions := make([]*front.Function, len(mainProg.Functions))
	copy(allFunctions, mainProg.Functions)

	// Добавляем функции из импортов
	importedFuncs := im.GetAllFunctions()
	allFunctions = append(allFunctions, importedFuncs...)

	// ОПТИМИЗАЦИЯ — используем ВСЕ функции
	mergedProg := &front.Program{
		Imports:   mainProg.Imports,
		Functions: mainProg.AllFunctions, // ← Все функции!
	}

	mid := midlevel.NewMidLevel(mergedProg)
	optProg := mid.OptimizeIR()
	fmt.Println(cli.Colors.Success("Optimization complete"))

	// Конвейер: AST → IR
	pipeline := backend.NewPipeline(optProg)
	ir := pipeline.Process()
	fmt.Printf(cli.Colors.Info("Generated IR with %d functions\n"), len(ir.Functions))

	// Получаем текущий профиль
	pm := cli.NewProfileManager()
	currentProfile := pm.GetCurrentProfile()
	compilerPath := pm.GetProfilePath(currentProfile)

	// Сборка бинарника
	buildConfig := &backend.BuildConfig{
		Profile:      currentProfile,
		CompilerPath: compilerPath,
		OutputName:   cfg.BuildOutName,
		OutputDir:    cfg.BuildOutPath,
		IsTest:       false,
	}

	if buildConfig.OutputName == "" {
		buildConfig.OutputName = cfg.Name + "_" + cfg.Version
	}
	if buildConfig.OutputDir == "" {
		buildConfig.OutputDir = "bin/"
	}
	if buildConfig.OutputName == "" {
		buildConfig.OutputName = "myapp"
	}

	// Создаём директорию
	if err := os.MkdirAll(buildConfig.OutputDir, 0755); err != nil {
		errors.NewFatalError("2004", fmt.Sprintf("Cannot create output directory: %v", err), 0, 0, "")
		errors.PrintErrors()
		os.Exit(1)
		return
	}

	// Генерация C кода
	fmt.Println("Generating C code...")
	back := backend.NewBackend(optProg)
	cCode := back.GenCFromIR(ir)

	if saveC {
		cFileName := filepath.Join(buildConfig.OutputDir, "output.c")
		if err := os.WriteFile(cFileName, []byte(cCode), 0644); err == nil {
			fmt.Printf(cli.Colors.Info("C code saved to: %s\n"), cFileName)
		}
	}

	// Сборка бинарника
	fmt.Println("Building binary...")
	if !back.Build(ir, buildConfig) {
		errors.PrintErrors()
		os.Exit(1)
		return
	}

	fmt.Println(cli.Colors.Success("Build successful!"))
}

func testProject() {
	fmt.Println(cli.Colors.Bold(cli.Colors.Cyan("Testing Skorpion project...")))
	// TODO: Реализовать тестирование
}

func printAST(node front.Node, indent int) {
	prefix := ""
	for i := 0; i < indent; i++ {
		prefix += "  "
	}

	switch n := node.(type) {
	case *front.Program:
		fmt.Println(prefix + cli.Colors.Bold("Program"))
		for _, imp := range n.Imports {
			printAST(imp, indent+1)
		}
		for _, fn := range n.Functions {
			printAST(fn, indent+1)
		}

	case *front.Import:
		alias := n.Alias
		if alias == "" {
			alias = "none"
		}
		allStr := ""
		if n.All {
			allStr = " (all)"
		}
		fmt.Printf("%s%s %s (alias: %s)%s\n", prefix, cli.Colors.Cyan("Import"), n.Path, alias, allStr)

	case *front.Function:
		exportStr := ""
		if !n.IsExport {
			exportStr = cli.Colors.Dim(" (non-exportable)")
		}
		fmt.Printf("%s%s %s(%s) -> %s%s\n", prefix,
			cli.Colors.Yellow("Function"),
			cli.Colors.Bold(n.Name),
			formatParams(n.Params),
			n.ReturnType,
			exportStr)
		if n.Body != nil {
			printAST(n.Body, indent+1)
		}

	case *front.Block:
		fmt.Println(prefix + cli.Colors.Dim("{ Block }"))
		for _, stmt := range n.Statements {
			printAST(stmt, indent+1)
		}

	case *front.VarDecl:
		initStr := ""
		if n.Expr != nil {
			initStr = " = "
			if num, ok := n.Expr.(*front.Number); ok {
				initStr += num.Value
			} else if str, ok := n.Expr.(*front.String); ok {
				initStr += `"` + str.Value + `"`
			} else {
				initStr += "..."
			}
		}
		fmt.Printf("%s%s %s %s%s\n", prefix,
			cli.Colors.Magenta("Var"),
			n.Type,
			n.Name,
			initStr)

	case *front.Assign:
		exprStr := "..."
		if n.Expr != nil {
			if num, ok := n.Expr.(*front.Number); ok {
				exprStr = num.Value
			} else if str, ok := n.Expr.(*front.String); ok {
				exprStr = `"` + str.Value + `"`
			}
		}
		fmt.Printf("%s%s %s = %s\n", prefix,
			cli.Colors.Magenta("Assign"),
			n.Name,
			exprStr)

	case *front.BinaryExpr:
		leftStr := "..."
		rightStr := "..."
		if num, ok := n.Left.(*front.Number); ok {
			leftStr = num.Value
		}
		if num, ok := n.Right.(*front.Number); ok {
			rightStr = num.Value
		}
		fmt.Printf("%s%s %s %s %s\n", prefix,
			cli.Colors.Cyan("Binary"),
			leftStr,
			cli.Colors.Bold(n.Op),
			rightStr)

	case *front.ReturnStmt:
		exprStr := ""
		if n.Expr != nil {
			if num, ok := n.Expr.(*front.Number); ok {
				exprStr = " " + num.Value
			}
		}
		fmt.Printf("%s%s%s\n", prefix,
			cli.Colors.Yellow("Return"),
			exprStr)

	case *front.IfStmt:
		fmt.Println(prefix + cli.Colors.Yellow("If"))
		if n.Then != nil {
			printAST(n.Then, indent+1)
		}
		if n.Else != nil {
			fmt.Println(prefix + cli.Colors.Dim("Else:"))
			printAST(n.Else, indent+1)
		}

	case *front.WhileStmt:
		fmt.Println(prefix + cli.Colors.Yellow("While"))
		if n.Body != nil {
			printAST(n.Body, indent+1)
		}

	case *front.ForStmt:
		fmt.Println(prefix + cli.Colors.Yellow("For"))
		if n.Body != nil {
			printAST(n.Body, indent+1)
		}

	case *front.Number:
		fmt.Printf("%s%s %s\n", prefix,
			cli.Colors.Green("Number"),
			n.Value)

	case *front.String:
		fmt.Printf("%s%s \"%s\"\n", prefix,
			cli.Colors.Green("String"),
			n.Value)

	case *front.Ident:
		fmt.Printf("%s%s %s\n", prefix,
			cli.Colors.Green("Ident"),
			n.Name)

	case *front.CallExpr:
		argsStr := ""
		for i, arg := range n.Args {
			if i > 0 {
				argsStr += ", "
			}
			if num, ok := arg.(*front.Number); ok {
				argsStr += num.Value
			} else if str, ok := arg.(*front.String); ok {
				argsStr += `"` + str.Value + `"`
			} else {
				argsStr += "..."
			}
		}
		fmt.Printf("%s%s %s(%s)\n", prefix,
			cli.Colors.Cyan("Call"),
			n.Name,
			argsStr)

	default:
		fmt.Printf("%s%s\n", prefix, cli.Colors.Dim("Unknown node"))
	}
}

func formatParams(params []*front.Param) string {
	if len(params) == 0 {
		return ""
	}
	result := ""
	for i, p := range params {
		if i > 0 {
			result += ", "
		}
		result += p.Type + " " + p.Name
	}
	return result
}

func printHelp() {
	fmt.Println(cli.Colors.Bold("Skorpion Compiler v1.0.0"))
	fmt.Println(cli.Colors.Dim("Copyrights. (c) 2026 iamtowvee"))
	fmt.Println()
	fmt.Println(cli.Colors.Bold("USAGE:"))
	fmt.Println("  skorpion <COMMAND> [OPTIONS]")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("COMMANDS:"))
	fmt.Println("  build                 Build project from current directory")
	fmt.Println("  build --path=\"DIR\"    Build project from specified directory")
	fmt.Println("  test                  Run tests")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("PROFILE MANAGEMENT:"))
	fmt.Println("  add-profile NAME : PATH    Add new compiler profile")
	fmt.Println("  edit-profile NAME : PATH   Edit existing compiler profile")
	fmt.Println("  set-profile NAME           Set current compiler profile")
	fmt.Println("  del-profile NAME           Delete compiler profile")
	fmt.Println("  --profile-list             List all compiler profiles")
	fmt.Println("  --current-profile          Show current compiler profile")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("SETTINGS:"))
	fmt.Println("  color [true|false|toggle]  Enable/disable colors")
	fmt.Println("  updates [true|false|toggle] Enable/disable update checks")
	fmt.Println("  --colors                   Show current color setting")
	fmt.Println("  --updates                  Show current update setting")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("DEBUG OPTIONS:"))
	fmt.Println("  --uncolored               Disable all colors")
	fmt.Println("  --save-c                  Save generated C code")
	fmt.Println("  --ast                     Print AST")
	fmt.Println("  --tokens                  Print tokens")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("OTHER:"))
	fmt.Println("  --help, -h                Show this help")
	fmt.Println("  --version, -v             Show version")
}

func printVersion() {
	fmt.Println(cli.Colors.Bold("Skorpion Compiler v1.0.0"))
	fmt.Println(cli.Colors.Dim("Copyrights. (c) 2026 iamtowvee"))
	fmt.Println(cli.Colors.Dim("Distributed under MIT License"))
}
