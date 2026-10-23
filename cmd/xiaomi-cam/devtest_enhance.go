//go:build heshots

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
)

// XC_ENHTEST=<dir>: zooms 3x on the real camera (the live sharpen turns on), saves a window
// shot, then the enhanced photo (and the AI one when Real-ESRGAN is installed); prints the
// files and the time.
func init() {
	dir := os.Getenv("XC_ENHTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setGrid(false)
			u.setViewOnly(true)
			u.win.Resize(fyne.NewSize(1280, 720))
		})
		time.Sleep(10 * time.Second)
		fyne.DoAndWait(func() {
			s := u.video.Size()
			pos := fyne.NewPos(s.Width*0.9, s.Height*0.45)
			for i := 0; i < 5; i++ {
				u.zoomAt(zoomStep, &pos)
			}
		})
		time.Sleep(8 * time.Second)
		fmt.Println("zoom", u.video.dz.z, "decode", u.decodeW, "enhanced", u.decodeEnhanced, "gpu", openclSharpen.available())
		fyne.DoAndWait(func() {
			if f, err := os.Create(filepath.Join(dir, "live-zoom.png")); err == nil {
				_ = png.Encode(f, u.win.Canvas().Capture())
				_ = f.Close()
			}
		})
		t := time.Now()
		fyne.DoAndWait(u.enhancedSnapshot)
		for u.enhButton.isBusy() && time.Since(t) < 3*time.Minute {
			time.Sleep(300 * time.Millisecond)
		}
		fmt.Printf("enhanced photo in %v: %s\n", time.Since(t).Round(100*time.Millisecond), u.status.Text)
		fyne.Do(u.app.Quit)
	}
}
