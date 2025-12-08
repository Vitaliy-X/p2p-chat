//go:build desktop || bindings

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	appDirName    = "p2p-chat"
	defaultDBName = "p2p-chat-gui.db"
)

func resolveDBPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultDBName
	}
	if isSQLiteDSN(path) {
		return path, nil
	}
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		baseDir, err := appDataDir()
		if err != nil {
			return "", err
		}
		expanded = filepath.Join(baseDir, expanded)
	}
	expanded = filepath.Clean(expanded)
	if err := os.MkdirAll(filepath.Dir(expanded), 0o700); err != nil {
		return "", fmt.Errorf("create sqlite directory: %w", err)
	}
	return expanded, nil
}

func isSQLiteDSN(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file:")
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func appDataDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err == nil {
		return filepath.Join(configDir, appDirName), nil
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return "", fmt.Errorf("resolve app data directory: %w", err)
	}
	return filepath.Join(home, "."+appDirName), nil
}
