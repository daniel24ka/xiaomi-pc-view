//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
)

// XC_UITEST=<dir>: screenshots of the main window and the cameras dialog.
func init() {
	dir := os.Getenv("XC_UITEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.Do(func() {
				img := u.win.Canvas().Capture()
				fmt.Println(name, img.Bounds().Dx(), "x", img.Bounds().Dy())
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, nil)
					_ = f.Close()
				}
			})
		}
		time.Sleep(10 * time.Second)
		shot("ui_main.jpg")
		fyne.Do(u.toggleRecording)
		time.Sleep(3 * time.Second)
		shot("ui_recording.jpg")
		fyne.Do(u.toggleRecording)
		time.Sleep(2 * time.Second)
		fyne.Do(u.showSettings)
		time.Sleep(time.Second)
		shot("ui_settings.jpg")
		time.Sleep(500 * time.Millisecond)
		fyne.Do(u.app.Quit)
	}
}
