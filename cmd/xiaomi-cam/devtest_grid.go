//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// XC_GRIDTEST=<dir>: a temporary second "camera" relaying the first one (no second camera
// session, not saved), grid view, select tiles, back to one camera.
func init() {
	dir := os.Getenv("XC_GRIDTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		do := func(f func()) {
			done := make(chan struct{})
			fyne.Do(func() { f(); close(done) })
			<-done
		}
		shot := func(name string) {
			do(func() {
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
					_ = f.Close()
				}
			})
		}
		cam := u.camera.Selected
		time.Sleep(8 * time.Second)
		_, err := streams.New("Test relay", rtspURL(cam))
		fmt.Println("relay:", err)
		do(func() { u.camerasChanged(); fmt.Println("grid button visible:", u.gridButton.Visible()) })
		do(func() { u.setGrid(true) })
		time.Sleep(15 * time.Second)
		do(func() {
			for _, t := range u.tiles {
				fmt.Println("tile", t.name, "frame:", t.view.frame != nil)
			}
		})
		shot("grid1.jpg")
		do(func() { u.selectTile("Test relay") })
		time.Sleep(time.Second)
		do(func() { fmt.Println("selected:", u.camera.Selected, "| current:", u.current.Load()) })
		shot("grid2.jpg")
		do(func() { u.selectTile(cam); u.setGrid(false) })
		time.Sleep(10 * time.Second)
		do(func() { fmt.Println("single view, frame:", u.video.frame != nil, "selected:", u.camera.Selected) })
		streams.Delete("Test relay")
		fyne.Do(u.app.Quit)
	}
}
