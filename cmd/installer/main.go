//go:build windows

package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

//go:embed payload/*
var payload embed.FS

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

func main() {
	if len(os.Args) > 1 && strings.EqualFold(os.Args[1], "--uninstall") {
		uninstall()
		return
	}

	choice := msg("JACoB Setup", "Install the optional Action Recorder?\n\nThe recorder can capture keyboard actions only when you explicitly start a recording and Elite Dangerous is the foreground window.\n\nYes  — install JACoB with Action Recorder\nNo   — install JACoB without recording support\nCancel — exit setup", mbYesNoCancel|mbIconQuestion)
	if choice == idCancel {
		return
	}
	includeRecorder := choice == idYes

	if err := install(includeRecorder); err != nil {
		msg("JACoB Setup", "Installation failed:\n\n"+err.Error(), mbOK|mbIconWarning)
		return
	}

	launch := msg("JACoB Setup", "JACoB Alpha 0.2.2 has been installed.\n\nA Start Menu shortcut was created.\n\nLaunch JACoB now?", mbYesNo|mbIconInfo)
	if launch == idYes {
		exe := filepath.Join(installDir(), "JACoB.exe")
		_ = exec.Command(exe).Start()
	}
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
	if msg("Uninstall JACoB", "Remove JACoB from this Windows account?\n\nYour custom tabs and UI theme in AppData will be kept.", mbYesNo|mbIconQuestion) != idYes {
		return
	}

	dir := installDir()
	_ = exec.Command("taskkill", "/IM", "JACoB.exe", "/F").Run()
	removeShortcuts()
	_ = hidden("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\JACoB`, "/f").Run()

	cmd := fmt.Sprintf(`ping 127.0.0.1 -n 3 >nul & rmdir /s /q "%s"`, dir)
	c := exec.Command("cmd", "/c", cmd)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = c.Start()
	msg("Uninstall JACoB", "JACoB has been removed.\n\nUser data was left in AppData so custom tabs and themes can be restored by reinstalling.", mbOK|mbIconInfo)
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
	key := `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\JACoB`
	fields := [][2]string{
		{"DisplayName", "JACoB - Journal Aligned Control Bridge"},
		{"DisplayVersion", "0.2.2-alpha"},
		{"Publisher", "JACoB Project"},
		{"InstallLocation", dir},
		{"DisplayIcon", filepath.Join(dir, "JACoB.ico")},
		{"UninstallString", `"` + filepath.Join(dir, "Uninstall.exe") + `" --uninstall`},
		{"Comments", map[bool]string{true: "Action Recorder installed", false: "Action Recorder not installed"}[recorder]},
	}
	for _, f := range fields {
		if err := hidden("reg", "add", key, "/v", f[0], "/t", "REG_SZ", "/d", f[1], "/f").Run(); err != nil {
			return err
		}
	}
	return nil
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
