package mcpbridge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The release downloads put the CLI and the desktop in one directory; the CLI must find the
// desktop under the name its own platform package uses.
func TestDesktopBesideFindsThePlatformPackageName(t *testing.T) {
	suffix, desktop := "", "testdesk"
	if runtime.GOOS == "windows" {
		suffix, desktop = ".exe", "TestDesk.exe"
	}
	dir := t.TempDir()
	cli := filepath.Join(dir, "testdesk-cli"+suffix)
	touch(t, cli)
	if got := desktopBeside(cli); got != "" {
		t.Fatalf("a lone CLI must not find a desktop, got %q", got)
	}
	touch(t, filepath.Join(dir, desktop))
	if got := desktopBeside(cli); !strings.EqualFold(filepath.Base(got), desktop) {
		t.Fatalf("expected %s beside the CLI, got %q", desktop, got)
	}
}

func TestDesktopBesideAcceptsEitherSpellingOnCaseSensitiveSystems(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows resolves names case-insensitively")
	}
	dir := t.TempDir()
	cli := filepath.Join(dir, "testdesk-cli")
	touch(t, cli)
	touch(t, filepath.Join(dir, "TestDesk"))
	probe := filepath.Join(dir, "testdesk")
	if _, err := os.Stat(probe); err == nil {
		t.Skip("case-insensitive filesystem")
	}
	if got := desktopBeside(cli); !strings.EqualFold(filepath.Base(got), "TestDesk") {
		t.Fatalf("expected TestDesk beside the CLI, got %q", got)
	}
}

func TestDesktopBesideNeverReturnsTheCLIItself(t *testing.T) {
	dir := t.TempDir()
	name := "testdesk"
	if runtime.GOOS == "windows" {
		name = "TestDesk.exe"
	}
	self := filepath.Join(dir, name)
	touch(t, self)
	if got := desktopBeside(self); got != "" {
		t.Fatalf("the process must not launch itself as the desktop, got %q", got)
	}
}

func TestDesktopBesideIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "testdesk-cli")
	touch(t, cli)
	if err := os.Mkdir(filepath.Join(dir, "testdesk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Mkdir(filepath.Join(dir, "TestDesk.exe"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := desktopBeside(cli); got != "" {
		t.Fatalf("a directory is not an executable, got %q", got)
	}
}
