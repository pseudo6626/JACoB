//go:build windows

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"jacob/internal/buildinfo"
)

//go:embed payload/*
var payload embed.FS

const uninstallKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\JACoB`

const (
	mbOK           = 0x00000000
	mbYesNo        = 0x00000004
	mbYesNoCancel  = 0x00000003
	mbIconInfo     = 0x00000040
	mbIconQuestion = 0x00000020
	mbIconWarning  = 0x00000030
	idYes          = 6
	idNo           = 7
	idCancel       = 2
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	messageBoxW = user32.NewProc("MessageBoxW")
)

type setupOptions struct {
	uninstall   bool
	autoUpdate  bool
	waitPID     int
	dataDir     string
	sourceExe   string
	healthPort  int
	recorderSet bool
	recorder    bool
}

func main() {
	opts := parseOptions(os.Args[1:])
	if opts.uninstall {
		uninstall()
		return
	}

	if opts.autoUpdate {
		previousVersion := installedVersion()
		dataDir := strings.TrimSpace(opts.dataDir)
		if dataDir == "" {
			dataDir = defaultJACoBDataDir()
		}
		if opts.healthPort <= 0 || opts.healthPort > 65535 {
			opts.healthPort = 6626
		}

		updateLog(dataDir, "automatic update started: installed=%q target=%q waitPID=%d port=%d", previousVersion, buildinfo.Version, opts.waitPID, opts.healthPort)

		if strings.TrimSpace(previousVersion) == "" {
			updateLog(dataDir, "automatic update refused: no managed JACoB installation is registered")
			msg("JACoB Update", "Automatic update requires an installed JACoB copy. Portable builds should be updated manually from GitHub.", mbOK|mbIconWarning)
			return
		}

		installedExe := filepath.Join(installDir(), "JACoB.exe")
		if strings.TrimSpace(opts.sourceExe) != "" && !samePath(opts.sourceExe, installedExe) {
			updateLog(dataDir, "automatic update refused: running executable %q is not managed install %q", opts.sourceExe, installedExe)
			msg("JACoB Update", "Automatic update is only available to the installed JACoB copy. This appears to be a portable build; update it manually from GitHub.", mbOK|mbIconWarning)
			return
		}

		if opts.waitPID > 0 {
			updateLog(dataDir, "waiting for old core PID %d to exit", opts.waitPID)
			if !waitForPID(opts.waitPID, 8*time.Second) {
				updateLog(dataDir, "old core PID %d did not exit cleanly; forcing that process closed", opts.waitPID)
				_ = hidden("taskkill", "/PID", strconv.Itoa(opts.waitPID), "/T", "/F").Run()
				if !waitForPID(opts.waitPID, 5*time.Second) {
					updateLog(dataDir, "update aborted: old core PID %d is still running", opts.waitPID)
					msg("JACoB Update", "The previous JACoB process would not close. The update was stopped before replacing any files.", mbOK|mbIconWarning)
					return
				}
			}
			updateLog(dataDir, "old core PID %d is stopped", opts.waitPID)
		}

		if anyJACoBProcess() {
			updateLog(dataDir, "additional JACoB.exe process detected; closing stale instances")
			_ = hidden("taskkill", "/IM", "JACoB.exe", "/T", "/F").Run()
			if !waitForNoJACoB(5 * time.Second) {
				updateLog(dataDir, "update aborted: a JACoB.exe process is still running")
				msg("JACoB Update", "Another JACoB process is still running. The update was stopped before replacing any files.", mbOK|mbIconWarning)
				return
			}
		}

		if !waitForPortFree(opts.healthPort, 5*time.Second) {
			updateLog(dataDir, "update aborted: localhost port %d is still occupied", opts.healthPort)
			msg("JACoB Update", fmt.Sprintf("Local port %d is still in use after JACoB closed. The update was stopped before replacing any files.", opts.healthPort), mbOK|mbIconWarning)
			return
		}
		updateLog(dataDir, "handoff clean: no JACoB.exe remains and port %d is free", opts.healthPort)

		includeRecorder := installedRecorderEnabled()
		if opts.recorderSet {
			includeRecorder = opts.recorder
		}
		updateLog(dataDir, "preserving installed capture choice: full=%t", includeRecorder)

		dir := installDir()
		target := filepath.Join(dir, "JACoB.exe")
		backup := target + ".previous"
		_ = os.Remove(backup)

		if err := install(includeRecorder); err != nil {
			updateLog(dataDir, "installation failed: %v", err)
			var recoveryErr error
			if _, statErr := os.Stat(backup); statErr == nil {
				_, recoveryErr = rollbackExecutable(dir, previousVersion, dataDir)
			} else {
				_, recoveryErr = startJACoB(target, dataDir)
			}
			msg("JACoB Update", rollbackMessage("Update failed while replacing the installation: "+err.Error(), recoveryErr), mbOK|mbIconWarning)
			return
		}

		newPID, err := startJACoB(target, dataDir)
		if err != nil {
			updateLog(dataDir, "new executable could not start: %v", err)
			_, rollbackErr := rollbackExecutable(dir, previousVersion, dataDir)
			msg("JACoB Update", rollbackMessage("Updated JACoB could not be started: "+err.Error(), rollbackErr), mbOK|mbIconWarning)
			return
		}
		updateLog(dataDir, "new core launched as PID %d; verifying health", newPID)

		if err := verifyRunningVersion(buildinfo.Version, newPID, opts.healthPort, 15*time.Second); err != nil {
			updateLog(dataDir, "new core verification failed: %v", err)
			_ = hidden("taskkill", "/PID", strconv.Itoa(newPID), "/T", "/F").Run()
			_ = waitForPID(newPID, 5*time.Second)
			_ = waitForPortFree(opts.healthPort, 5*time.Second)

			rollbackPID, rollbackErr := rollbackExecutable(dir, previousVersion, dataDir)
			if rollbackErr == nil {
				updateLog(dataDir, "rollback launched previous version as PID %d", rollbackPID)
				if verifyErr := verifyRunningVersion(previousVersion, 0, opts.healthPort, 15*time.Second); verifyErr != nil {
					rollbackErr = fmt.Errorf("previous executable was restored but did not pass health verification: %w", verifyErr)
					updateLog(dataDir, "rollback verification failed: %v", verifyErr)
				} else {
					updateLog(dataDir, "rollback verified: version %s is online", previousVersion)
				}
			}
			msg("JACoB Update", rollbackMessage("Updated JACoB failed its startup/version check: "+err.Error(), rollbackErr), mbOK|mbIconWarning)
			return
		}

		updateLog(dataDir, "update complete: %s is online as PID %d", buildinfo.Version, newPID)
		return
	}

	installed := installedVersion()
	includeRecorder := false
	if installed != "" {
		cmp := buildinfo.CompareVersion(buildinfo.Version, installed)
		var prompt string
		switch {
		case cmp > 0:
			prompt = fmt.Sprintf("JACoB %s is already installed.\n\nThis setup contains %s. Update the existing installation?\n\nSaved tabs, navigation, language settings and appearance files will be kept.", installed, buildinfo.Version)
		case cmp == 0:
			prompt = fmt.Sprintf("JACoB %s is already installed.\n\nReinstall this build?\n\nSaved tabs, navigation, language settings and appearance files will be kept.", installed)
		default:
			prompt = fmt.Sprintf("JACoB %s is already installed.\n\nThis setup contains the older build %s. Replace the installed version?", installed, buildinfo.Version)
		}
		if msg("JACoB Setup", prompt, mbYesNo|mbIconQuestion) != idYes {
			return
		}
		includeRecorder = installedRecorderEnabled()
		_ = exec.Command("taskkill", "/IM", "JACoB.exe", "/F").Run()
		time.Sleep(700 * time.Millisecond)
	} else {
		choice := msg("JACoB Setup", "Install the full capture-capable JACoB build?\n\nYes  — Full build: Action Recorder, Game View, Vision and OCR\nNo   — No Capture build: no keyboard recorder, no screen/video capture, no Vision or OCR\nCancel — exit setup", mbYesNoCancel|mbIconQuestion)
		if choice == idCancel {
			return
		}
		includeRecorder = choice == idYes
	}

	if err := install(includeRecorder); err != nil {
		msg("JACoB Setup", "Installation failed:\n\n"+err.Error(), mbOK|mbIconWarning)
		return
	}

	verb := "installed"
	if installed != "" {
		verb = "updated"
	}
	launch := msg("JACoB Setup", fmt.Sprintf("JACoB %s has been %s.\n\nA Start Menu shortcut is available.\n\nLaunch JACoB now?", buildinfo.Display, verb), mbYesNo|mbIconInfo)
	if launch == idYes {
		_ = exec.Command(filepath.Join(installDir(), "JACoB.exe")).Start()
	}
}

func parseOptions(args []string) setupOptions {
	o := setupOptions{healthPort: 6626}
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		switch {
		case strings.EqualFold(a, "--uninstall"):
			o.uninstall = true
		case strings.EqualFold(a, "--update"):
			o.autoUpdate = true
		case strings.EqualFold(a, "--wait-pid") && i+1 < len(args):
			i++
			o.waitPID, _ = strconv.Atoi(args[i])
		case strings.EqualFold(a, "--data-dir") && i+1 < len(args):
			i++
			o.dataDir = strings.TrimSpace(args[i])
		case strings.EqualFold(a, "--source-exe") && i+1 < len(args):
			i++
			o.sourceExe = strings.TrimSpace(args[i])
		case strings.EqualFold(a, "--health-port") && i+1 < len(args):
			i++
			o.healthPort, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(strings.ToLower(a), "--recorder="):
			o.recorderSet = true
			v := strings.TrimSpace(strings.SplitN(a, "=", 2)[1])
			o.recorder = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
		}
	}
	return o
}

func install(includeRecorder bool) error {
	dir := installDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	examples := filepath.Join(dir, "Examples")
	if err := os.MkdirAll(examples, 0o755); err != nil {
		return err
	}
	docs := filepath.Join(dir, "Documentation")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		return err
	}

	appName := "payload/JACoB-no-capture.exe"
	if includeRecorder {
		appName = "payload/JACoB.exe"
	}
	app, err := payload.ReadFile(appName)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "JACoB.exe")
	backup := target + ".previous"
	if old, readErr := os.ReadFile(target); readErr == nil {
		if err := os.WriteFile(backup, old, 0o755); err != nil {
			return fmt.Errorf("backup current JACoB executable: %w", err)
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read current JACoB executable: %w", readErr)
	}
	if err := os.WriteFile(target, app, 0o755); err != nil {
		return err
	}

	icon, _ := payload.ReadFile("payload/JACoB.ico")
	if len(icon) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "JACoB.ico"), icon, 0o644)
	}

	theme, _ := payload.ReadFile("payload/UI-Theme-Example.html")
	if len(theme) > 0 {
		_ = os.WriteFile(filepath.Join(examples, "UI Theme Example.html"), theme, 0o644)
	}
	visionDiagnosticsPath := filepath.Join(examples, "Vision Diagnostics.html")
	if includeRecorder {
		visionDiagnostics, _ := payload.ReadFile("payload/Vision-Diagnostics.html")
		if len(visionDiagnostics) > 0 {
			_ = os.WriteFile(visionDiagnosticsPath, visionDiagnostics, 0o644)
		}
	} else {
		_ = os.Remove(visionDiagnosticsPath)
	}
	miningCompanion, _ := payload.ReadFile("payload/Ring-Mining-Companion.html")
	if len(miningCompanion) > 0 {
		_ = os.WriteFile(filepath.Join(examples, "Ring Mining Companion.html"), miningCompanion, 0o644)
	}

	docFiles := []string{
		"JACoB-Custom-Tab-Developer-Reference.md",
		"JACoB-Custom-Tab-Developer-Reference.html",
		"JACoB-User-Guide.md",
		"JACoB-Theming.md",
		"JACoB-Security.md",
		"docs.css",
	}
	for _, name := range docFiles {
		b, readErr := payload.ReadFile("payload/Documentation/" + name)
		if readErr == nil {
			_ = os.WriteFile(filepath.Join(docs, name), b, 0o644)
		}
	}

	recorderPath := filepath.Join(examples, "Action Recorder.html")
	if includeRecorder {
		rec, _ := payload.ReadFile("payload/Action-Recorder.html")
		if len(rec) > 0 {
			_ = os.WriteFile(recorderPath, rec, 0o644)
		}
	} else {
		_ = os.Remove(recorderPath)
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	selfBytes, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "Uninstall.exe"), selfBytes, 0o755); err != nil {
		return err
	}

	if err := shortcuts(dir); err != nil {
		return err
	}
	if err := registerUninstall(dir, includeRecorder); err != nil {
		return err
	}
	return nil
}

func uninstall() {
	if msg("Uninstall JACoB", "Remove JACoB from this Windows account?\n\nSaved tabs, navigation, language settings and appearance files in AppData will be kept.", mbYesNo|mbIconQuestion) != idYes {
		return
	}

	dir := installDir()
	_ = exec.Command("taskkill", "/IM", "JACoB.exe", "/F").Run()
	removeShortcuts()
	_ = hidden("reg", "delete", uninstallKey, "/f").Run()

	cmd := fmt.Sprintf(`ping 127.0.0.1 -n 3 >nul & rmdir /s /q "%s"`, dir)
	c := exec.Command("cmd", "/c", cmd)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = c.Start()
	msg("Uninstall JACoB", "JACoB has been removed.\n\nUser data remains in AppData for a later installation.", mbOK|mbIconInfo)
}

func installedVersion() string {
	return regValue("DisplayVersion")
}

func installedRecorderEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(regValue("CaptureEnabled")))
	if v != "0x1" && v != "1" && v != "0x0" && v != "0" {
		v = strings.ToLower(strings.TrimSpace(regValue("RecorderEnabled")))
	}
	if v == "0x1" || v == "1" {
		return true
	}
	if v == "0x0" || v == "0" {
		return false
	}
	return strings.Contains(strings.ToLower(regValue("Comments")), "recorder installed")
}

func regValue(name string) string {
	out, err := hidden("reg", "query", uninstallKey, "/v", name).CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if !strings.Contains(strings.ToLower(line), strings.ToLower(name)) {
			continue
		}
		upper := strings.ToUpper(line)
		for _, typ := range []string{"REG_SZ", "REG_DWORD", "REG_EXPAND_SZ"} {
			if i := strings.Index(upper, typ); i >= 0 {
				return strings.TrimSpace(line[i+len(typ):])
			}
		}
	}
	return ""
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	needle := `"` + strconv.Itoa(pid) + `"`
	out, err := hidden("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), needle)
}

func waitForPID(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !processRunning(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func anyJACoBProcess() bool {
	out, err := hidden("tasklist", "/FI", "IMAGENAME eq JACoB.exe", "/FO", "CSV", "/NH").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), `"jacob.exe"`)
}

func waitForNoJACoB(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !anyJACoBProcess() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func waitForPortFree(port int, timeout time.Duration) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 180*time.Millisecond)
		if err != nil {
			return true
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func defaultJACoBDataDir() string {
	if v := strings.TrimSpace(os.Getenv("JACOB_DATA_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("EDBRIDGE_DATA_DIR")); v != "" {
		return v
	}
	if base := strings.TrimSpace(os.Getenv("APPDATA")); base != "" {
		return filepath.Join(base, "JACoB")
	}
	return filepath.Join(os.TempDir(), "JACoB")
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(filepath.Clean(a))
	bb, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return strings.EqualFold(aa, bb)
}

func updateLog(dataDir, format string, args ...any) {
	if strings.TrimSpace(dataDir) == "" {
		return
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "update.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	prefix := time.Now().Format("2006-01-02 15:04:05")
	values := append([]any{prefix}, args...)
	_, _ = fmt.Fprintf(f, "%s "+format+"\r\n", values...)
}

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Programs", "JACoB")
}

func startMenuDir() string {
	base := os.Getenv("APPDATA")
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "JACoB")
}

func shortcuts(dir string) error {
	sm := startMenuDir()
	if err := os.MkdirAll(sm, 0o755); err != nil {
		return err
	}
	if err := makeShortcut(filepath.Join(sm, "JACoB.lnk"), filepath.Join(dir, "JACoB.exe"), dir); err != nil {
		return err
	}
	return makeShortcut(filepath.Join(sm, "Uninstall JACoB.lnk"), filepath.Join(dir, "Uninstall.exe"), dir)
}

func removeShortcuts() { _ = os.RemoveAll(startMenuDir()) }

func makeShortcut(path, target, workDir string) error {
	icon := filepath.Join(workDir, "JACoB.ico")
	script := fmt.Sprintf(`$w=New-Object -ComObject WScript.Shell;$s=$w.CreateShortcut('%s');$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.IconLocation='%s,0';$s.Save()`, ps(path), ps(target), ps(workDir), ps(icon))
	return hidden("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).Run()
}

func registerUninstall(dir string, recorder bool) error {
	fields := [][2]string{
		{"DisplayName", "JACoB - Journal Aligned Control Bridge"},
		{"DisplayVersion", buildinfo.Version},
		{"Publisher", "JACoB Project"},
		{"InstallLocation", dir},
		{"DisplayIcon", filepath.Join(dir, "JACoB.ico")},
		{"UninstallString", `"` + filepath.Join(dir, "Uninstall.exe") + `" --uninstall`},
		{"Comments", map[bool]string{true: "Full capture build installed", false: "No Capture build installed"}[recorder]},
	}
	for _, f := range fields {
		if err := hidden("reg", "add", uninstallKey, "/v", f[0], "/t", "REG_SZ", "/d", f[1], "/f").Run(); err != nil {
			return err
		}
	}
	recorderDWORD := "0"
	if recorder {
		recorderDWORD = "1"
	}
	if err := hidden("reg", "add", uninstallKey, "/v", "RecorderEnabled", "/t", "REG_DWORD", "/d", recorderDWORD, "/f").Run(); err != nil {
		return err
	}
	return hidden("reg", "add", uninstallKey, "/v", "CaptureEnabled", "/t", "REG_DWORD", "/d", recorderDWORD, "/f").Run()
}

func hidden(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c
}

func ps(s string) string { return strings.ReplaceAll(s, "'", "''") }

func msg(title, text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(title)
	x, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := messageBoxW.Call(0, uintptr(unsafe.Pointer(x)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

func verifyRunningVersion(expected string, expectedPID, port int, timeout time.Duration) error {
	client := http.Client{Timeout: 900 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	var last string
	url := fmt.Sprintf("http://127.0.0.1:%d/api/health", port)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			var health struct {
				Product string `json:"product"`
				Version string `json:"version"`
				PID     int    `json:"pid"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&health)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && decodeErr == nil && strings.EqualFold(health.Product, "JACoB") {
				if health.Version == expected && (expectedPID <= 0 || health.PID == expectedPID) {
					return nil
				}
				if expectedPID > 0 {
					last = fmt.Sprintf("core reported version %q PID %d; expected version %q PID %d", health.Version, health.PID, expected, expectedPID)
				} else {
					last = fmt.Sprintf("core reported version %q, expected %q", health.Version, expected)
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if last == "" {
		last = "local health endpoint did not become ready"
	}
	return fmt.Errorf("%s", last)
}

func envWithOverride(key, value string) []string {
	prefix := strings.ToUpper(key) + "="
	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(item), prefix) {
			env = append(env, item)
		}
	}
	if strings.TrimSpace(value) != "" {
		env = append(env, key+"="+value)
	}
	return env
}

func startJACoB(target, dataDir string) (int, error) {
	cmd := exec.Command(target)
	cmd.Dir = filepath.Dir(target)
	cmd.Env = envWithOverride("JACOB_DATA_DIR", dataDir)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func rollbackExecutable(dir, previousVersion, dataDir string) (int, error) {
	target := filepath.Join(dir, "JACoB.exe")
	backup := target + ".previous"
	if _, err := os.Stat(backup); err != nil {
		return 0, fmt.Errorf("previous executable is unavailable: %w", err)
	}
	_ = os.Remove(target + ".failed")
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, target+".failed"); err != nil {
			return 0, fmt.Errorf("preserve failed executable: %w", err)
		}
	}
	if err := os.Rename(backup, target); err != nil {
		return 0, fmt.Errorf("restore previous executable: %w", err)
	}
	if strings.TrimSpace(previousVersion) != "" {
		_ = hidden("reg", "add", uninstallKey, "/v", "DisplayVersion", "/t", "REG_SZ", "/d", previousVersion, "/f").Run()
	}
	pid, err := startJACoB(target, dataDir)
	if err != nil {
		return 0, fmt.Errorf("restart previous JACoB: %w", err)
	}
	return pid, nil
}

func rollbackMessage(reason string, rollbackErr error) string {
	if rollbackErr == nil {
		return reason + "\n\nThe previous JACoB executable was restored and restarted."
	}
	return reason + "\n\nAutomatic rollback also failed: " + rollbackErr.Error()
}
