//go:build devtest

package main

import (
	"fmt"
	"image"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// XC_SEAMLESSTEST=1: sound on/off and video-only (decode size change) must not reconnect
// to the camera: one producer session, short gaps between frames.
func init() {
	if os.Getenv("XC_SEAMLESSTEST") == "" {
		return
	}
	devHook = func(u *ui) {
		var mu sync.Mutex
		var last time.Time
		var maxGap time.Duration
		var gapAt string
		phase := "start"
		orig := u.player.onFrame
		u.player.onFrame = func(img *image.RGBA, gen int64) {
			mu.Lock()
			if !last.IsZero() && time.Since(last) > maxGap {
				maxGap, gapAt = time.Since(last), phase
			}
			last = time.Now()
			mu.Unlock()
			orig(img, gen)
		}
		sessions := map[any]bool{}
		watch := func() {
			if s := streams.Get(u.camera.Selected); s != nil {
				for _, c := range s.Conns() {
					sessions[c] = true
				}
			}
		}
		step := func(name string, f func()) {
			mu.Lock()
			phase = name
			mu.Unlock()
			fyne.Do(f)
			for i := 0; i < 8; i++ {
				time.Sleep(time.Second)
				watch()
			}
		}
		time.Sleep(12 * time.Second)
		watch()
		mu.Lock()
		maxGap = 0
		mu.Unlock()
		step("sound on", func() { u.sound.SetChecked(true) })
		step("sound off", func() { u.sound.SetChecked(false) })
		step("video-only", func() { u.setViewOnly(true) })
		step("back", func() { u.setViewOnly(false) })
		mu.Lock()
		fmt.Printf("camera sessions: %d, longest gap between frames: %v (during %s), status: %s\n",
			len(sessions), maxGap.Round(10*time.Millisecond), gapAt, u.status.Text)
		mu.Unlock()
		fyne.Do(u.app.Quit)
	}
}
