package res

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"skrp/res/backend"
	"skrp/res/cli"
	"skrp/res/errors"
	"skrp/res/front"
	"skrp/res/midlevel"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// currentSkorpionVersion — текущая версия Skorpion.
const currentSkorpionVersion = "1.0.0"

// skorpionRepo — GitHub-репозиторий для проверки обновлений.
// Формат: "<owner>/<repo>".
const skorpionRepo = "iamtowvee/skorpion"

// Braille-спиннер: 8 кадров, полный цикл за 0.5 сек
var spinnerFrames = []string{"⠧", "⠏", "⠋", "⠽", "⠼", "⠓", "⠋", "⠏"}
var spinnerInterval = 500 * time.Millisecond / time.Duration(len(spinnerFrames))

// ============================================================
// Update check state
// ============================================================

var (
	zigCheckResult      *zigCheck
	zigCheckMu          sync.Mutex
	skorpionCheckResult *skorpionCheck
	skorpionCheckMu     sync.Mutex
)

type zigCheck struct {
	Installed string // установленная версия, "" если нет
	Latest    string // последняя версия, "" если не удалось получить
	HasLocal  bool   // установлен ли Zig локально
	HasSystem bool   // найден ли Zig в PATH
}

type skorpionCheck struct {
	Current string // текущая версия Skorpion
	Latest  string // последняя версия с GitHub, "" если не удалось
}

// ============================================================
// Progress bar
// ============================================================

type progressWriter struct {
	total      int64
	written    int64
	barWidth   int
	startTime  time.Time
	stopChan   chan struct{}
	doneChan   chan struct{}
	lastRender atomic.Int64
}

func newProgressWriter(total int64) *progressWriter {
	return &progressWriter{
		total:     total,
		barWidth:  10,
		startTime: time.Now(),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	atomic.AddInt64(&pw.written, int64(n))
	return n, nil
}

func (pw *progressWriter) Start() {
	go pw.renderLoop()
}

func (pw *progressWriter) Stop() {
	close(pw.stopChan)
	<-pw.doneChan
	pw.render(100)
	fmt.Println()
}

func (pw *progressWriter) renderLoop() {
	defer close(pw.doneChan)

	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-pw.stopChan:
			return
		case <-ticker.C:
			written := atomic.LoadInt64(&pw.written)
			var percent int
			if pw.total > 0 {
				percent = int(written * 100 / pw.total)
			}
			pw.renderWithSpinner(percent, spinnerFrames[frame])
			frame = (frame + 1) % len(spinnerFrames)
		}
	}
}

func (pw *progressWriter) render(percent int) {
	pw.renderWithSpinner(percent, " ")
}

func (pw *progressWriter) renderWithSpinner(percent int, spinner string) {
	if percent > 100 {
		percent = 100
	}
	if percent < 0 {
		percent = 0
	}

	filled := percent * pw.barWidth / 100
	bar := strings.Repeat("|", filled) + strings.Repeat(".", pw.barWidth-filled)

	fmt.Printf("\r%s [%s] %d%%", spinner, bar, percent)
}

// downloadWithProgress — скачивает URL в dst с прогресс-баром.
func downloadWithProgress(url, dst string) error {
	client := &http.Client{Timeout: 60 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	pw := newProgressWriter(resp.ContentLength)
	pw.Start()
	defer pw.Stop()

	_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
	return err
}

// ============================================================
// Flags
// ============================================================

var (
	showTokens        bool
	showAST           bool
	saveC             bool
	uncolored         bool
	noOptimize        bool
	buildPath         string
	noCheckZigUpdates bool
	noCheckUpdates    bool
)

// ============================================================
// Entry point
// ============================================================

func InitLang(args []string) {
	errors.InitErrors()
	defer errors.CloseErrors()

	// 1. Предварительный проход: ищем --path=... (до парсинга флагов)
	projectPath := "."
	for _, arg := range args {
		if strings.HasPrefix(arg, "--path=") {
			projectPath = arg[7:]
			break
		}
	}

	// 2. Парсим флаги (нужно ДО проверок — чтобы знать, отключены ли они)
	args = parseFlags(args)

	// 3. Запускаем фоновые проверки
	if !noCheckZigUpdates {
		go checkZigInBackground()
	}
	if !noCheckUpdates {
		go checkSkorpionInBackground()
	}

	if len(args) < 1 {
		printHelp()
		return
	}

	// 4. Читаем manifest.spc (если он есть)
	configPath := filepath.Join(projectPath, "manifest.spc")
	cfg := front.ParseConfig(configPath)

	// 5. Применяем env
	if cfg != nil && len(cfg.Env) > 0 {
		for _, envVar := range cfg.Env {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}

	// 6. Добавляем execute в конец args
	if cfg != nil && len(cfg.Execute) > 0 {
		args = append(args, cfg.Execute...)
	}

	switch args[0] {
	case "build":
		buildProject()
	case "test":
		testProject()
	case "setup-zig":
		setupZig(args[1:])
	case "--explain", "-e":
		if len(args) > 1 {
			errors.ExplainError(args[1])
		} else {
			fmt.Println("Usage: skorpion --explain <error-code>")
			fmt.Println("Example: skorpion --explain Err+1043")
		}

	case "add-win-profile":
		cli.HandleAddProfile(args, "windows")
	case "edit-win-profile":
		cli.HandleEditProfile(args, "windows")
	case "set-win-profile":
		cli.HandleSetProfile(args, "windows")
	case "del-win-profile":
		cli.HandleDeleteProfile(args, "windows")
	case "--win-profile-list":
		cli.HandleProfileList("windows")
	case "--current-win-profile":
		cli.HandleCurrentProfile("windows")

	case "add-linux-profile":
		cli.HandleAddProfile(args, "linux")
	case "edit-linux-profile":
		cli.HandleEditProfile(args, "linux")
	case "set-linux-profile":
		cli.HandleSetProfile(args, "linux")
	case "del-linux-profile":
		cli.HandleDeleteProfile(args, "linux")
	case "--linux-profile-list":
		cli.HandleProfileList("linux")
	case "--current-linux-profile":
		cli.HandleCurrentProfile("linux")

	case "color":
		cli.HandleColor(args)
	case "updates":
		cli.HandleUpdates(args)
	case "--colors":
		cli.HandleShowColors()
	case "--updates":
		cli.HandleShowUpdates()

	case "--help", "-h":
		printHelp()
	case "--version", "-v":
		printVersion()

	default:
		fmt.Printf(cli.Colors.Error("Unknown command: %s\n"), args[0])
		fmt.Println(cli.Colors.Warning("Run 'skorpion --help' for usage"))
	}

	printUpdateWarnings()
}

// ============================================================
// Flag parsing
// ============================================================

func parseFlags(args []string) []string {
	var result []string
	showTokens = false
	showAST = false
	saveC = false
	uncolored = false
	noOptimize = false
	buildPath = "."
	noCheckZigUpdates = false
	noCheckUpdates = false

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
		case "--no-optimize", "-N":
			noOptimize = true
		case "--noCheckZigUpdates":
			noCheckZigUpdates = true
		case "--noCheckUpdates":
			noCheckUpdates = true
		default:
			if len(arg) > 7 && arg[:7] == "--path=" {
				buildPath = arg[7:]
				continue
			}
			result = append(result, arg)
		}
	}

	return result
}

// ============================================================
// setup-zig command
// ============================================================

func setupZig(args []string) {
	startTime := time.Now()

	force := false
	for _, a := range args {
		if a == "--force" {
			force = true
		}
	}

	fmt.Println(cli.Colors.Bold(cli.Colors.Cyan("Setting up Zig...")))

	zigDir, err := zigInstallDir()
	if err != nil {
		errors.NewFatalError("3100", fmt.Sprintf("Cannot determine Zig install dir: %v", err), 0, 0, "")
		printErrorReport(startTime, "", nil)
		os.Exit(1)
		return
	}

	zigPath := filepath.Join(zigDir, zigExeName())

	var installedVersion string
	if _, err := os.Stat(zigPath); err == nil {
		installedVersion, _ = getZigVersion(zigPath)
	}

	fmt.Println(cli.Colors.Info("Checking latest Zig version..."))
	latestVersion, err := fetchLatestZigVersion()
	if err != nil {
		fmt.Println(cli.Colors.Warning("Cannot fetch latest version: " + err.Error()))

		if installedVersion != "" && !force {
			fmt.Printf(cli.Colors.Success("Zig already installed: %s\n"), installedVersion)
			return
		}

		errors.NewFatalError("3103",
			"Cannot determine Zig version to download and no local Zig found", 0, 0, "")
		printErrorReport(startTime, "", nil)
		os.Exit(1)
		return
	}

	fmt.Printf(cli.Colors.Info("Latest Zig: %s\n"), latestVersion)

	if installedVersion != "" && !force {
		if compareVersions(installedVersion, latestVersion) >= 0 {
			fmt.Printf(cli.Colors.Success("Zig already installed: %s\n"), installedVersion)
			fmt.Printf(cli.Colors.Info("Use --force to re-download.\n"))
			return
		}

		fmt.Println(cli.BG_YELLOW + cli.BLACK + " WARNING " + cli.RESET +
			" New zig version available: " +
			cli.BRIGHT_WHITE + latestVersion + cli.RESET +
			cli.BRIGHT_BLACK + " (current: " + installedVersion + ")" + cli.RESET)
		fmt.Println(cli.Colors.Info("Updating..."))
	}

	if err := downloadZigWithFallback(zigDir, latestVersion); err != nil {
		errors.NewFatalError("3101", fmt.Sprintf("Failed to download Zig: %v", err), 0, 0, "")
		printErrorReport(startTime, "", nil)
		os.Exit(1)
		return
	}

	fmt.Println(cli.Colors.Success("Zig installed successfully!"))
	fmt.Println(cli.Colors.Info(fmt.Sprintf("Location: %s", zigDir)))
}

// ============================================================
// build command
// ============================================================

func buildProject() {
	startTime := time.Now()

	fmt.Println(cli.Colors.Bold(cli.Colors.Cyan("Building Skorpion project...")))

	projectPath := buildPath

	configPath := filepath.Join(projectPath, "manifest.spc")
	cfg := front.ParseConfig(configPath)
	if cfg == nil {
		errors.NewFatalError("0010", "Cannot read manifest.spc", 0, 0, "manifest.spc")
		printErrorReport(startTime, "manifest.spc", nil)
		return
	}

	mainFile := cfg.Main
	if mainFile == "" {
		mainFile = "main.sk"
	}
	mainFile = filepath.Join(projectPath, mainFile)

	content, err := os.ReadFile(mainFile)
	if err != nil {
		errors.NewFatalError("0011", fmt.Sprintf("Cannot read %s", mainFile), 0, 0, mainFile)
		printErrorReport(startTime, mainFile, nil)
		return
	}
	sourceLines := strings.Split(string(content), "\n")

	mainProg, err := front.LoadProgram(mainFile)
	if err != nil {
		errors.NewFatalError("0011", fmt.Sprintf("Import error: %v", err), 0, 0, mainFile)
		printErrorReport(startTime, mainFile, sourceLines)
		os.Exit(1)
		return
	}

	if showTokens {
		fmt.Println(cli.Colors.Bold(cli.Colors.Yellow("\n=== Tokens ===")))
		lexer := front.NewLexer(string(content))
		tok := lexer.NextToken()
		for tok.Type != front.TOKEN_EOF {
			fmt.Printf("%s: %q\n", cli.Colors.Cyan(tok.Type.String()), tok.Literal)
			tok = lexer.NextToken()
		}
		fmt.Println()

		if errors.HasFatal() {
			printErrorReport(startTime, mainFile, sourceLines)
			os.Exit(1)
			return
		}
	}

	im := front.NewImportManager(projectPath)
	im.LoadMain(mainFile)

	fmt.Printf(cli.Colors.Info("Parsed %d functions\n"), len(mainProg.Functions))

	if showAST {
		fmt.Println(cli.Colors.Bold(cli.Colors.Yellow("\n=== AST ===")))
		printAST(mainProg, 0)
		fmt.Println()
	}

	semantic := midlevel.NewSemanticAnalyzer(mainProg)
	semantic.SetImportManager(im)

	if !semantic.Analyze() {
		if len(semantic.Errors) > 0 {
			for _, e := range semantic.Errors {
				code := e.Code
				if code == "" {
					code = "0000"
				}
				errors.NewError(code, e.Message, e.Line, e.Column, e.File)
			}
		}
		printErrorReport(startTime, mainFile, sourceLines)
		os.Exit(1)
		return
	}
	fmt.Println(cli.Colors.Success("Semantic analysis passed"))

	allFunctions := make([]*front.Function, len(mainProg.Functions))
	copy(allFunctions, mainProg.Functions)

	allImportedFunctions := im.GetAllFunctionsInternal()
	allFunctions = append(allFunctions, allImportedFunctions...)

	mergedProg := &front.Program{
		Imports:        mainProg.Imports,
		Functions:      allFunctions,
		ErrorDecls:     mainProg.ErrorDecls,
		GlobalIncludeC: mainProg.GlobalIncludeC,
		AllFunctions:   allFunctions,
	}

	var optProg *front.Program
	if noOptimize {
		optProg = mergedProg
		fmt.Println(cli.Colors.Warning("Optimization skipped (--no-optimize)"))
	} else {
		mid := midlevel.NewMidLevel(mergedProg)
		optProg = mid.OptimizeIR()
		fmt.Println(cli.Colors.Success("Optimization complete"))
	}

	pipeline := backend.NewPipeline(optProg)
	ir := pipeline.Process()
	fmt.Printf(cli.Colors.Info("Generated IR with %d functions\n"), len(ir.Functions))

	pm := cli.NewProfileManager()

	buildConfig := &backend.BuildConfig{
		ProfileManager: pm,
		Targets:        cfg.Target,
		OutputName:     cfg.BuildOutName,
		OutputDir:      cfg.BuildOutPath,
		IsTest:         false,
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

	if err := os.MkdirAll(buildConfig.OutputDir, 0755); err != nil {
		errors.NewFatalError("0022", fmt.Sprintf("Cannot create output directory: %v", err), 0, 0, "")
		printErrorReport(startTime, mainFile, sourceLines)
		os.Exit(1)
		return
	}

	fmt.Println("Generating C code...")
	back := backend.NewBackend(optProg)
	cCode := back.GenCFromIR(ir)

	if saveC {
		cFileName := filepath.Join(buildConfig.OutputDir, "output.c")
		if err := os.WriteFile(cFileName, []byte(cCode), 0644); err == nil {
			fmt.Printf(cli.Colors.Info("C code saved to: %s\n"), cFileName)
		}
	}

	fmt.Println("Building binary...")
	if !back.Build(ir, buildConfig) {
		printErrorReport(startTime, mainFile, sourceLines)
		os.Exit(1)
		return
	}

	if errors.HasWarnings() {
		printErrorReport(startTime, mainFile, sourceLines)
	}

	fmt.Println(cli.Colors.Success("Build successful!"))
}

func testProject() {
	fmt.Println(cli.Colors.Bold(cli.Colors.Cyan("Testing Skorpion project...")))
	// TODO: Реализовать тестирование
}

func printErrorReport(startTime time.Time, filePath string, sourceLines []string) {
	report := errors.ErrorReport{
		Errors:     errors.TakeErrorsList(),
		Warnings:   errors.TakeWarningsList(),
		FilePath:   filePath,
		SourceCode: sourceLines,
		TotalTime:  time.Since(startTime),
	}
	errors.PrintErrorReport(report)
}

// ============================================================
// Update checks
// ============================================================

// checkZigInBackground — тихо проверяет Zig в фоне.
func checkZigInBackground() {
	result := &zigCheck{}

	// 1. Локальный Zig
	if zigDir, err := zigInstallDir(); err == nil {
		zigPath := filepath.Join(zigDir, zigExeName())
		if _, err := os.Stat(zigPath); err == nil {
			if v, err := getZigVersion(zigPath); err == nil {
				result.Installed = v
				result.HasLocal = true
			}
		}
	}

	// 2. Zig в PATH
	if !result.HasLocal {
		if zigPath, err := exec.LookPath(zigExeName()); err == nil {
			if v, err := getZigVersion(zigPath); err == nil {
				result.Installed = v
				result.HasSystem = true
			}
		}
	}

	// 3. Последняя версия
	if v, err := fetchLatestZigVersion(); err == nil {
		result.Latest = v
	}

	zigCheckMu.Lock()
	zigCheckResult = result
	zigCheckMu.Unlock()
}

// checkSkorpionInBackground — тихо проверяет обновления Skorpion через GitHub API.
func checkSkorpionInBackground() {
	result := &skorpionCheck{
		Current: currentSkorpionVersion,
	}

	client := &http.Client{Timeout: 15 * time.Second}
	apiURL := "https://api.github.com/repos/" + skorpionRepo + "/releases/latest"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		skorpionCheckMu.Lock()
		skorpionCheckResult = result
		skorpionCheckMu.Unlock()
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Skorpion-Compiler")

	resp, err := client.Do(req)
	if err != nil {
		skorpionCheckMu.Lock()
		skorpionCheckResult = result
		skorpionCheckMu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		skorpionCheckMu.Lock()
		skorpionCheckResult = result
		skorpionCheckMu.Unlock()
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		skorpionCheckMu.Lock()
		skorpionCheckResult = result
		skorpionCheckMu.Unlock()
		return
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	result.Latest = latest

	skorpionCheckMu.Lock()
	skorpionCheckResult = result
	skorpionCheckMu.Unlock()
}

// printUpdateWarnings — выводит оба предупреждения (Zig и Skorpion), если нужно.
func printUpdateWarnings() {
	printZigWarning()
	printSkorpionWarning()
}

func printZigWarning() {
	zigCheckMu.Lock()
	r := zigCheckResult
	zigCheckMu.Unlock()

	if r == nil {
		return
	}

	var msg string

	switch {
	case !r.HasLocal && !r.HasSystem:
		msg = "Zig is not installed. Run 'skorpion setup-zig' to download it."

	case r.Latest == "":
		return

	case compareVersions(r.Installed, r.Latest) < 0:
		msg = "New Zig version available: " +
			cli.BRIGHT_WHITE + r.Latest + cli.RESET +
			cli.BRIGHT_BLACK + " (current: " + r.Installed + ")" + cli.RESET +
			" — run 'skorpion setup-zig' to update."

	default:
		return
	}

	printDebug(msg)
}

func printSkorpionWarning() {
	skorpionCheckMu.Lock()
	r := skorpionCheckResult
	skorpionCheckMu.Unlock()

	if r == nil || r.Latest == "" {
		return
	}
	if compareVersions(r.Current, r.Latest) >= 0 {
		return
	}

	msg := "New Skorpion version available: " +
		cli.BRIGHT_WHITE + r.Latest + cli.RESET +
		cli.BRIGHT_BLACK + " (current: " + r.Current + ")" + cli.RESET

	printDebug(msg)
}

// printDebug — выводит строку в стиле [ DEBUG ] <msg>.
func printDebug(msg string) {
	prefix := cli.Colors.Colorize(cli.BRIGHT_BLACK+cli.BG_BLACK, " DEBUG ")
	fmt.Printf("%s %s\n", prefix, msg)
}

// ============================================================
// Version comparison
// ============================================================

// compareVersions — простое сравнение версий.
// Возвращает -1, если a < b; 0, если равно; 1, если a > b.
func compareVersions(a, b string) int {
	aParts := splitVersion(a)
	bParts := splitVersion(b)

	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// splitVersion — "0.13.0-dev.123" → [0, 13, 0]
func splitVersion(v string) []int {
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	return nums
}

// ============================================================
// Zig install / download
// ============================================================

func zigInstallDir() (string, error) {
	var base string

	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
		return filepath.Join(base, "Skorpion", "zig"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "skorpion", "zig"), nil
	}
}

func zigExeName() string {
	if runtime.GOOS == "windows" {
		return "zig.exe"
	}
	return "zig"
}

func getZigVersion(zigPath string) (string, error) {
	cmd := exec.Command(zigPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// fetchLatestZigVersion — получает последнюю стабильную версию Zig с index.json.
func fetchLatestZigVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Get("https://ziglang.org/download/index.json")
	if err != nil {
		return "", fmt.Errorf("cannot fetch index.json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("index.json: HTTP %d", resp.StatusCode)
	}

	var index map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return "", fmt.Errorf("cannot parse index.json: %w", err)
	}

	var versions []string
	for key := range index {
		if key == "master" {
			continue
		}
		if strings.Contains(key, "-") {
			continue
		}
		if len(key) == 0 || key[0] < '0' || key[0] > '9' {
			continue
		}
		versions = append(versions, key)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no stable versions in index.json")
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})

	return versions[len(versions)-1], nil
}

// zigDownloadURLFor — возвращает URL архива для указанной версии под текущую ОС/архитектуру.
func zigDownloadURLFor(version string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Get("https://ziglang.org/download/index.json")
	if err != nil {
		return "", fmt.Errorf("cannot fetch index.json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("index.json: HTTP %d", resp.StatusCode)
	}

	var index map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return "", fmt.Errorf("cannot parse index.json: %w", err)
	}

	versionData, ok := index[version]
	if !ok {
		return "", fmt.Errorf("version %s not found in index.json", version)
	}

	platformKey := zigPlatformKey()

	filesRaw, ok := versionData[platformKey]
	if !ok {
		return "", fmt.Errorf("platform %s not found for version %s", platformKey, version)
	}

	files, ok := filesRaw.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid files entry for %s", platformKey)
	}

	tarballRaw, ok := files["tarball"]
	if !ok {
		return "", fmt.Errorf("no tarball for %s", platformKey)
	}

	tarball, ok := tarballRaw.(string)
	if !ok {
		return "", fmt.Errorf("tarball is not a string")
	}

	return tarball, nil
}

func zigPlatformKey() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	case "386":
		arch = "x86"
	}

	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}

	return arch + "-" + osName
}

func fetchMirrorList() []string {
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Get("https://ziglang.org/download/community-mirrors.txt")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var mirrors []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			mirrors = append(mirrors, line)
		}
	}
	return mirrors
}

// downloadZigWithFallback — пробует официальный сайт, потом зеркала.
func downloadZigWithFallback(targetDir, version string) error {
	officialURL, err := zigDownloadURLFor(version)
	if err != nil {
		return fmt.Errorf("cannot determine download URL: %w", err)
	}

	candidates := []string{officialURL}

	mirrors := fetchMirrorList()
	if len(mirrors) > 0 {
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(mirrors), func(i, j int) {
			mirrors[i], mirrors[j] = mirrors[j], mirrors[i]
		})

		u, err := url.Parse(officialURL)
		if err != nil {
			return fmt.Errorf("cannot parse URL: %w", err)
		}
		path := u.Path

		for _, m := range mirrors {
			m = strings.TrimRight(m, "/")
			candidates = append(candidates, m+path)
		}
	}

	var lastErr error

	for i, downloadURL := range candidates {
		fmt.Printf("  [%d/%d] %s\n", i+1, len(candidates), downloadURL)

		tmpFile, err := os.CreateTemp("", "zig-*"+zigArchiveExt())
		if err != nil {
			return err
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		err = downloadWithProgress(downloadURL, tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			lastErr = err
			fmt.Printf("    failed: %v\n", err)
			continue
		}

		if err := os.RemoveAll(targetDir); err != nil {
			os.Remove(tmpPath)
			return err
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			os.Remove(tmpPath)
			return err
		}

		fmt.Println("  Extracting...")
		if err := extractArchive(tmpPath, targetDir); err != nil {
			os.Remove(tmpPath)
			lastErr = err
			fmt.Printf("    extract failed: %v\n", err)
			continue
		}

		os.Remove(tmpPath)
		return nil
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no working mirrors found")
}

func zigArchiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.xz"
}

func extractArchive(archive, targetDir string) error {
	switch {
	case strings.HasSuffix(archive, ".zip"):
		return extractZip(archive, targetDir)
	case strings.HasSuffix(archive, ".tar.xz"):
		return extractTarXz(archive, targetDir)
	}
	return fmt.Errorf("unknown archive format: %s", archive)
}

func extractZip(archive, targetDir string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()

	var rootPrefix string
	if len(r.File) > 0 {
		rootPrefix = strings.SplitN(r.File[0].Name, "/", 2)[0] + "/"
	}

	for _, f := range r.File {
		name := strings.TrimPrefix(f.Name, rootPrefix)
		if name == "" {
			continue
		}
		outPath := filepath.Join(targetDir, name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(outPath, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(outPath), 0755)
		out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarXz(archive, targetDir string) error {
	cmd := exec.Command("tar", "-xJf", archive, "-C", targetDir, "--strip-components=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ============================================================
// AST printing (без изменений)
// ============================================================

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
		for _, errDecl := range n.ErrorDecls {
			printAST(errDecl, indent+1)
		}
		for _, fn := range n.Functions {
			printAST(fn, indent+1)
		}

	case *front.ErrorDecl:
		fmt.Printf("%s%s %s", prefix,
			cli.Colors.Magenta("ErrorDecl"),
			cli.Colors.Bold(n.Name))
		if n.IsNew {
			fmt.Printf(" = new %s", n.Parent)
		} else {
			fmt.Printf(" = %s", n.Parent)
		}
		fmt.Println()
		for _, field := range n.Fields {
			defStr := ""
			if field.DefaultValue != nil {
				if num, ok := field.DefaultValue.(*front.Number); ok {
					defStr = "[" + num.Value + "]"
				} else if str, ok := field.DefaultValue.(*front.String); ok {
					defStr = `["` + str.Value + `"]`
				} else {
					defStr = "[...]"
				}
			}
			fmt.Printf("%s  %s: %s%s\n", prefix,
				cli.Colors.Cyan(field.Name),
				field.Type,
				defStr)
		}

	case *front.TryStmt:
		fmt.Println(prefix + cli.Colors.Yellow("Try"))
		if n.Body != nil {
			printAST(n.Body, indent+1)
		}
		for _, clause := range n.Catches {
			fmt.Println(prefix + cli.Colors.Yellow("Catch"))
			if clause.TypeName != "" {
				fmt.Printf("%s  Type: %s\n", prefix, cli.Colors.Cyan(clause.TypeName))
			}
			if clause.VarName != "" {
				fmt.Printf("%s  As: %s\n", prefix, cli.Colors.Green(clause.VarName))
			}
			if clause.Body != nil {
				printAST(clause.Body, indent+1)
			}
		}

	case *front.ThrowStmt:
		fmt.Println(prefix + cli.Colors.Red("Throw"))
		if n.Expr != nil {
			printAST(n.Expr, indent+1)
		}

	case *front.ErrorInstance:
		fmt.Printf("%s%s %s\n", prefix,
			cli.Colors.Magenta("ErrorInstance"),
			cli.Colors.Bold(n.TypeName))
		for name, value := range n.Fields {
			fmt.Printf("%s  %s:\n", prefix, cli.Colors.Cyan(name))
			printAST(value, indent+2)
		}

	case *front.FieldAccess:
		fmt.Printf("%s%s %s.%s\n", prefix,
			cli.Colors.Cyan("FieldAccess"),
			n.Object,
			n.Field)

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

	case *front.RangeExpr:
		fmt.Printf("%s%s\n", prefix, cli.Colors.Cyan("RangeExpr"))

	case *front.CallRangeExpr:
		fmt.Printf("%s%s %s(...)\n", prefix, cli.Colors.Cyan("CallRange"), n.Name)

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
		for _, elsif := range n.Elsifs {
			fmt.Println(prefix + cli.Colors.Yellow("Elsif"))
			if elsif.Then != nil {
				printAST(elsif.Then, indent+1)
			}
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

	case *front.ForInStmt:
		fmt.Printf("%s%s %s %s in ...\n", prefix,
			cli.Colors.Yellow("ForIn"),
			n.VarType,
			n.VarName)
		if n.Iterable != nil {
			printAST(n.Iterable, indent+1)
		}
		if n.Body != nil {
			printAST(n.Body, indent+1)
		}

	case *front.IknowIdoBlock:
		fmt.Println(prefix + cli.Colors.Magenta("IknowIdo"))
		if n.Body != nil {
			printAST(n.Body, indent+1)
		}

	case *front.CaseStmt:
		fmt.Println(prefix + cli.Colors.Yellow("Case"))
		for _, branch := range n.Branches {
			fmt.Printf("%s  Patterns: ", prefix)
			for i, pat := range branch.Patterns {
				if i > 0 {
					fmt.Print(", ")
				}
				printAST(pat, 0)
			}
			if branch.Body != nil {
				printAST(branch.Body, indent+2)
			}
		}
		if n.Default != nil {
			fmt.Println(prefix + cli.Colors.Yellow("  Default:"))
			printAST(n.Default, indent+2)
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

// ============================================================
// Help & version
// ============================================================

func printHelp() {
	fmt.Println(cli.Colors.Bold("Skorpion Compiler v" + currentSkorpionVersion))
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
	fmt.Println(cli.Colors.Bold("WINDOWS PROFILE MANAGEMENT:"))
	fmt.Println("  add-win-profile NAME : PATH    Add new Windows compiler profile")
	fmt.Println("  edit-win-profile NAME : PATH   Edit existing Windows compiler profile")
	fmt.Println("  set-win-profile NAME           Set current Windows compiler profile")
	fmt.Println("  del-win-profile NAME           Delete Windows compiler profile")
	fmt.Println("  --win-profile-list             List all Windows compiler profiles")
	fmt.Println("  --current-win-profile          Show current Windows compiler profile")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("LINUX PROFILE MANAGEMENT:"))
	fmt.Println("  add-linux-profile NAME : PATH    Add new Linux compiler profile")
	fmt.Println("  edit-linux-profile NAME : PATH   Edit existing Linux compiler profile")
	fmt.Println("  set-linux-profile NAME           Set current Linux compiler profile")
	fmt.Println("  del-linux-profile NAME           Delete Linux compiler profile")
	fmt.Println("  --linux-profile-list             List all Linux compiler profiles")
	fmt.Println("  --current-linux-profile          Show current Linux compiler profile")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("SETTINGS:"))
	fmt.Println("  color [true|false|toggle]  Enable/disable colors")
	fmt.Println("  updates [true|false|toggle] Enable/disable update checks")
	fmt.Println("  --colors                   Show current color setting")
	fmt.Println("  --updates                  Show current update setting")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("BUILD OPTIONS:"))
	fmt.Println("  --no-optimize, -N         Skip optimization")
	fmt.Println()
	fmt.Println(cli.Colors.Bold("UPDATE CHECKS:"))
	fmt.Println("  --noCheckZigUpdates       Disable background check for Zig updates")
	fmt.Println("  --noCheckUpdates          Disable background check for Skorpion updates")
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
	fmt.Println()
	fmt.Println(cli.Colors.Bold("TOOLCHAIN:"))
	fmt.Println("  setup-zig             Download and install Zig (C backend)")
	fmt.Println("  setup-zig --force     Re-download Zig even if already installed")
	fmt.Println()
}

func printVersion() {
	fmt.Println(cli.Colors.Bold("Skorpion Compiler v" + currentSkorpionVersion))
	fmt.Println(cli.Colors.Dim("Copyrights. (c) 2026 iamtowvee"))
	fmt.Println(cli.Colors.Dim("Distributed under MIT License"))
}
