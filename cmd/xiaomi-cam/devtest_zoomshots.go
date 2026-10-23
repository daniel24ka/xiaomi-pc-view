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

// XC_ZOOMSHOTS=<dir>: the digital zoom on the real camera: 1x, 4x right after zooming (still
// the window-sized decode = a stretched picture) and 4x after the sharper decode tier.
func init() {
	dir := os.Getenv("XC_ZOOMSHOTS")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = png.Encode(f, img)
					_ = f.Close()
				}
			})
		}
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setGrid(false)
			u.setViewOnly(true) // the picture only
			u.win.Resize(fyne.NewSize(1280, 720))
		})
		time.Sleep(10 * time.Second)
		shot("zoom1.png")
		fmt.Println("decode width at 1x:", u.decodeW)
		fyne.DoAndWait(func() {
			s := u.video.Size()
			pos := fyne.NewPos(s.Width*0.9, s.Height*0.45) // the building on the right: windows, balconies
			for i := 0; i < 6; i++ {
				u.zoomAt(zoomStep, &pos)
			}
		})
		time.Sleep(150 * time.Millisecond)
		shot("zoom4-stretched.png")
		fmt.Println("zoom:", u.video.dz.z, "decode width right after zooming:", u.decodeW)
		time.Sleep(7 * time.Second)
		shot("zoom4-sharp.png")
		fmt.Println("decode width after the tier switch:", u.decodeW)
		fyne.Do(u.app.Quit)
	}
}
