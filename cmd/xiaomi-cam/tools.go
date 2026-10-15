package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// toolCommand runs ffmpeg / ffprobe: on Windows the ones next to the app (the installer
// puts them there) or in PATH, without a console window; on Linux from PATH.
func toolCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	path := name
	if runtime.GOOS == "windows" {
		if p := filepath.Join(appDir(), name+".exe"); fileExists(p) {
			path = p
		}
	}
	var cmd *exec.Cmd
	if ctx != nil {
		cmd = exec.CommandContext(ctx, path, args...)
	} else {
		cmd = exec.Command(path, args...)
	}
	hideConsole(cmd)
	return cmd
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
