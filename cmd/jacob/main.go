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
	"strings"
	"syscall"
	"time"

	"jacob/internal/bindings"
	"jacob/internal/core"
	"jacob/internal/customtabs"
	"jacob/internal/journal"
)

const localURL = "http://127.0.0.1:4510/"

func main() {
	dataDir := customtabs.DefaultDirectory()
	setupLog(dataDir)

	if runningJACoB() {
		openBrowser(localURL)
		return
	}

	journalDir, jc := journal.DetectDirectory()
	if journalDir == "" {
		log.Printf("Elite journal directory was not found automatically")
		for _, c := range jc {
			log.Printf("checked journal path: %s", c)
		}
	} else {
		log.Printf("Journal directory: %s", journalDir)
	}

	bindingsDir, bc := bindings.DetectDirectory()
	if bindingsDir == "" {
		log.Printf("Elite bindings directory was not found automatically")
		for _, c := range bc {
			log.Printf("checked bindings path: %s", c)
		}
	} else {
		log.Printf("Bindings directory: %s", bindingsDir)
	}

	lan := boolDefault(firstEnv("JACOB_LAN", "EDBRIDGE_LAN"), true)
	bind := "127.0.0.1:4510"
	if lan {
		bind = "0.0.0.0:4510"
	}

	cfg := core.Config{
		Bind:          envCompat("JACOB_BIND", "EDBRIDGE_BIND", bind),
		DataDir:       dataDir,
		JournalDir:    journalDir,
		BindingsDir:   bindingsDir,
		ReplayJournal: truthy(firstEnv("JACOB_REPLAY_JOURNAL", "EDBRIDGE_REPLAY_JOURNAL")),
		EnableInput:   boolDefault(firstEnv("JACOB_ENABLE_INPUT", "EDBRIDGE_ENABLE_INPUT"), true),
		AutoBind:      boolDefault(firstEnv("JACOB_AUTOBIND", "EDBRIDGE_AUTOBIND"), false),
		LANEnabled:    lan,
		PairToken:     firstEnv("JACOB_PAIR_TOKEN", "EDBRIDGE_PAIR_TOKEN"),
	}

	ctx, stop := signalContext()
	defer stop()
	s := core.New(cfg)

	if boolDefault(os.Getenv("JACOB_OPEN_BROWSER"), runtime.GOOS == "windows") {
		go func() { time.Sleep(450 * time.Millisecond); openBrowser(localURL) }()
	}

	if err := s.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server stopped: %v", err)
	}
}

func setupLog(dir string) {
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, "jacob.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	log.SetFlags(log.Ldate | log.Ltime)
}

func runningJACoB() bool {
	client := http.Client{Timeout: 250 * time.Millisecond}
	r, err := client.Get(localURL + "api/health")
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
