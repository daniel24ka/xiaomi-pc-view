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

// XC_PTZTEST=<dir>: short click and long hold; frames 1 s and 4 s after release must match
// (the camera stopped).
func init() {
	dir := os.Getenv("XC_PTZTEST")
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
		time.Sleep(15 * time.Second)
		save("p0.jpg")
		u.mover.press(motorRight) // short click
		time.Sleep(100 * time.Millisecond)
		u.mover.release()
		time.Sleep(time.Second)
		save("p1.jpg")
		time.Sleep(3 * time.Second)
		save("p2.jpg")
		u.mover.press(motorLeft) // hold 1.5 s
		time.Sleep(1500 * time.Millisecond)
		u.mover.release()
		time.Sleep(time.Second)
		save("p3.jpg")
		time.Sleep(3 * time.Second)
		save("p4.jpg")
		fmt.Println("done")
		fyne.Do(u.app.Quit)
	}
}
