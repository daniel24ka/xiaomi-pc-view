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

// XC_NIGHTTEST=<dir>: night mode list (read, set, restore) and the sound checkbox spinner.
func init() {
	dir := os.Getenv("XC_NIGHTTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		state := func(tag string) {
			fyne.DoAndWait(func() {
				fmt.Printf("%s caps=%v night=%q nightDisabled=%v sound=%v soundDisabled=%v busy=%d\n", tag,
					u.caps.NightMode, u.night.Selected, u.night.Disabled(), u.sound.Checked, u.sound.Disabled(), u.busyCount)
			})
		}
		shot := func(name string) {
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, nil)
					_ = f.Close()
				}
			})
		}
		time.Sleep(12 * time.Second)
		state("start")
		shot("night_bar.jpg")
		var orig int
		fyne.DoAndWait(func() { orig = u.night.SelectedIndex() })
		fyne.Do(func() { u.night.SetSelectedIndex(0) }) // Auto – infrared
		time.Sleep(100 * time.Millisecond)
		state("setting")
		time.Sleep(5 * time.Second)
		state("set")
		props, err := getProps(u.camera.Selected, propNightShot)
		fmt.Println("camera night-shot:", props, err)
		fyne.Do(func() { u.night.SetSelectedIndex(orig) })
		time.Sleep(5 * time.Second)
		props, err = getProps(u.camera.Selected, propNightShot)
		fmt.Println("restored night-shot:", props, err)

		fyne.Do(func() { u.sound.SetChecked(!u.sound.Checked) })
		for i := 0; i < 12; i++ {
			time.Sleep(500 * time.Millisecond)
			state(fmt.Sprintf("sound %.1fs", float64(i+1)/2))
			if i == 0 {
				shot("sound_busy.jpg")
			}
		}
		fyne.Do(func() { u.sound.SetChecked(!u.sound.Checked) })
		time.Sleep(6 * time.Second)
		state("sound back")
		fyne.Do(u.app.Quit)
	}
}
