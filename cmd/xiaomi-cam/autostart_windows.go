//go:build windows

package main

import "golang.org/x/sys/windows/registry"

// Start at login hidden in the tray: the value "Xiaomi Cam" of
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run (the user's startup programs).

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "Xiaomi Cam"
)

func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

func setAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err = k.DeleteValue(runValue); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	// the main program, also from its softgl copy: it picks the OpenGL at every start
	return k.SetStringValue(runValue, `"`+appExe()+`" --tray`)
}
