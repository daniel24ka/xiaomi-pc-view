package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// Window size and position are remembered separately for the normal window (with frame)
// and the video-only window (without frame), in the app preferences.

const (
	geomFramed    = "geometry.framed"
	geomFrameless = "geometry.frameless"
)

// X, Y - window position in screen pixels; W, H - window content size in Fyne units
// (set with fyne.Window.Resize, so Fyne doesn't override it with its own size)
type geometry struct {
	X, Y int
	W, H float32
}

func (u *ui) geometryKey() string {
	if u.viewOnly {
		return geomFrameless
	}
	return geomFramed
}

// saveGeometry stores the current window geometry for the current mode.
// Called every second, on mode change and when hiding to tray.
func (u *ui) saveGeometry() {
	win := glfwWindow(u.win)
	if win == nil || !u.visible || u.win.FullScreen() || win.GetAttrib(glfw.Iconified) == glfw.True {
		return
	}
	var g geometry
	g.X, g.Y = win.GetPos()
	size := u.win.Canvas().Size()
	g.W, g.H = size.Width, size.Height
	if g.W < 200 || g.H < 100 {
		return
	}
	s := fmt.Sprintf("%d,%d,%.0f,%.0f", g.X, g.Y, g.W, g.H)
	prefs := u.app.Preferences()
	if prefs.String(u.geometryKey()) != s {
		prefs.SetString(u.geometryKey(), s)
	}
}

// restoreGeometry applies the saved geometry of a mode; false if there is none or it
// doesn't fit any monitor (e.g. the monitor was disconnected).
func (u *ui) restoreGeometry(key string) bool {
	win := glfwWindow(u.win)
	if win == nil {
		return false
	}
	var g geometry
	if n, _ := fmt.Sscanf(u.app.Preferences().String(key), "%d,%d,%f,%f", &g.X, &g.Y, &g.W, &g.H); n != 4 {
		return false
	}
	if !onSomeMonitor(g) {
		return false
	}
	u.win.Resize(fyne.NewSize(g.W, g.H))
	win.SetPos(g.X, g.Y)
	return true
}

// onSomeMonitor - at least the top-left part of the window (100x50) is on a monitor
func onSomeMonitor(g geometry) bool {
	for _, m := range glfw.GetMonitors() {
		x, y, w, h := m.GetWorkarea()
		if g.X+100 > x && g.X < x+w && g.Y+50 > y && g.Y < y+h && g.W >= 200 && g.H >= 100 {
			return true
		}
	}
	return false
}
