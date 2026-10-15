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

// XC_VIEWTEST=<dir>: video only on and off again, in the single view and in the grid;
// screenshots and the state of the picture.
func init() {
	dir := os.Getenv("XC_VIEWTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, nil)
					_ = f.Close()
				}
			})
		}
		state := func(tag string) {
			fyne.DoAndWait(func() {
				x, y := glfwWindow(u.win).GetPos()
				fmt.Printf("winpos=%d,%d ", x, y)
				fmt.Printf("%s: grid=%v viewOnly=%v canvas=%v videoArea=%v pos=%v video=%v visible=%v status=%q\n", tag, u.grid, u.viewOnly,
					u.win.Canvas().Size(), u.videoArea.Size(), u.videoArea.Position(), u.video.Size(), u.visible, u.status.Text)
				for _, t := range u.tiles {
					fmt.Printf("   tile %q size=%v frame=%v\n", t.name, t.view.Size(), t.view.frame != nil)
				}
			})
		}
		time.Sleep(10 * time.Second)
		for _, grid := range []bool{false, true} {
			fyne.DoAndWait(func() { u.setGrid(grid) })
			time.Sleep(6 * time.Second)
			tag := map[bool]string{false: "single", true: "grid"}[grid]
			state(tag + " start")
			fyne.DoAndWait(func() { u.setViewOnly(true) })
			time.Sleep(4 * time.Second)
			state(tag + " video only")
			shot(tag + "_only.jpg")
			fyne.DoAndWait(func() { u.setViewOnly(false) })
			fmt.Println("BACK", tag)
			time.Sleep(4 * time.Second)
			state(tag + " back")
			shot(tag + "_back.jpg")
		}
		// a real minimize still hides to the tray
		fyne.DoAndWait(func() { glfwWindow(u.win).Iconify() })
		time.Sleep(2 * time.Second)
		state("minimized")
		fyne.DoAndWait(u.show)
		time.Sleep(5 * time.Second)
		state("shown")
		fyne.Do(u.app.Quit)
	}
}
