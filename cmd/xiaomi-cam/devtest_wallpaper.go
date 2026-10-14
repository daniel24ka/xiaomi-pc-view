//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_WALLTEST=<file.jpg>: wallpaper mode on for a few seconds, then off; prints the X11 window id
// and geometry so an outside script can check the window type and input region.
func init() {
	if os.Getenv("XC_WALLTEST") == "" {
		return
	}
	devHook = func(u *ui) {
		geom := func(tag string) {
			fyne.DoAndWait(func() {
				win := glfwWindow(u.win)
				x, y := win.GetPos()
				w, h := win.GetSize()
				fmt.Printf("%s xid=%d pos=%d,%d size=%dx%d wallpaper=%v viewOnly=%v visible=%v\n", tag, win.GetX11Window(), x, y, w, h, u.wallpaper, u.viewOnly, u.visible)
			})
		}
		time.Sleep(10 * time.Second)
		geom("before")
		fyne.Do(func() { u.setWallpaper(true) })
		time.Sleep(4 * time.Second)
		geom("WALLPAPER")
		fyne.DoAndWait(func() {
			img := u.win.Canvas().Capture()
			if f, err := os.Create(os.Getenv("XC_WALLTEST")); err == nil {
				_ = jpeg.Encode(f, img, nil)
				_ = f.Close()
			}
		})
		time.Sleep(8 * time.Second)
		fyne.Do(func() { u.setWallpaper(false) })
		time.Sleep(3 * time.Second)
		geom("NORMAL")
		time.Sleep(3 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
