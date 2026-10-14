//go:build devtest

package main

import (
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// XC_AIUI=<dir>: screenshots of the AI settings tab and the task form.
func init() {
	dir := os.Getenv("XC_AIUI")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.Do(func() {
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
					_ = f.Close()
				}
			})
		}
		time.Sleep(6 * time.Second)
		fyne.Do(func() { u.win.Resize(fyne.NewSize(1300, 800)) })
		time.Sleep(time.Second)
		var d dialog.Dialog
		fyne.Do(func() {
			d = dialog.NewCustom("AI", "OK", u.aiSettings(), u.win)
			d.Resize(fyne.NewSize(860, 640))
			d.Show()
		})
		time.Sleep(time.Second)
		shot("ai_tab.jpg")
		fyne.Do(func() {
			d.Hide()
			u.editTask(u.ai.config().Tasks[0], func() {})
		})
		time.Sleep(time.Second)
		shot("ai_form.jpg")
		fyne.Do(u.app.Quit)
	}
}
