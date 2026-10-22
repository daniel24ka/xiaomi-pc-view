//go:build heshots

package main

import (
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// XC_HESHOTS=<dir>: screenshots of the Hebrew UI (main, menu, add camera, Mi login form,
// settings, shortcuts). No Mi login is started.
func init() {
	dir := os.Getenv("XC_HESHOTS")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			time.Sleep(800 * time.Millisecond)
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
					_ = f.Close()
				}
			})
		}
		closeAll := func() {
			fyne.DoAndWait(func() {
				for _, o := range u.win.Canvas().Overlays().List() {
					u.win.Canvas().Overlays().Remove(o)
				}
			})
		}
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.win.Resize(fyne.NewSize(1100, 700))
		})
		time.Sleep(3 * time.Second)
		closeAll()
		shot("he-main.jpg")
		fyne.DoAndWait(func() { test.Tap(u.menuButton) })
		shot("he-menu.jpg")
		closeAll()
		fyne.DoAndWait(func() { showChooseCamera(u.win, func(string) {}) })
		shot("he-add.jpg")
		closeAll()
		fyne.DoAndWait(func() { showLogin(u.win, func(string) {}) })
		shot("he-login.jpg")
		closeAll()
		fyne.DoAndWait(u.showSettings)
		shot("he-settings.jpg")
		closeAll()
		fyne.DoAndWait(u.showShortcuts)
		shot("he-shortcuts.jpg")
		closeAll()
		fyne.Do(u.app.Quit)
	}
}
