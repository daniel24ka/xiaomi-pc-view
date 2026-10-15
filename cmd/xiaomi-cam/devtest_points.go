//go:build devtest

package main

import (
	"fmt"
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
