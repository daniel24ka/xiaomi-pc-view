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

// XC_HISTTEST=<dir>: history UI flow - open, step back 10 min, back to live; screenshots.
func init() {
	dir := os.Getenv("XC_HISTTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.Do(func() {
				fmt.Println(time.Now().Format("15:04:05"), name, "| status:", u.status.Text, "| frame:", u.video.frame != nil)
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
					_ = f.Close()
				}
			})
		}
		time.Sleep(12 * time.Second)
		fyne.Do(u.openHistory)
		time.Sleep(30 * time.Second)
		shot("hist1.jpg")
		fyne.Do(func() { u.playHistory(u.historyTime().Add(-10 * time.Minute)) })
		time.Sleep(30 * time.Second)
		shot("hist2.jpg")
		fyne.Do(u.goLive)
		time.Sleep(12 * time.Second)
		shot("live.jpg")
		fyne.Do(u.app.Quit)
	}
}
