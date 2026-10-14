//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// XC_PBTEST=<dir>: play SD card recording from 10 minutes ago, save a frame.
func init() {
	dir := os.Getenv("XC_PBTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		save := func(name string) {
			fyne.Do(func() {
				if f := u.video.frame; f != nil {
					fmt.Println(time.Now().Format("15:04:05"), "saved", name)
					if out, err := os.Create(filepath.Join(dir, name)); err == nil {
						_ = jpeg.Encode(out, f, nil)
						_ = out.Close()
					}
				}
			})
		}
		time.Sleep(12 * time.Second)
		save("live.jpg")
		name := u.camera.Selected
		src := cameraSource(name)
		now := time.Now()
		for i, start := range []time.Time{
			now.Add(-20 * time.Minute),
		} {
			u.player.stop()
			time.Sleep(time.Second)
			fyne.Do(func() { u.video.frame = nil })
			streams.Get(name).SetSource(src + "&playback=" + strconv.FormatInt(start.Unix(), 10))
			fyne.Do(u.restart)
			time.Sleep(25 * time.Second)
			got := false
			fyne.Do(func() { got = u.video.frame != nil })
			time.Sleep(100 * time.Millisecond)
			fmt.Println("playback", start.Format("15:04:05"), "frame:", got)
			save(fmt.Sprintf("pb%d.jpg", i))
		}
		u.player.stop()
		streams.Get(name).SetSource(src)
		fyne.Do(u.app.Quit)
	}
}
