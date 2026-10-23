//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func hideConsole(*exec.Cmd) {}

func raisePriority() {} // Windows only

// detach keeps the process running after this one exits (its own session).
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

const ffmpegMissing = "ffmpeg: %w (zainstaluj: sudo apt install ffmpeg)"

// prepareOpenGL: Windows only (gl_windows.go)
func prepareOpenGL() {}

func appDir() string { return "" } // Windows only
