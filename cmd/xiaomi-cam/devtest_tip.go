//go:build devtest

package main

import (
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// XC_TIPTEST=<dir>: show the tooltip of the record button and take a screenshot.
func init() {
	dir := os.Getenv("XC_TIPTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(6 * time.Second)
		fyne.Do(func() {
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(u.recButton)
			u.recButton.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{AbsolutePosition: pos.Add(fyne.NewPos(10, 10))}})
		})
		time.Sleep(1500 * time.Millisecond)
		fyne.Do(func() {
			if f, err := os.Create(filepath.Join(dir, "tip.jpg")); err == nil {
				_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
				_ = f.Close()
			}
			u.app.Quit()
		})
	}
}
