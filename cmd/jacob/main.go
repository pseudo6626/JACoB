package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"jacob/internal/bindings"
	"jacob/internal/buildinfo"
	"jacob/internal/core"
	"jacob/internal/customtabs"
	"jacob/internal/journal"
	"jacob/internal/pathconfig"
)

const (
	localURL       = "http://127.0.0.1:6626/"
	legacyLocalURL = "http://127.0.0.1:4510/"
)

func main() {
	dataDir := customtabs.DefaultDirectory()
	setupLog(dataDir)
	log.Printf("process/start: version=%s sdk=%d pid=%d os=%s arch=%s go=%s dataDir=%q", buildinfo.Version, buildinfo.SDKVersion, os.Getpid(), runtime.GOOS, runtime.GOARCH, runtime.Version(), dataDir)
	if exe, err := os.Executable(); err == nil {
		log.Printf("process/executable: %q", exe)
	} else {
		log.Printf("process/executable lookup failed: %v", err)
	}
	log.Printf("process/environment: lan=%q bind=%q input=%q autobind=%q replay=%q browser=%q journalOverride=%t bindingsOverride=%t controlSchemesOverride=%t pairKeyOverride=%t",
		firstEnv("JACOB_LAN", "EDBRIDGE_LAN"), firstEnv("JACOB_BIND", "EDBRIDGE_BIND"), firstEnv("JACOB_ENABLE_INPUT", "EDBRIDGE_ENABLE_INPUT"), firstEnv("JACOB_AUTOBIND", "EDBRIDGE_AUTOBIND"), firstEnv("JACOB_REPLAY_JOURNAL", "EDBRIDGE_REPLAY_JOURNAL"), os.Getenv("JACOB_OPEN_BROWSER"),
		firstEnv("JACOB_JOURNAL_DIR", "EDBRIDGE_JOURNAL_DIR") != "", firstEnv("JACOB_BINDINGS_DIR", "EDBRIDGE_BINDINGS_DIR") != "", firstEnv("JACOB_CONTROL_SCHEMES_DIR", "EDBRIDGE_CONTROL_SCHEMES_DIR") != "", firstEnv("JACOB_PAIR_TOKEN", "EDBRIDGE_PAIR_TOKEN") != "")
	defer log.Printf("process/exit: pid=%d", os.Getpid())

	if runningURL := runningJACoB(); runningURL != "" {
		log.Printf("process/existing-instance: healthy JACoB already answers at %s; opening browser and exiting this process", runningURL)
		openBrowser(runningURL)
		return
	}

	manualPaths, pathErr := pathconfig.Load(dataDir)
	if pathErr != nil {
		log.Printf("JACoB path settings could not be loaded; automatic detection will be used: %v", pathErr)
		manualPaths = pathconfig.Config{}
	}

	journalDir, jc := journal.DetectDirectoryWithOverride(manualPaths.JournalDir)
	if journalDir == "" {
		log.Printf("Elite journal directory was not found automatically")
		for _, c := range jc {
			log.Printf("checked journal path: %s", c)
		}
	} else {
		log.Printf("Journal directory: %s", journalDir)
	}

	bindingsDir, bc := bindings.DetectDirectoryWithOverride(manualPaths.BindingsDir)
	controlSchemeDirs := bindings.DetectControlSchemeDirectoriesWithOverrides(manualPaths.ControlSchemeDirs)
	if bindingsDir == "" {
		log.Printf("Elite bindings directory was not found automatically")
		for _, c := range bc {
			log.Printf("checked bindings path: %s", c)
		}
	} else {
		log.Printf("Bindings directory: %s", bindingsDir)
	}

	log.Printf("config/paths: journal=%q journalCandidates=%d manualJournal=%t envJournal=%t", journalDir, len(jc), strings.TrimSpace(manualPaths.JournalDir) != "", firstEnv("JACOB_JOURNAL_DIR", "EDBRIDGE_JOURNAL_DIR") != "")
	log.Printf("config/paths: bindings=%q bindingsCandidates=%d manualBindings=%t envBindings=%t controlSchemeDirs=%d manualControlSchemeDirs=%d envControlSchemes=%t", bindingsDir, len(bc), strings.TrimSpace(manualPaths.BindingsDir) != "", firstEnv("JACOB_BINDINGS_DIR", "EDBRIDGE_BINDINGS_DIR") != "", len(controlSchemeDirs), len(manualPaths.ControlSchemeDirs), firstEnv("JACOB_CONTROL_SCHEMES_DIR", "EDBRIDGE_CONTROL_SCHEMES_DIR") != "")

	lan := boolDefault(firstEnv("JACOB_LAN", "EDBRIDGE_LAN"), true)
	bind := "127.0.0.1:6626"
	if lan {
		bind = "0.0.0.0:6626"
	}

	cfg := core.Config{
		Bind:              envCompat("JACOB_BIND", "EDBRIDGE_BIND", bind),
		DataDir:           dataDir,
		JournalDir:        journalDir,
		BindingsDir:       bindingsDir,
		ReplayJournal:     truthy(firstEnv("JACOB_REPLAY_JOURNAL", "EDBRIDGE_REPLAY_JOURNAL")),
		EnableInput:       boolDefault(firstEnv("JACOB_ENABLE_INPUT", "EDBRIDGE_ENABLE_INPUT"), true),
		AutoBind:          boolDefault(firstEnv("JACOB_AUTOBIND", "EDBRIDGE_AUTOBIND"), false),
		LANEnabled:        lan,
		PairToken:         firstEnv("JACOB_PAIR_TOKEN", "EDBRIDGE_PAIR_TOKEN"),
		ManualPaths:       manualPaths,
		ControlSchemeDirs: controlSchemeDirs,
	}

	log.Printf("config/effective: bind=%q lan=%t input=%t autoBind=%t replayJournal=%t journalDir=%q bindingsDir=%q controlSchemeDirs=%d pairKeyOverride=%t", cfg.Bind, cfg.LANEnabled, cfg.EnableInput, cfg.AutoBind, cfg.ReplayJournal, cfg.JournalDir, cfg.BindingsDir, len(cfg.ControlSchemeDirs), firstEnv("JACOB_PAIR_TOKEN", "EDBRIDGE_PAIR_TOKEN") != "")

	ctx, stop := signalContext()
	defer stop()
	s := core.New(cfg)

	if boolDefault(os.Getenv("JACOB_OPEN_BROWSER"), defaultOpenBrowser()) {
		go func() { time.Sleep(450 * time.Millisecond); openBrowser(localURL) }()
	}

	err := s.Run(ctx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server/stop: unexpected error: %v", err)
		return
	}
	log.Printf("server/stop: clean shutdown")
}

func setupLog(dir string) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("logging: cannot create data directory %q: %v", dir, err)
		return
	}
	path := filepath.Join(dir, "jacob.log")
	if err := rotateLog(path, 8<<20, 3); err != nil {
		log.Printf("logging: rotation warning for %q: %v", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("logging: cannot open %q: %v", path, err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.Printf("logging: persistent log=%q rotation=8MiB backups=3", path)
}

func rotateLog(path string, maxBytes int64, backups int) error {
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Size() < maxBytes || backups < 1 {
		return nil
	}
	_ = os.Remove(path + "." + strconv.Itoa(backups))
	for i := backups - 1; i >= 1; i-- {
		oldPath := path + "." + strconv.Itoa(i)
		newPath := path + "." + strconv.Itoa(i+1)
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
	}
	return os.Rename(path, path+".1")
}

func runningJACoB() string {
	for _, url := range []string{localURL, legacyLocalURL} {
		if probeJACoB(url) {
			return url
		}
	}
	return ""
}

func probeJACoB(url string) bool {
	client := http.Client{Timeout: 250 * time.Millisecond}
	r, err := client.Get(url + "api/health")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return false
	}
	var v map[string]any
	if json.NewDecoder(r.Body).Decode(&v) != nil {
		return false
	}
	return strings.EqualFold(asString(v["product"]), "JACoB")
}

func asString(v any) string { s, _ := v.(string); return s }

func defaultOpenBrowser() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	if runtime.GOOS == "linux" {
		return strings.TrimSpace(os.Getenv("DISPLAY")) != "" || strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != ""
	}
	return false
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
func envCompat(primary, legacy, d string) string {
	if v := firstEnv(primary, legacy); v != "" {
		return v
	}
	return d
}
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
func boolDefault(v string, d bool) bool {
	if strings.TrimSpace(v) == "" {
		return d
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return d
}
