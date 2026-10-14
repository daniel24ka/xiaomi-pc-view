//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
)

// XC_ZOOMTEST=<dir>: camera zoom 1x -> 3x -> 1x, frames saved to dir.
func init() {
	dir := os.Getenv("XC_ZOOMTEST")
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
		save("z1.jpg")
		fyne.Do(func() { u.zoomBy(1); u.zoomBy(1); u.zoomBy(1); u.zoomBy(1) }) // 3x, sent once
		time.Sleep(6 * time.Second)
		save("z3.jpg")
		fmt.Println(time.Now().Format("15:04:05"), "connected at 3x:", xiaomi.Command(u.camera.Selected, 531, `{}`), u.status.Text)
		fyne.Do(func() { u.zoomBy(-maxZoom) })
		time.Sleep(6 * time.Second)
		save("z1b.jpg")
		fmt.Println(time.Now().Format("15:04:05"), "connected at 1x:", xiaomi.Command(u.camera.Selected, 531, `{}`), u.status.Text)
		fyne.Do(u.app.Quit)
	}
}
