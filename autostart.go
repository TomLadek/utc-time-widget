//go:build windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Autostart uses the per-user Run key, which needs no admin rights and is
// shown (and can be disabled by the user) in Task Manager's "Startup apps".
const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "utc-time-widget"
)

// enableAutostart registers the current executable to start at logon. It is
// called on every launch, so moving the executable updates the entry.
func enableAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	cmd := `"` + exe + `"`
	if cur, _, err := k.GetStringValue(runValueName); err == nil && cur == cmd {
		return nil
	}
	return k.SetStringValue(runValueName, cmd)
}

func disableAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(runValueName); err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return err
	}
	return nil
}
