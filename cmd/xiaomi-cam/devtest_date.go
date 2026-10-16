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

// XC_DATETEST=<dir>: the history date picker: open history, screenshots of the calendar in
// Polish and English (it must be inside the window), pick yesterday.
func init() {
	dir := os.Getenv("XC_DATETEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			time.Sleep(700 * time.Millisecond)
			fyne.DoAndWait(func() {
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, u.win.Canvas().Capture(), &jpeg.Options{Quality: 90})
					_ = f.Close()
				}
			})
		}
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setViewOnly(false)
			u.setGrid(false)
			u.camera.SetSelected("Dom przód")
			u.win.Resize(fyne.NewSize(1280, 800))
		})
		time.Sleep(8 * time.Second)
		fyne.Do(u.openHistory)
		time.Sleep(6 * time.Second)
		report := func(tag string) {
			fyne.DoAndWait(func() {
				p := u.hist.date.pop
				if p == nil || !p.Visible() {
					fmt.Println(tag, "calendar not shown")
					return
				}
				pos, size, win := p.Content.Position(), p.Content.Size(), u.win.Canvas().Size()
				fmt.Printf("%s: calendar at %v size %v, window %v, inside %v\n", tag, pos, size, win,
					pos.X >= 0 && pos.Y >= 0 && pos.X+size.Width <= win.Width && pos.Y+size.Height <= win.Height)
			})
		}
		fyne.Do(u.hist.date.open)
		shot("date_pl.jpg")
		report("pl")
		fyne.DoAndWait(func() { u.hist.date.show(u.hist.date.month.AddDate(0, -1, 0)) })
		shot("date_pl_prev.jpg")
		report("pl, previous month")
		fyne.DoAndWait(func() { u.hist.date.pop.Hide() })
		time.Sleep(time.Second)
		fyne.DoAndWait(func() {
			lang = "en"
			u.hist.date.open()
		})
		shot("date_en.jpg")
		report("en")
		fyne.DoAndWait(func() {
			lang = "pl"
			u.hist.date.show(time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local))
			u.hist.date.pick(time.Now().AddDate(0, 0, -1))
		})
		time.Sleep(12 * time.Second)
		fyne.DoAndWait(func() {
			fmt.Printf("picked yesterday: button %q, position %s, status %q\n", u.hist.date.btn.Text,
				u.historyPosition().Format("02.01 15:04:05"), u.status.Text)
		})
		shot("date_yesterday.jpg")
		fyne.Do(u.goLive)
		time.Sleep(3 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
