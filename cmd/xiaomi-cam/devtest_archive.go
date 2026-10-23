//go:build heshots

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
)

// XC_ARCHTEST=<dir>: the history bar with the PC archive (cam-archive running): opens it,
// plays, 8x, pause, the thumbnail preview, the previous event, saving a clip and a frame,
// back to live; prints what happened and saves window shots.
func init() {
	dir := os.Getenv("XC_ARCHTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.DoAndWait(func() {
				if f, err := os.Create(filepath.Join(dir, name+".png")); err == nil {
					_ = png.Encode(f, u.win.Canvas().Capture())
					_ = f.Close()
				}
			})
		}
		state := func(what string) {
			fyne.DoAndWait(func() {
				h := &u.hist
				a := &h.arch
				fmt.Printf("%-10s pc=%v source=%v tier=%s pos=%s paused=%v status=%q\n", what, h.pc, h.source.Visible(),
					a.tier, u.historyPosition().Format("15:04:05"), a.paused, u.status.Text)
				fmt.Printf("           frames=%d gen=%d oldGen=%d clip=%s-%s from=%s\n", a.frames, u.player.gen.Load(), a.oldGen,
					a.clipStart.Format("15:04:05"), a.clipEnd.Format("15:04:05"), a.from.Format("15:04:05"))
			})
		}
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setGrid(false)
			u.win.Resize(fyne.NewSize(1280, 760))
		})
		on := appPrefs.Bool(prefArchive) // the archive is off by default
		appPrefs.SetBool(prefArchive, true)
		time.Sleep(8 * time.Second)
		fyne.DoAndWait(u.openHistory)
		time.Sleep(7 * time.Second)
		state("open")
		shot("arch-open")

		fyne.DoAndWait(func() { u.hist.speed.SetSelectedIndex(3) }) // 8x
		time.Sleep(5 * time.Second)
		var p1, p2 time.Time
		fyne.DoAndWait(func() { p1 = u.historyPosition() })
		time.Sleep(2 * time.Second)
		fyne.DoAndWait(func() { p2 = u.historyPosition() })
		fmt.Printf("8x: %.1f s of recording in 2 s\n", p2.Sub(p1).Seconds())
		state("8x")

		fyne.DoAndWait(u.toggleArchivePause)
		fyne.DoAndWait(func() { p1 = u.historyPosition() })
		time.Sleep(2 * time.Second)
		fyne.DoAndWait(func() { p2 = u.historyPosition() })
		fmt.Printf("paused: moved %.1f s\n", p2.Sub(p1).Seconds())
		state("pause")
		shot("arch-paused")

		hover := time.Now().Add(-25 * time.Minute)
		fyne.DoAndWait(func() {
			u.hist.tl.center, u.hist.tl.span = time.Now().Add(-30*time.Minute), 2*time.Hour
			u.hist.tl.hover = hover
			u.hist.tl.Refresh()
			u.showHistoryPreview(hover, u.hist.tl.xAt(hover))
		})
		time.Sleep(4 * time.Second)
		fyne.DoAndWait(func() {
			fmt.Printf("preview: thumb=%v label=%q\n", u.histPreview.img.Visible(), u.histPreview.label.Text)
		})
		shot("arch-preview")
		fyne.DoAndWait(func() { u.showHistoryPreview(time.Time{}, 0) })

		fyne.DoAndWait(func() { u.hist.speed.SetSelectedIndex(0) })
		fyne.DoAndWait(func() { u.jumpEvent(-1) })
		time.Sleep(6 * time.Second)
		state("event")
		shot("arch-event")

		var pos time.Time
		fyne.DoAndWait(func() { pos = u.historyPosition() })
		fyne.DoAndWait(func() { u.saveArchiveClip(pos) })
		for i := 0; i < 60; i++ {
			time.Sleep(time.Second)
			var st string
			fyne.DoAndWait(func() { st = u.status.Text })
			if st != "" && st != T("Zapisuję nagranie…") {
				break
			}
		}
		state("saved")
		fyne.DoAndWait(u.archiveSnapshot)
		time.Sleep(2 * time.Second)
		state("photo")

		fyne.DoAndWait(u.goLive)
		time.Sleep(6 * time.Second)
		fyne.DoAndWait(func() { fmt.Printf("live: hist=%v status=%q\n", u.hist.on, u.status.Text) })
		appPrefs.SetBool(prefArchive, on)
		fyne.Do(u.app.Quit)
	}
}
