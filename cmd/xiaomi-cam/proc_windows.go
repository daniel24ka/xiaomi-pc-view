//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow        = 0x08000000
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// hideConsole: ffmpeg is a console program; without this every start flashes a window.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// detach keeps the process running after this one exits.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: detachedProcess | createNewProcessGroup}
}

// ffmpeg.exe is installed next to the app
const ffmpegMissing = "ffmpeg: %w (zainstaluj aplikację ponownie: brak ffmpeg.exe)"
