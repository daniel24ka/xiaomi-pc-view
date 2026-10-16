//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// XC_AREATEST=<dir>: the virtual fence editor on the live picture: open, drag a corner,
// save (13.1), read the area and the motor position (it must not change) back, screenshots;
// then the fence entry of 13.1 is put back as it was.
func init() {
	dir := os.Getenv("XC_AREATEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			time.Sleep(900 * time.Millisecond)
			fyne.DoAndWait(func() {
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, u.win.Canvas().Capture(), &jpeg.Options{Quality: 90})
					_ = f.Close()
				}
			})
		}
		props := func(tag string) {
			p, err := getProps(u.camera.Selected, xccore.PropAIArea, xccore.PropMotorLocation, xccore.PropAreaOSD)
			fmt.Printf("%s: err %v\n", tag, err)
			for _, v := range p {
				fmt.Printf("   %d.%d code %d %v\n", v.Siid, v.Piid, v.Code, v.Value)
			}
		}
		const name = "Dom przód"
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setViewOnly(false)
			u.setGrid(false)
			u.camera.SetSelected(name)
			u.win.Resize(fyne.NewSize(1280, 800))
		})
		time.Sleep(10 * time.Second)
		det, err := getDetection(name)
		if err != nil {
			fmt.Println("detection:", err)
			fyne.Do(u.app.Quit)
			return
		}
		orig := det.Fence
		fmt.Printf("fence before: %+v cruise %v tracking %v\n", orig, det.Cruise, det.Tracking)
		props("before")
		fyne.DoAndWait(func() { u.editArea(det, xccore.AreaFence) })
		shot("area_open.jpg")
		fyne.DoAndWait(func() {
			e := u.areaEdit
			// a new rectangle: the pavement in front of the gate
			from := e.toWidget(120, 200)
			to := e.toWidget(420, 330)
			e.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: to}, Dragged: fyne.NewDelta(to.X-from.X, to.Y-from.Y)})
			e.DragEnd()
			fmt.Printf("drawn: %v\n", e.r)
		})
		shot("area_drawn.jpg")
		fyne.DoAndWait(func() { u.areaButtons.Objects[1].(*ptrButton).OnTapped() })
		time.Sleep(6 * time.Second)
		closeTop := func() {
			fyne.DoAndWait(func() {
				if top := u.win.Canvas().Overlays().Top(); top != nil {
					u.win.Canvas().Overlays().Remove(top)
				}
			})
		}
		shot("area_saved_dialog.jpg")
		closeTop()
		fyne.DoAndWait(func() { fmt.Printf("status %q, editor visible %v\n", u.status.Text, u.areaBox.Visible()) })
		props("after save")
		time.Sleep(5 * time.Second)
		shot("area_live.jpg") // the camera draws the fence (13.6)
		props("5 s later")

		// put the fence back
		err = xccore.SetArea(name, orig)
		fmt.Println("restore:", err)
		time.Sleep(3 * time.Second)
		props("restored")
		fyne.Do(u.app.Quit)
	}
}
