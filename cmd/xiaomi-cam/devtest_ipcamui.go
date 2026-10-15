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

// XC_IPUITEST=<dir>: screenshots of the "Add camera" dialogs (choice, IP camera form of
// each kind, ONVIF search).
func init() {
	dir := os.Getenv("XC_IPUITEST")
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
		closeAll := func() {
			fyne.DoAndWait(func() {
				for _, o := range u.win.Canvas().Overlays().List() {
					u.win.Canvas().Overlays().Remove(o)
				}
			})
		}
		time.Sleep(6 * time.Second)
		fyne.Do(func() { showChooseCamera(u.win, func(string) {}) })
		time.Sleep(time.Second)
		shot("choose.jpg")
		closeAll()
		for i, k := range ipKinds {
			fyne.Do(func() { showAddIPCamera(u.win, &ipForm{Kind: k.id}, "", func(string, *ipCamera) error { return nil }) })
			time.Sleep(time.Second)
			shot(fmt.Sprintf("form_%s.jpg", k.id))
			closeAll()
			_ = i
		}
		fyne.Do(u.app.Quit)
	}
}
