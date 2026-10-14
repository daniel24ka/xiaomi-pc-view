//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// XC_RECMODETEST=1: record 6 s in "each" and "grid" modes with a temporary relay camera.
func init() {
	if os.Getenv("XC_RECMODETEST") == "" {
		return
	}
	devHook = func(u *ui) {
		cam := u.camera.Selected
		time.Sleep(8 * time.Second)
		_, _ = streams.New("Test relay", rtspURL(cam))
		for _, mode := range []string{recordEach, recordGrid} {
			recs, err := startRecordings(mode, cam, 6*time.Second)
			fmt.Println("mode", mode, "started", len(recs), "err", err)
			time.Sleep(4 * time.Second)
			fmt.Println("active:", len(activeRecordings()))
			for _, r := range recs {
				<-r.done
				fmt.Println("FILE", r.path)
			}
		}
		streams.Delete("Test relay")
		fyne.Do(u.app.Quit)
	}
}
