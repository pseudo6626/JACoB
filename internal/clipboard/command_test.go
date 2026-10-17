package clipboard

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

func TestClipboardCommandChild(t *testing.T) {
	if os.Getenv("JACOB_CLIPBOARD_TEST_CHILD") != "1" {
		return
	}
	if os.Getenv("JACOB_CLIPBOARD_TEST_HANG") == "1" {
		time.Sleep(10 * time.Second)
		os.Exit(1)
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil || string(b) != os.Getenv("JACOB_CLIPBOARD_TEST_EXPECT") {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestCommandTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JACOB_CLIPBOARD_TEST_CHILD", "1")
	t.Setenv("JACOB_CLIPBOARD_TEST_HANG", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := runCommand(ctx, commandSpec{name: "test", path: exe, args: []string{"-test.run=^TestClipboardCommandChild$"}}, "target"); err == nil {
		t.Fatal("hung child succeeded")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatal("expected deadline")
	}
}
