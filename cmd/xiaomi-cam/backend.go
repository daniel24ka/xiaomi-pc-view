package main

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/xccore"
)

const (
	apiAddr  = xccore.APIAddr
	rtspAddr = xccore.RTSPAddr
)

// showRunningInstance asks an already running app to show its window.
// Only one instance can run: the camera accepts one session and ports are fixed.
func showRunningInstance() bool {
	client := http.Client{Timeout: time.Second}
	res, err := client.Post("http://"+apiAddr+"/api/xiaomi-cam/show", "", nil)
	if err != nil {
		return false
	}
	_ = res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// startBackend runs the embedded go2rtc (xccore) with the config in
// ~/.config/xiaomi-cam-viewer.
func startBackend() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return xccore.Start(filepath.Join(dir, "xiaomi-cam-viewer"))
}

// onShowRequest is called when another instance of the app was started.
func onShowRequest(f func()) {
	api.HandleFunc("api/xiaomi-cam/show", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f()
		}
	})
}
