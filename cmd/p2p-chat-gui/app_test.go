//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDBPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configDir, appDirName, defaultDBName)

	got, err := resolveDBPath("")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(got)); err != nil {
		t.Fatal(err)
	}

	absolute := filepath.Join(t.TempDir(), "chat.db")
	got, err = resolveDBPath(absolute)
	if err != nil {
		t.Fatal(err)
	}
	if got != absolute {
		t.Fatalf("absolute path = %q, want %q", got, absolute)
	}
}
