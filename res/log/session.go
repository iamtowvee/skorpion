// Package log реализует сессионный лог компилятора Skorpion.
//
// Один запуск = один файл logs/YYYY-MM-DDTHH-MM-SS.log.
// Создаётся в Start(), пишется инкрементально, финализируется в
// Finalize() (из defer в main или при panic).
//
// Секции: HEADER, CONFIG, SOURCE, TOKENS, AST, IR, C CODE,
// C COMPILER OUTPUT, ERRORS, CRASH.
package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxSourceSize      = 2 * 1024 * 1024
	maxTotalSourceSize = 8 * 1024 * 1024
	maxCCodeSize       = 4 * 1024 * 1024
	maxRingSize        = 5000
)

var (
	mu       sync.Mutex
	file     *os.File
	path     string
	started  time.Time
	ring     []string
	ringPos  int
	debugOn  bool
	closed   bool
	started_ bool

	useLocal    bool
	localLogDir string
	exitCode    int

	meta = struct {
		Version string
		Args    []string
		CWD     string
	}{}

	wroteHeader, wroteConfig, wroteSources, wroteErrors               bool
	wroteTokens, wroteAST, wroteIR, wroteCCode, wroteCLog, wroteCrash bool

	configText                                       string
	sources                                          = map[string]string{}
	astText, tokensText, irText, cCodeText, cLogText string
)

var readOnly bool

func StartReadOnly(projectName, projectVersion, projectDir string) {
	mu.Lock()
	defer mu.Unlock()
	if started_ {
		return
	}
	started_ = true
	started = time.Now()
	ring = make([]string, maxRingSize)
	readOnly = true
}

// Start инициализирует сессию лога.
//
// Политика путей:
//  1. SK_LOG_DIR задан         → пишем туда, useLocal = true.
//  2. SK_LOCAL_LOG=1           → пишем в <projectDir>/logs/, useLocal = true.
//  3. иначе                    → <CacheRoot>/logs/<name>__<version>/
//
// Удаление при успехе — только для useLocal == true (см. Finalize).
func Start(projectName, projectVersion, projectDir string) {
	mu.Lock()
	defer mu.Unlock()
	if started_ {
		return
	}
	started_ = true
	started = time.Now()
	ring = make([]string, maxRingSize)

	if dir := os.Getenv("SK_LOG_DIR"); dir != "" {
		useLocal = true
		localLogDir = dir
	} else if projectDir != "" && os.Getenv("SK_LOCAL_LOG") == "1" {
		useLocal = true
		localLogDir = LocalLogDir(projectDir)
	}

	dir := ProjectLogDir(projectName, projectVersion)
	if useLocal {
		dir = localLogDir
	}
	_ = os.MkdirAll(dir, 0755)

	name := started.Format("2006-01-02T15-04-05") + ".log"
	path = filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		path = ""
		return
	}
	file = f

	meta.Args = os.Args
	if wd, err := os.Getwd(); err == nil {
		meta.CWD = wd
	}

	rotateLocked(5)
}

// SetExitCode вызывается из main перед Finalize.
func SetExitCode(code int) {
	mu.Lock()
	defer mu.Unlock()
	exitCode = code
}

// Path возвращает путь к текущему логу ("" если не создан).
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// SetVersion — установить версию компилятора для header.
func SetVersion(v string) {
	mu.Lock()
	defer mu.Unlock()
	meta.Version = v
}

// EnableDebug — включает запись debug-сообщений в файл.
func EnableDebug() {
	mu.Lock()
	defer mu.Unlock()
	debugOn = true
}

func IsDebugEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return debugOn
}

// Writer — io.Writer для Tee внешних процессов (C-компилятор).
// Возвращает io.Discard, если лог недоступен.
func Writer() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	if file == nil || closed {
		return io.Discard
	}
	return file
}

// Debug — пишет debug-сообщение в ring (всегда) и в файл (если enabled).
func Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	mu.Lock()
	defer mu.Unlock()
	ring[ringPos%maxRingSize] = msg
	ringPos++
	if !debugOn || file == nil || closed {
		return
	}
	fmt.Fprintln(file, " DEBUG  "+msg)
}

// Tail — последние n debug-сообщений (для crash-репорта).
func Tail(n int) []string {
	mu.Lock()
	defer mu.Unlock()
	return tailLocked(n)
}

// SetConfig сохраняет manifest.spc.
func SetConfig(text string) {
	mu.Lock()
	defer mu.Unlock()
	configText = text
}

// AddSource сохраняет исходник (с лимитами).
func AddSource(name, content string) {
	mu.Lock()
	defer mu.Unlock()
	if len(content) > maxSourceSize {
		content = content[:maxSourceSize] +
			"\n\n...[truncated, original " + itoa(len(content)) + " bytes]\n"
	}
	total := 0
	for _, c := range sources {
		total += len(c)
	}
	if total+len(content) > maxTotalSourceSize {
		sources[name] = "...[skipped: total source limit reached]\n"
		return
	}
	sources[name] = content
}

// SetTokens / SetAST / SetIR / SetCCode — сохраняют соответствующие дампы.
func SetTokens(t string) { mu.Lock(); tokensText = t; mu.Unlock() }
func SetAST(t string)    { mu.Lock(); astText = t; mu.Unlock() }
func SetIR(t string)     { mu.Lock(); irText = t; mu.Unlock() }

func SetCCode(t string) {
	mu.Lock()
	defer mu.Unlock()
	if len(t) > maxCCodeSize {
		t = t[:maxCCodeSize] + "\n...[truncated]\n"
	}
	cCodeText = t
}

func SetCLog(t string) {
	mu.Lock()
	defer mu.Unlock()
	if len(t) > maxCCodeSize {
		t = t[:maxCCodeSize] + "\n...[truncated]\n"
	}
	cLogText = t
}

// WriteCrash пишет crash-секцию. Идемпотентно.
func WriteCrash(panicVal interface{}, stack []byte) {
	mu.Lock()
	defer mu.Unlock()
	if wroteCrash || file == nil || closed {
		return
	}
	wroteCrash = true

	writeLocked("\n=== CRASH ===\n")
	writeLocked("Time:    %s\n", time.Now().Format(time.RFC3339))
	writeLocked("Panic:   %v\n", panicVal)
	writeLocked("Version: %s\n", meta.Version)
	writeLocked("OS:      %s/%s\n", runtime.GOOS, runtime.GOARCH)
	writeLocked("Go:      %s\n", runtime.Version())
	writeLocked("Args:    %q\n", meta.Args)
	writeLocked("CWD:     %s\n", meta.CWD)
	writeLocked("\n=== STACK ===\n%s\n", stack)

	if ringPos > 0 {
		n := ringPos
		if n > maxRingSize {
			n = maxRingSize
		}
		if n > 200 {
			n = 200
		}
		writeLocked("\n=== LAST DEBUG LINES (ring, %d of %d) ===\n", n, minInt(ringPos, maxRingSize))
		for _, line := range tailLocked(n) {
			writeLocked("%s\n", line)
		}
	}
}

// Finalize дописывает все секции и закрывает файл.
//
// Удаление:
//   - Локальный лог (<projectDir>/logs/ или SK_LOG_DIR) при exitCode == 0 —
//     удаляется. При exitCode != 0 — сохраняется.
//   - Общий лог (<base>/skorpion/logs/<name>__<ver>/) сохраняется всегда.
func Finalize(writeErrors func(io.Writer)) {
	mu.Lock()
	defer mu.Unlock()
	if file == nil || closed {
		return
	}

	writeHeaderLocked()
	writeConfigLocked()
	writeSourcesLocked()

	if writeErrors != nil {
		writeLocked("=== ERRORS ===\n")
		writeErrors(file)
		writeLocked("\n")
	}

	if debugOn {
		if tokensText != "" && !wroteTokens {
			wroteTokens = true
			writeLocked("=== TOKENS ===\n%s\n\n", tokensText)
		}
		if astText != "" && !wroteAST {
			wroteAST = true
			writeLocked("=== AST ===\n%s\n\n", astText)
		}
	}
	if irText != "" && !wroteIR {
		wroteIR = true
		writeLocked("=== IR ===\n%s\n\n", irText)
	}
	if cCodeText != "" && !wroteCCode {
		wroteCCode = true
		writeLocked("=== C CODE ===\n%s\n\n", cCodeText)
	}
	if cLogText != "" && !wroteCLog {
		wroteCLog = true
		writeLocked("=== C COMPILER OUTPUT ===\n%s\n\n", cLogText)
	}

	_ = file.Sync()
	_ = file.Close()
	closed = true

	// Локальный лог при успехе — удаляем.
	// Общий (<CacheRoot>/logs/...) — не трогаем никогда.
	if useLocal && exitCode == 0 && path != "" {
		_ = os.Remove(path)
		if localLogDir != "" {
			_ = os.Remove(localLogDir) // удалит, только если пуста
		}
	}
}

// ============================================================================
// Внутренние
// ============================================================================

func writeLocked(format string, args ...interface{}) {
	if file == nil || closed {
		return
	}
	fmt.Fprintf(file, format, args...)
}

func writeHeaderLocked() {
	if wroteHeader {
		return
	}
	wroteHeader = true
	writeLocked("=== HEADER ===\n")
	writeLocked("Version: %s\n", meta.Version)
	writeLocked("OS:      %s/%s\n", runtime.GOOS, runtime.GOARCH)
	writeLocked("Go:      %s\n", runtime.Version())
	writeLocked("Time:    %s\n", started.Format(time.RFC3339))
	writeLocked("Args:    %q\n", meta.Args)
	writeLocked("CWD:     %s\n", meta.CWD)
	writeLocked("\n")
}

func writeConfigLocked() {
	if wroteConfig || configText == "" {
		return
	}
	wroteConfig = true
	writeLocked("=== CONFIG (manifest.spc) ===\n%s\n\n", configText)
}

func writeSourcesLocked() {
	if wroteSources || len(sources) == 0 {
		return
	}
	wroteSources = true
	names := make([]string, 0, len(sources))
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		writeLocked("=== SOURCE: %s (%d bytes) ===\n%s\n\n", n, len(sources[n]), sources[n])
	}
}

func tailLocked(n int) []string {
	if n > ringPos {
		n = ringPos
	}
	if n > maxRingSize {
		n = maxRingSize
	}
	if n <= 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := ringPos - n; i < ringPos; i++ {
		if i < 0 {
			continue
		}
		idx := ((i % maxRingSize) + maxRingSize) % maxRingSize
		if ring[idx] != "" {
			out = append(out, ring[idx])
		}
	}
	return out
}

// rotateLocked оставляет не больше keep последних .log в текущем каталоге.
func rotateLocked(keep int) {
	if keep <= 0 {
		keep = 5
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var logs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		logs = append(logs, filepath.Join(dir, e.Name()))
	}
	sort.Strings(logs)
	if len(logs) <= keep {
		return
	}
	for _, f := range logs[:len(logs)-keep] {
		_ = os.Remove(f)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// Баннер
// ============================================================================

// PrintCrashBanner печатает баннер с просьбой прислать лог.
func PrintCrashBanner(w io.Writer, panicVal interface{}, version string) {
	p := Path()
	fmt.Fprintf(w, "\n[%s > IMPORTANT]: Please send this message to https://github.com/iamtowvee/skorpion/issues/new\n",
		time.Now().Format("2006-01-02T15:04"))
	fmt.Fprintf(w, "   >>> COMPILER DATA\n")
	fmt.Fprintf(w, "    |\n")
	fmt.Fprintf(w, "    | VERSION: %s\n", version)
	fmt.Fprintf(w, "    | OS:      %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "    | GO:      %s\n", runtime.Version())
	fmt.Fprintf(w, "    | PANIC:   %v\n", panicVal)
	fmt.Fprintf(w, "    |\n")
	fmt.Fprintf(w, "   >>> HOW YOU CAN HELP US\n")
	if p != "" {
		fmt.Fprintf(w, "    | Attach file %q to issue\n", p)
	} else {
		fmt.Fprintf(w, "    | (log file could not be created — check write permissions in CWD)\n")
	}
}

// PrintLogPath печатает путь к логу при нормальном завершении с ошибками.
func PrintLogPath(w io.Writer) {
	p := Path()
	if p == "" {
		return
	}
	fmt.Fprintf(w, "\nFull log saved to: %s\n", p)
}
