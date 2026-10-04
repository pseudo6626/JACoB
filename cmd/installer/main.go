//go:build windows

package main

import (
	"embed"
	"fmt"
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
		if opts.waitPID > 0 {
			waitForPID(opts.waitPID, 30*time.Second)
		}
		includeRecorder := installedRecorderEnabled()
		if opts.recorderSet {
			includeRecorder = opts.recorder
		}
		if err := install(includeRecorder); err != nil {
			msg("JACoB Update", "Update failed:\n\n"+err.Error(), mbOK|mbIconWarning)
			return
		}
		_ = exec.Command(filepath.Join(installDir(), "JACoB.exe")).Start()
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
		choice := msg("JACoB Setup", "Install the optional Action Recorder?\n\nThe recorder captures keyboard actions only when you explicitly start a recording and Elite Dangerous is the foreground window.\n\nYes  — install with Action Recorder\nNo   — install without recording support\nCancel — exit setup", mbYesNoCancel|mbIconQuestion)
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
	var o setupOptions
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

	appName := "payload/JACoB-no-recorder.exe"
	if includeRecorder {
		appName = "payload/JACoB.exe"
	}
	app, err := payload.ReadFile(appName)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "JACoB.exe"), app, 0o755); err != nil {
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

	docFiles := []string{
		"JACoB-Custom-Tab-Developer-Reference.md",
		"JACoB-Custom-Tab-Developer-Reference.html",
		"JACoB-User-Guide.md",
		"JACoB-Theming.md",
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
	v := strings.ToLower(strings.TrimSpace(regValue("RecorderEnabled")))
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

func waitForPID(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	needle := strconv.Itoa(pid)
	for time.Now().Before(deadline) {
		out, _ := hidden("tasklist", "/FI", "PID eq "+needle, "/NH").CombinedOutput()
		text := strings.ToLower(string(out))
		if !strings.Contains(text, needle) || strings.Contains(text, "no tasks are running") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
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
		{"Comments", map[bool]string{true: "Action Recorder installed", false: "Action Recorder not installed"}[recorder]},
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
	return hidden("reg", "add", uninstallKey, "/v", "RecorderEnabled", "/t", "REG_DWORD", "/d", recorderDWORD, "/f").Run()
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
