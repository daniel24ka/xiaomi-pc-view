//go:build heshots

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_FREEZETEST=1: switches the quality twice while the camera is still being dialled (a
// camera that is slow to answer) and measures how long the UI takes to answer meanwhile.
// It used to freeze for the length of the dial.
func init() {
	if os.Getenv("XC_FREEZETEST") == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(3 * time.Second)
		var worst time.Duration
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				t := time.Now()
				fyne.DoAndWait(func() {})
				if d := time.Since(t); d > worst {
					worst = d
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
		for _, i := range []int{1, 0, 1, 0} {
			i := i
			t := time.Now()
			fyne.DoAndWait(func() { u.quality.SetSelectedIndex(i) })
			fmt.Printf("quality -> %d: the click returned in %v\n", i, time.Since(t).Round(time.Millisecond))
			time.Sleep(1500 * time.Millisecond)
		}
		time.Sleep(4 * time.Second)
		close(stop)
		fmt.Printf("worst UI answer time while switching: %v\n", worst.Round(time.Millisecond))
		fyne.Do(u.app.Quit)
	}
}
