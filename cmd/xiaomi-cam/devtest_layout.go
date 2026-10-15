//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// XC_LAYOUTTEST=<dir>: minimum widths of the bars, the window at its minimum width, the
// light panel and the menu open (screenshots).
func init() {
	dir := os.Getenv("XC_LAYOUTTEST")
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
		sizes := func(tag string) {
			fyne.DoAndWait(func() {
				fmt.Printf("%s: top=%v bottom=%v content=%v window=%v video=%v hist=%v(%v)\n", tag, u.bars[0].MinSize(), u.bars[1].MinSize(),
					u.win.Content().MinSize(), u.win.Canvas().Size(), u.videoArea.MinSize(), u.hist.bar.MinSize(), u.hist.bar.Visible())
				for i, o := range u.videoArea.Objects {
					fmt.Printf("   video[%d] %T min=%v visible=%v\n", i, o, o.MinSize(), o.Visible())
				}
				if c, ok := u.win.Content().(*fyne.Container); ok {
					for i, o := range c.Objects {
						fmt.Printf("   content[%d] %T min=%v visible=%v\n", i, o, o.MinSize(), o.Visible())
					}
				}
			})
		}
		time.Sleep(10 * time.Second)
		for _, name := range cameraNames() {
			fyne.Do(func() { u.camera.SetSelected(name) })
			time.Sleep(4 * time.Second)
			sizes(name)
		}
		fyne.Do(func() { u.camera.SetSelected("Dom przód") })
		time.Sleep(5 * time.Second)
		fyne.DoAndWait(func() { u.win.Resize(fyne.NewSize(100, 600)) })
		time.Sleep(2 * time.Second)
		sizes("min")
		shot("min.jpg")
		fyne.DoAndWait(func() { test.Tap(u.camControls) })
		time.Sleep(time.Second)
		shot("light.jpg")
		fyne.DoAndWait(func() {
			for _, o := range u.win.Canvas().Overlays().List() {
				u.win.Canvas().Overlays().Remove(o)
			}
		})
		fyne.DoAndWait(func() { u.setGrid(true) })
		time.Sleep(8 * time.Second)
		sizes("grid")
		shot("grid_min.jpg")
		fyne.DoAndWait(func() { u.win.Resize(fyne.NewSize(1400, 900)) })
		time.Sleep(10 * time.Second)
		sizes("grid big")
		fyne.DoAndWait(func() { fmt.Printf("grid status: %q\n", u.status.Text) })
		shot("grid5.jpg")
		fyne.DoAndWait(func() { u.setGrid(false) })
		time.Sleep(2 * time.Second)

		// edit: the form of an IP camera, saving unchanged, renaming and back
		const cam = "Pula (Chorwacja)"
		fyne.DoAndWait(func() { u.editCamera(cam, dialog.NewCustom("", "", widget.NewLabel(""), u.win), func() {}) })
		time.Sleep(time.Second)
		shot("edit.jpg")
		fyne.DoAndWait(func() {
			for _, o := range u.win.Canvas().Overlays().List() {
				u.win.Canvas().Overlays().Remove(o)
			}
		})
		playing := func(tag, name string) {
			start := time.Now()
			for time.Since(start) < 20*time.Second {
				var ok bool
				fyne.DoAndWait(func() { ok = u.camera.Selected == name && u.gotFrame })
				if ok {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			fyne.DoAndWait(func() {
				fmt.Printf("%s: selected=%q frame=%v (%.1fs) options=%v ipcam=%v\n", tag, u.camera.Selected, u.gotFrame,
					time.Since(start).Seconds(), u.camera.Options, ipCam(name) != nil)
			})
		}
		fyne.Do(func() { u.camera.SetSelected(cam) })
		playing("selected", cam)
		c := ipCam(cam)
		save := func(old, name string) {
			fyne.DoAndWait(func() {
				if u.camera.Selected == old {
					u.player.stop()
				}
				err := updateIPCamera(old, name, c)
				u.camerasChanged()
				u.camera.Selected = ""
				u.camera.SetSelected(name)
				fmt.Println("update", old, "->", name, err)
			})
		}
		save(cam, cam)
		playing("saved unchanged", cam)
		save(cam, "Pula test")
		playing("renamed", "Pula test")
		save("Pula test", cam)
		playing("renamed back", cam)
		fmt.Println("cameras:", cameraNames())
		fyne.Do(u.app.Quit)
	}
}
