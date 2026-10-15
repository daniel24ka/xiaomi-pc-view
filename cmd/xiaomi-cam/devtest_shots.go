//go:build devtest

package main

import (
	"fmt"
	"fyne.io/fyne/v2/widget"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// XC_SHOTS=<dir>: screenshots of the main views and dialogs (for checking the look):
// main.jpg, light.jpg, menu.jpg, cameras.jpg, settings.jpg, grid.jpg.
func init() {
	dir := os.Getenv("XC_SHOTS")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			time.Sleep(800 * time.Millisecond)
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
					_ = f.Close()
				}
			})
		}
		closeAll := func() {
			fyne.DoAndWait(func() {
				for _, o := range u.win.Canvas().Overlays().List() {
					u.win.Canvas().Overlays().Remove(o)
				}
			})
		}
		fyne.DoAndWait(func() {
			// the test window may take the focus while the user types elsewhere: no shortcuts
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
		})
		time.Sleep(4 * time.Second)
		fyne.DoAndWait(func() {
			u.setViewOnly(false)
			u.setGrid(false)
			u.camera.SetSelected("Dom przód")
			u.win.Resize(fyne.NewSize(1280, 800))
		})
		time.Sleep(8 * time.Second)
		fyne.DoAndWait(func() {
			if u.onVideoMouse != nil {
				u.onVideoMouse() // the controls on the picture are shown
			}
		})
		fyne.DoAndWait(func() {
			d := fyne.CurrentApp().Driver()
			for n, o := range map[string]fyne.CanvasObject{"pictureControls": u.pictureControls, "bottom": u.bottomControls,
				"chip": u.statusChip, "ptz": u.capPTZ, "points": u.capPoints, "zoom": u.capZoom, "status": u.status, "videoArea": u.videoArea} {
				fmt.Printf("%s visible=%v pos=%v size=%v min=%v\n", n, o.Visible(), d.AbsolutePositionForObject(o), o.Size(), o.MinSize())
			}
		})
		shot("main.jpg")
		fyne.DoAndWait(func() { test.Tap(u.camControls) })
		shot("light.jpg")
		closeAll()
		fyne.DoAndWait(func() { test.Tap(u.menuButton) })
		shot("menu.jpg")
		closeAll()
		fyne.DoAndWait(u.showCameras)
		shot("cameras.jpg")
		closeAll()
		fyne.DoAndWait(u.showSettings)
		shot("settings.jpg")
		closeAll()
		fyne.DoAndWait(func() { u.setGrid(true) })
		time.Sleep(6 * time.Second)
		fyne.DoAndWait(func() {
			for _, t := range u.tiles {
				walkLabels(t.obj, func(l *widget.Label) {
					fmt.Printf("tile %q label size=%v min=%v trunc=%v\n", t.name, l.Size(), l.MinSize(), l.Truncation)
				})
			}
		})
		shot("grid.jpg")
		fyne.DoAndWait(func() { u.setGrid(false) })
		time.Sleep(time.Second)
		fyne.Do(u.app.Quit)
	}
}

func walkLabels(o fyne.CanvasObject, f func(*widget.Label)) {
	switch v := o.(type) {
	case *widget.Label:
		f(v)
	case *fyne.Container:
		for _, c := range v.Objects {
			walkLabels(c, f)
		}
	}
}
