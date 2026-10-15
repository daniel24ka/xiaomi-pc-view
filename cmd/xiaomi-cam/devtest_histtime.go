//go:build devtest

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

// XC_HISTTIME=1: times of the SD card history: open, then steps back by 1 min; the playback
// search and dial are logged.
func init() {
	if os.Getenv("XC_HISTTIME") == "" {
		return
	}
	t0 := time.Now()
	miss.Debugf = func(format string, args ...any) {
		fmt.Printf("  %6.2fs miss: %s\n", time.Since(t0).Seconds(), fmt.Sprintf(format, args...))
	}
	histDebugf = func(format string, args ...any) { miss.Debugf("app: "+format, args...) }
	devHook = func(u *ui) {
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setGrid(false)
			u.camera.SetSelected("Dom przód")
		})
		time.Sleep(12 * time.Second)
		wait := func(tag string, start time.Time) {
			for time.Since(start) < 60*time.Second {
				var got bool
				fyne.DoAndWait(func() { got = u.gotFrame })
				if got {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			fyne.DoAndWait(func() {
				fmt.Printf("%s: %.1fs to the picture, at %s, status %q\n", tag, time.Since(start).Seconds(),
					u.historyTime().Format("15:04:05"), u.status.Text)
			})
		}
		t0 = time.Now()
		fyne.Do(u.openHistory)
		wait("open", t0)
		steps := []struct {
			tag string
			d   time.Duration
		}{{"back 1", -time.Minute}, {"back 2", -time.Minute},
			{"jump -10m17s", -10*time.Minute - 17*time.Second}}
		var opened time.Time
		fyne.DoAndWait(func() { opened = u.historyTime() })
		steps = append(steps, struct {
			tag string
			d   time.Duration
		}{"back to the opened time (seen before)", 0})
		for _, st := range steps {
			time.Sleep(3 * time.Second)
			t0 = time.Now()
			fyne.Do(func() {
				if st.d == 0 {
					u.playHistory(opened.Add(20 * time.Second))
				} else {
					u.playHistory(u.historyTime().Add(st.d))
				}
			})
			wait(st.tag, t0)
		}
		// how long ffmpeg needs to the first decoded frame (the producer is running)
		measure := func(tag string) {
			for _, extra := range [][]string{
				{"-analyzeduration", "500000", "-probesize", "500000"},
				{"-analyzeduration", "500000", "-probesize", "500000", "-threads", "1"},
				{"-analyzeduration", "100000", "-probesize", "32768"},
			} {
				args := append([]string{"-hide_banner", "-loglevel", "error", "-fflags", "nobuffer", "-flags", "low_delay"}, extra...)
				args = append(args, "-rtsp_transport", "tcp", "-i", rtspURL("Dom przód"), "-map", "0:v:0", "-frames:v", "1", "-f", "null", "-")
				st := time.Now()
				out, err := exec.Command("ffmpeg", args...).CombinedOutput()
				fmt.Printf("%s ffmpeg first frame %v: %.2fs %v %s\n", tag, extra, time.Since(st).Seconds(), err, strings.TrimSpace(string(out)))
			}
		}
		measure("history")
		fyne.Do(u.goLive)
		time.Sleep(8 * time.Second)
		measure("live")
		fyne.Do(u.app.Quit)
	}
}
