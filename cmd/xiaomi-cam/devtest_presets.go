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

// XC_PRESETTEST=<dir>: save the position as a test point, turn away, go back, compare frames,
// then remove the test point.
func init() {
	dir := os.Getenv("XC_PRESETTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		save := func(name string) {
			fyne.Do(func() {
				if f := u.video.frame; f != nil {
					if out, err := os.Create(filepath.Join(dir, name)); err == nil {
						_ = jpeg.Encode(out, f, nil)
						_ = out.Close()
					}
				}
			})
		}
		cam := u.camera.Selected
		time.Sleep(12 * time.Second)
		save("s0.jpg")
		err := savePresetSlot(cam, 1, "test zażółć")
		fmt.Println("saved point 1:", err)
		fyne.Do(u.loadPoints)
		u.mover.press(motorLeft)
		time.Sleep(2 * time.Second)
		u.mover.release()
		time.Sleep(2 * time.Second)
		save("s1.jpg")
		fyne.Do(func() {
			fmt.Println("button 1:", u.pointNames[0].Text)
			u.pointPressed(1)
		})
		time.Sleep(6 * time.Second)
		save("s2.jpg")
		fyne.Do(func() {
			if f, err := os.Create(filepath.Join(dir, "bar.jpg")); err == nil {
				_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
				_ = f.Close()
			}
		})
		time.Sleep(500 * time.Millisecond)
		fmt.Println("delete:", deletePreset(cam, 1))
		fyne.Do(u.app.Quit)
	}
}
