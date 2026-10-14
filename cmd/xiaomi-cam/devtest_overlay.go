//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_OVERLAYTEST=1: video-only buttons hide 1 s after the cursor leaves the window.
func init() {
	if os.Getenv("XC_OVERLAYTEST") == "" {
		return
	}
	devHook = func(u *ui) {
		do := func(f func()) {
			done := make(chan struct{})
			fyne.Do(func() { f(); close(done) })
			<-done
		}
		time.Sleep(5 * time.Second)
		do(func() { u.setViewOnly(true) })
		do(func() { u.hideOverlayAfter(time.Second) }) // what the cursor-leave callback does
		time.Sleep(500 * time.Millisecond)
		do(func() { fmt.Println("after 0.5 s visible:", u.exitView.Visible()) })
		time.Sleep(800 * time.Millisecond)
		do(func() { fmt.Println("after 1.3 s visible:", u.exitView.Visible()) })
		do(func() { u.setViewOnly(false) })
		fyne.Do(u.app.Quit)
	}
}
