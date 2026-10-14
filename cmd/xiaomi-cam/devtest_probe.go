//go:build devtest

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
)

// XC_PROBETEST=1: find SD card file starts by probing PLAYBACK_REQ second by second.
func init() {
	if os.Getenv("XC_PROBETEST") == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(12 * time.Second)
		name := u.camera.Selected
		from := time.Now().Add(-15 * time.Minute).Truncate(time.Minute)
		t0 := time.Now()
		n := 0
		for t := from; t.Before(from.Add(4 * time.Minute)); t = t.Add(time.Second) {
			ts := t.Unix()
			p := fmt.Sprintf(`{"sessionid":%d,"starttime":%d,"endtime":0,"autoswitchtolive":0,"offset":0,"speed":1,"avchannelmerge":1,"channel":0}`, ts, ts)
			res, err := xiaomi.Request(name, 0x10D, p, 0x10E, 2*time.Second)
			n++
			if err != nil {
				fmt.Println(t.Format("15:04:05"), "error:", err)
				continue
			}
			if !strings.Contains(string(res), "filenotfound") {
				fmt.Println(t.Format("15:04:05"), "FOUND:", strings.TrimRight(string(res), "\x00"))
			}
		}
		fmt.Printf("%d probes in %v\n", n, time.Since(t0))
		fyne.Do(u.app.Quit)
	}
}
