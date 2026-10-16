//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_HANGTEST=1: blocks the UI goroutine for 8 s; the watchdog must write a hang dump.
func init() {
	if os.Getenv("XC_HANGTEST") == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(3 * time.Second)
		fmt.Println("blocking the UI")
		fyne.Do(func() { time.Sleep(8 * time.Second) })
		time.Sleep(10 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
