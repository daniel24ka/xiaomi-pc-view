//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The UI needs OpenGL 2.1. Without a graphics driver (a virtual machine, Remote Desktop,
// a PC without the GPU driver) Windows has only OpenGL 1.1 and no window opens. The
// installer has a second copy of the app in softgl\ with the software OpenGL of Mesa next
// to it (the app links opengl32.dll, which Windows takes from the app's folder first).
// glprobe\xc-glprobe.exe (no OpenGL of its own) tests the driver at every start; without
// OpenGL 2.1 the app starts its softgl copy and exits. Normal PCs use the driver.
// XIAOMI_CAM_SOFTGL=1 forces Mesa.
//
// Mesa's driver is softpipe (GALLIUM_DRIVER): llvmpipe ends the app with exit code 1 (seen
// in a Windows 11 VM; in xc-glprobe llvmpipe works).

const softGLDir = "softgl"

// appDir - the folder of the app (also for its softgl copy): ffmpeg, autostart
func appDir() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(self)
	if strings.EqualFold(filepath.Base(dir), softGLDir) {
		return filepath.Dir(dir)
	}
	return dir
}

// appExe - the main program (not its softgl copy)
func appExe() string { return filepath.Join(appDir(), "xiaomi-cam.exe") }

// prepareOpenGL: call before the UI starts (it may start the softgl copy and exit).
func prepareOpenGL() {
	self, err := os.Executable()
	if err != nil || strings.EqualFold(filepath.Base(filepath.Dir(self)), softGLDir) {
		return
	}
	dir := filepath.Dir(self)
	soft := filepath.Join(dir, softGLDir, "xiaomi-cam.exe")
	probe := filepath.Join(dir, "glprobe", "xc-glprobe.exe")
	if !fileExists(soft) || !fileExists(probe) {
		return
	}
	if os.Getenv("XIAOMI_CAM_SOFTGL") != "1" && driverGL(probe) {
		return
	}
	cmd := exec.Command(soft, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "GALLIUM_DRIVER=softpipe")
	detach(cmd)
	if err = cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "xiaomi-cam: software OpenGL:", err)
		return
	}
	os.Exit(0)
}

// driverGL runs xc-glprobe.exe: true - the driver has OpenGL 2.1+ (or the test failed to
// run: leave it to the system).
func driverGL(probe string) bool {
	cmd := exec.Command(probe)
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		return true
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if e, ok := err.(*exec.ExitError); ok {
			return e.ExitCode() != 1
		}
		return true
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		return true
	}
}
