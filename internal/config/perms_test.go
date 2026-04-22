// internal/config/perms_test.go
package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/config"
)

func TestCheckPerms_Tight_NoWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX perms only")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if got := config.CheckPerms(path, &buf); got {
		t.Error("0600 should not warn")
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestCheckPerms_GroupWrite_Warns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX perms only")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(""), 0o660); err != nil {
		t.Fatal(err)
	}
	// Chmod explicitly to survive the umask.
	if err := os.Chmod(path, 0o660); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if got := config.CheckPerms(path, &buf); !got {
		t.Error("0660 should warn")
	}
	if !strings.Contains(buf.String(), "writable by group/other") {
		t.Errorf("expected warning text, got %q", buf.String())
	}
}

func TestCheckPerms_OtherWrite_Warns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX perms only")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(""), 0o606); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o606); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if got := config.CheckPerms(path, &buf); !got {
		t.Error("0606 should warn")
	}
}

func TestCheckPerms_MissingFile_NoWarning(t *testing.T) {
	var buf bytes.Buffer
	if got := config.CheckPerms("/nonexistent/file", &buf); got {
		t.Error("missing file should not warn (caller handles)")
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected output: %q", buf.String())
	}
}
