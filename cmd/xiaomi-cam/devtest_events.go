//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// XC_EVENTTEST=<dir>: camera events: history with the events of the SD card (timeline, the
// preview of an event, a jump to the previous event), the detection settings, the events
// list, Settings → Events, a short session without video and the watcher with its actions
// (the notification is printed, the config of the user is not changed).
func init() {
	dir := os.Getenv("XC_EVENTTEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		histDebugf = func(format string, args ...any) {
			fmt.Printf("%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
		}
		shot := func(name string) {
			time.Sleep(900 * time.Millisecond)
			fyne.DoAndWait(func() {
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, u.win.Canvas().Capture(), &jpeg.Options{Quality: 90})
					_ = f.Close()
				}
			})
		}
		closeTop := func() {
			fyne.DoAndWait(func() {
				if top := u.win.Canvas().Overlays().Top(); top != nil {
					u.win.Canvas().Overlays().Remove(top)
				}
			})
		}
		const name = "Dom przód"
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
			u.setViewOnly(false)
			u.setGrid(false)
			u.camera.SetSelected(name)
			u.win.Resize(fyne.NewSize(1280, 800))
		})
		did := cameraDid(cameraSource(name))
		fmt.Println("event kinds of the camera:", cameraEvents(name))
		time.Sleep(8 * time.Second)

		// history: the files and the events of all days
		t0 := time.Now()
		fyne.Do(u.openHistory)
		for time.Since(t0) < 40*time.Second {
			time.Sleep(time.Second)
			if evStore.complete(did, dayStart(time.Now()).AddDate(0, 0, -1)) {
				break
			}
		}
		time.Sleep(2 * time.Second)
		all := evStore.between(did, time.Now().AddDate(0, 0, -31), time.Now().Add(time.Hour))
		byKind := map[eventKind]int{}
		for _, e := range all {
			byKind[e.Kind]++
		}
		fmt.Printf("events after %v: %d %v\n", time.Since(t0).Round(time.Millisecond), len(all), byKind)
		for _, e := range all[max(len(all)-5, 0):] {
			fmt.Println("  event", e.Time.Format("2006-01-02 15:04:05"), e.Kind)
		}
		shot("ev_history.jpg")
		if len(all) > 0 {
			last := all[len(all)-1]
			fyne.DoAndWait(func() {
				tl := u.hist.tl
				tl.onHover = nil // a real mouse over the test window doesn't move it
				tl.center, tl.span, tl.browsed = last.Time, time.Hour, true
				t := last.Time.Add(20 * time.Second)
				tl.hover = t
				tl.Refresh()
				u.showHistoryPreview(t, tl.xAt(t))
				fmt.Printf("preview of the last event: %q\n", u.histPreview.label.Text)
			})
			shot("ev_hover.jpg")
			fyne.DoAndWait(func() { u.showHistoryPreview(time.Time{}, 0) })
		}
		// jump to the previous event from the playback position
		fyne.DoAndWait(func() { fmt.Println("position before the jump:", u.historyPosition().Format("15:04:05")) })
		fyne.Do(func() { u.jumpEvent(-1) })
		time.Sleep(12 * time.Second)
		fyne.DoAndWait(func() {
			fmt.Printf("after the jump: target %s, position %s, status %q\n", u.hist.target.Format("15:04:05"),
				u.historyPosition().Format("15:04:05"), u.status.Text)
		})
		shot("ev_jump.jpg")
		fyne.Do(u.goLive)
		time.Sleep(4 * time.Second)

		// the dialogs
		fyne.Do(u.showDetection)
		time.Sleep(4 * time.Second)
		shot("ev_detection.jpg")
		closeTop()
		fyne.Do(u.showEvents)
		shot("ev_list.jpg")
		closeTop()
		fyne.DoAndWait(func() {
			d := dialog.NewCustom(T("Ustawienia"), T("Zamknij"), u.eventsSettings(), u.win)
			d.Resize(fyne.NewSize(860, 640))
			d.Show()
		})
		shot("ev_settings.jpg")
		closeTop()

		// a short session without video (the window shows no video)
		fyne.DoAndWait(func() { u.player.stop() })
		time.Sleep(3 * time.Second)
		t0 = time.Now()
		list, err := readDayEvents(name, time.Now())
		fmt.Printf("short session: %d events today in %v, err %v\n", len(list), time.Since(t0).Round(time.Millisecond), err)

		// the watcher: the first check marks the events as seen; then the last event is made
		// new again and its action runs
		notified := 0
		u.ai.actions.notify = func(title, text string) {
			notified++
			fmt.Printf("notification: %s: %s\n", title, text)
		}
		cfg := eventsConfig{Watch: true, IntervalMin: 1, Kinds: xccore.AllEventKinds, Notify: true}
		saved := evStore.dids[did].Seen
		evStore.dids[did].Seen = 0
		u.checkEvents(name, cfg)
		fmt.Println("first check: notifications", notified, "seen", time.Unix(evStore.dids[did].Seen, 0).Format("15:04:05"))
		if len(list) > 0 {
			evStore.dids[did].Seen = list[len(list)-1].Time.Unix() - 1
			u.checkEvents(name, cfg)
		}
		fmt.Println("second check: notifications", notified)
		evStore.dids[did].Seen = max(saved, evStore.dids[did].Seen)
		watch.mu.Lock()
		fmt.Printf("watch state: checked %v, errs %v\n", watch.checked, watch.errs)
		watch.mu.Unlock()
		fyne.Do(u.app.Quit)
	}
}
