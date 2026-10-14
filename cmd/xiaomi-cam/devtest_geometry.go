//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_GEOMTEST=1: set framed/frameless geometry, switch modes, check they are restored.
// XC_GEOMTEST=2: only print the geometry after start (run after XC_GEOMTEST=1).
func init() {
	mode := os.Getenv("XC_GEOMTEST")
	if mode == "" {
		return
	}
	devHook = func(u *ui) {
		do := func(f func()) {
			done := make(chan struct{})
			fyne.Do(func() { f(); close(done) })
			<-done
		}
		print := func(label string) {
			do(func() {
				x, y := glfwWindow(u.win).GetPos()
				w, h := glfwWindow(u.win).GetSize()
				fmt.Printf("%s: pos %d,%d size %dx%d\n", label, x, y, w, h)
			})
		}
		time.Sleep(6 * time.Second)
		if mode == "2" {
			print("after restart (framed)")
			do(func() { u.setViewOnly(true) })
			time.Sleep(time.Second)
			print("after restart (frameless)")
			do(func() { u.setViewOnly(false) })
			time.Sleep(time.Second)
			do(u.app.Quit)
			return
		}
		do(func() { glfwWindow(u.win).SetSize(1300, 760); glfwWindow(u.win).SetPos(200, 150) })
		time.Sleep(2500 * time.Millisecond)
		print("framed set")
		do(func() { u.setViewOnly(true) })
		time.Sleep(time.Second)
		do(func() { glfwWindow(u.win).SetSize(800, 450); glfwWindow(u.win).SetPos(600, 400) })
		time.Sleep(2500 * time.Millisecond)
		print("frameless set")
		do(func() { u.setViewOnly(false) })
		time.Sleep(time.Second)
		print("framed restored")
		do(func() { u.setViewOnly(true) })
		time.Sleep(time.Second)
		print("frameless restored")
		do(func() { u.setViewOnly(false) })
		time.Sleep(2500 * time.Millisecond)
		do(u.app.Quit)
	}
}
