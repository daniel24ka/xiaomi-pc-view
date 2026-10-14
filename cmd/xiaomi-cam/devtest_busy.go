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

// XC_BUSYTEST=<dir>: screenshots of the "action in progress" spinners: connecting,
// snapshot, zoom, recording start/stop.
func init() {
	dir := os.Getenv("XC_BUSYTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				fmt.Println(name, "busy:", u.busyCount, "spinner:", u.video.spinner.running(), "snap:", u.snapButton.isBusy(), "rec:", u.recButton.isBusy(), u.recButton.Icon.Name())
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, nil)
					_ = f.Close()
				}
			})
		}
		time.Sleep(1500 * time.Millisecond)
		shot("busy_connecting.jpg")
		time.Sleep(12 * time.Second)
		shot("busy_live.jpg")
		fyne.Do(u.snapshot)
		time.Sleep(300 * time.Millisecond)
		shot("busy_snapshot.jpg")
		time.Sleep(6 * time.Second)
		shot("busy_snapshot_done.jpg")
		fyne.Do(func() { u.setZoom(2) })
		time.Sleep(100 * time.Millisecond)
		shot("busy_zoom.jpg")
		time.Sleep(3 * time.Second)
		fyne.Do(func() { u.setZoom(1) })
		time.Sleep(3 * time.Second)
		fyne.Do(u.toggleRecording)
		time.Sleep(50 * time.Millisecond)
		shot("busy_rec_start.jpg")
		time.Sleep(4 * time.Second)
		shot("busy_recording.jpg")
		fyne.Do(u.toggleRecording)
		time.Sleep(50 * time.Millisecond)
		shot("busy_rec_stop.jpg")
		time.Sleep(4 * time.Second)
		shot("busy_end.jpg")
		fyne.Do(u.app.Quit)
	}
}
