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

// XC_TLTEST=<dir>: the history timeline: open history, screenshot, hover over a minute with
// a thumbnail (the preview), zoom in and out.
func init() {
	dir := os.Getenv("XC_TLTEST")
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
		time.Sleep(12 * time.Second) // live: a thumbnail of this minute
		live := time.Now()
		fyne.Do(u.openHistory)
		time.Sleep(12 * time.Second)
		shot("tl_history.jpg")
		fyne.DoAndWait(func() {
			fmt.Printf("position %s, status %q, tl %s..%s\n", u.historyPosition().Format("15:04:05"), u.status.Text,
				u.hist.tl.from().Format("15:04"), u.hist.tl.from().Add(u.hist.tl.span).Format("15:04"))
			u.hist.tl.onHover = nil // a real mouse over the test window doesn't move it
			x := u.hist.tl.xAt(live)
			u.hist.tl.hover = live
			u.hist.tl.Refresh()
			u.showHistoryPreview(live, x)
			fmt.Println("preview thumb:", u.histPreview.path)
		})
		shot("tl_hover.jpg")
		fyne.DoAndWait(func() {
			u.hist.tl.span = 20 * time.Minute
			u.hist.tl.Refresh()
		})
		shot("tl_zoom_in.jpg")
		fyne.DoAndWait(func() {
			u.hist.tl.span = 12 * time.Hour
			u.hist.tl.Refresh()
		})
		shot("tl_zoom_out.jpg")
		// drag the scale back to 2:00 at night (2 h range), click there
		night := time.Date(live.Year(), live.Month(), live.Day(), 2, 0, 0, 0, time.Local)
		fyne.DoAndWait(func() {
			tl := u.hist.tl
			tl.span = 2 * time.Hour
			for i := 0; tl.center.After(night) && i < 1000; i++ {
				tl.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(300, 20)},
					Dragged: fyne.NewDelta(min(40, tl.xAt(tl.center)-tl.xAt(night)), 0)})
			}
			tl.DragEnd()
			fmt.Printf("dragged: tl %s..%s\n", tl.from().Format("15:04"), tl.from().Add(tl.span).Format("15:04"))
		})
		time.Sleep(3 * time.Second) // the playback position must not pull the range back
		fyne.DoAndWait(func() {
			tl := u.hist.tl
			fmt.Printf("after 3 s: tl %s..%s\n", tl.from().Format("15:04"), tl.from().Add(tl.span).Format("15:04"))
		})
		shot("tl_dragged.jpg")
		fyne.Do(func() {
			u.hist.tl.Tapped(&fyne.PointEvent{Position: fyne.NewPos(u.hist.tl.xAt(night.Add(30*time.Second)), 20)})
		})
		time.Sleep(15 * time.Second)
		fyne.DoAndWait(func() {
			fmt.Printf("picked 2:00: position %s, status %q\n", u.historyPosition().Format("15:04:05"), u.status.Text)
		})
		shot("tl_night.jpg")
		fyne.Do(u.goLive)
		time.Sleep(4 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
