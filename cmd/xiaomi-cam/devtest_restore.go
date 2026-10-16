//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_RESTORETEST=<cycles>: minimize to tray and show again (like the tray "Show" or a second
// start of the app), checking after each show that the UI goroutine answers.
func init() {
	if os.Getenv("XC_RESTORETEST") == "" {
		return
	}
	devHook = func(u *ui) {
		fyne.DoAndWait(func() {
			u.win.Canvas().SetOnTypedRune(nil)
			u.win.Canvas().SetOnTypedKey(nil)
		})
		time.Sleep(6 * time.Second)
		var n int
		fmt.Sscan(os.Getenv("XC_RESTORETEST"), &n)
		for i := 0; i < n; i++ {
			fyne.Do(func() { glfwWindow(u.win).Iconify() })
			time.Sleep(2500 * time.Millisecond)
			fyne.DoAndWait(func() { fmt.Printf("cycle %d: minimized, visible %v\n", i, u.visible) })
			t0 := time.Now()
			fyne.Do(u.show)
			done := make(chan struct{})
			go fyne.Do(func() { close(done) })
			select {
			case <-done:
				fmt.Printf("cycle %d: shown, UI answers after %v\n", i, time.Since(t0).Round(time.Millisecond))
			case <-time.After(8 * time.Second):
				fmt.Printf("cycle %d: UI HUNG\n", i)
				os.Exit(3)
			}
			time.Sleep(3 * time.Second)
		}
		fyne.Do(u.app.Quit)
	}
}
