//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Start at login hidden in the tray: ~/.config/autostart/xiaomi-cam.desktop (XDG autostart).

func autostartPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "autostart", "xiaomi-cam.desktop")
}

func autostartEnabled() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

func setAutostart(on bool) error {
	path := autostartPath()
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe := "xiaomi-cam" // installed from the .deb, in PATH
	if p, err := exec.LookPath("xiaomi-cam"); err != nil || !samePath(p) {
		if self, err := os.Executable(); err == nil {
			exe = self // started from another place (plain binary)
		}
	}
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Xiaomi Cam
Comment=Xiaomi Mi Home camera viewer (in the tray)
Exec="%s" --tray
Icon=xiaomi-cam
Terminal=false
X-GNOME-Autostart-enabled=true
`, exe)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(entry), 0o644)
}

// samePath - the xiaomi-cam in PATH is this program
func samePath(p string) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	a, _ := filepath.EvalSymlinks(p)
	b, _ := filepath.EvalSymlinks(self)
	return a == b
}
