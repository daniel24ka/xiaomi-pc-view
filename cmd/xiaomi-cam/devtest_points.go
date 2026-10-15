//go:build devtest

package main

import (
	"encoding/base64"
	"fmt"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"image/png"
	"os"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
)

// XC_POINTTEST=read: prints the camera points and the motor position (read only).
// XC_POINTTEST=<n>: presses point n in the UI and prints the shown zoom afterwards.
func init() {
	mode := os.Getenv("XC_POINTTEST")
	if mode == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(10 * time.Second)
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setGrid(false)
			u.camera.SetSelected("Dom przód")
		})
		time.Sleep(6 * time.Second)
		name := "Dom przód"
		list, err := getPresets(name)
		fmt.Printf("presets: %+v err=%v\n", list, err)
		props, err := getProps(name, propMotorLocation)
		fmt.Printf("motor: %+v err=%v\n", props, err)
		if mode == "shot" {
			for i, nm := range []string{"Brama wjazdowa", "Auto", "1"} {
				err := renamePresetSlot(name, 1, nm)
				fyne.Do(u.loadPoints)
				time.Sleep(4 * time.Second)
				fyne.DoAndWait(func() {
					u.pictureControlsActive()
					img := u.win.Canvas().Capture()
					f, _ := os.Create(fmt.Sprintf("%s/point%d.png", os.Getenv("XC_POINTDIR"), i))
					_ = png.Encode(f, img)
					_ = f.Close()
					fmt.Printf("point name %q err=%v shown=%q\n", nm, err, u.pointNames[0].Text)
				})
			}
		}
		if mode == "rename2" {
			for _, nm := range []string{"test", "1"} {
				fyne.DoAndWait(func() { u.pointNameDialog(1, true) })
				time.Sleep(time.Second)
				fyne.DoAndWait(func() {
					e := findEntry(u.win.Canvas())
					fmt.Printf("entry focused=%v text=%q\n", u.win.Canvas().Focused() == e, e.Text)
					e.SetText(nm)
					tapButton(u.win.Canvas(), T("Zapisz"))
				})
				time.Sleep(6 * time.Second)
				l, _ := getPresets(name)
				fyne.DoAndWait(func() {
					fmt.Printf("renamed to %q: camera=%+v button=%q status=%q\n", nm, l, u.pointNames[0].Text, u.status.Text)
				})
			}
		}
		if mode == "dialog" {
			for _, nm := range []string{"test", "1"} {
				fyne.DoAndWait(u.showPoints)
				time.Sleep(time.Second)
				fyne.DoAndWait(func() {
					tapButton(u.win.Canvas(), T("Zapisz bieżącą pozycję")) // the first one: point 1
				})
				time.Sleep(time.Second)
				fyne.DoAndWait(func() {
					e := findEntry(u.win.Canvas())
					fmt.Printf("entry found=%v text=%q\n", e != nil, func() string {
						if e != nil {
							return e.Text
						}
						return ""
					}())
					if e != nil {
						e.SetText(nm)
					}
					tapButton(u.win.Canvas(), T("Zapisz"))
				})
				time.Sleep(6 * time.Second)
				l, _ := getPresets(name)
				fyne.DoAndWait(func() {
					fmt.Printf("dialog rename to %q: camera=%+v button=%q status=%q\n", nm, l, u.pointNames[0].Text, u.status.Text)
				})
			}
		}
		if mode == "uirename" {
			for _, nm := range []string{"test", "1"} {
				err := savePresetSlot(name, 1, nm)
				fyne.Do(u.loadPoints)
				time.Sleep(4 * time.Second)
				l, _ := getPresets(name)
				fyne.DoAndWait(func() {
					fmt.Printf("ui rename to %q: err=%v camera=%+v button=%q\n", nm, err, l, u.pointNames[0].Text)
				})
			}
		}
		if mode == "rename" {
			for _, nm := range []string{"test", "1"} {
				l, _ := getPresets(name)
				for i := range l {
					if l[i].Idx == 1 {
						l[i].Name = base64.StdEncoding.EncodeToString([]byte(nm))
					}
				}
				err := putPresets(name, l)
				time.Sleep(2 * time.Second)
				l2, err2 := getPresets(name)
				fmt.Printf("rename to %q: put err=%v; read %+v err=%v\n", nm, err, l2, err2)
			}
		}
		if n, err := strconv.Atoi(mode); err == nil {
			fyne.DoAndWait(func() { u.setZoom(2) })
			time.Sleep(5 * time.Second)
			fyne.DoAndWait(func() { fmt.Printf("before: camZoom=%.1f label=%q\n", u.camZoom, u.zoomLabel.Text) })
			fyne.Do(func() { u.pointPressed(n) })
			time.Sleep(10 * time.Second)
			fyne.DoAndWait(func() { fmt.Printf("after point %d: camZoom=%.1f label=%q\n", n, u.camZoom, u.zoomLabel.Text) })
			props, err = getProps(name, propMotorLocation)
			fmt.Printf("motor: %+v err=%v\n", props, err)
		}
		fyne.Do(u.app.Quit)
	}
}

func walkObjects(o fyne.CanvasObject, f func(fyne.CanvasObject) bool) bool {
	if f(o) {
		return true
	}
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			if walkObjects(c, f) {
				return true
			}
		}
	case fyne.Widget:
		r := test.WidgetRenderer(v)
		for _, c := range r.Objects() {
			if walkObjects(c, f) {
				return true
			}
		}
	}
	return false
}

// tapButton taps the first button with the text in the top overlay (dialog).
func tapButton(c fyne.Canvas, text string) {
	ov := c.Overlays().Top()
	walkObjects(ov, func(o fyne.CanvasObject) bool {
		if b, ok := o.(*widget.Button); ok && b.Text == text {
			test.Tap(b)
			return true
		}
		if b, ok := o.(*ptrButton); ok && b.Text == text {
			test.Tap(b)
			return true
		}
		return false
	})
}

func findEntry(c fyne.Canvas) *widget.Entry {
	var e *widget.Entry
	walkObjects(c.Overlays().Top(), func(o fyne.CanvasObject) bool {
		if v, ok := o.(*widget.Entry); ok {
			e = v
			return true
		}
		return false
	})
	return e
}
