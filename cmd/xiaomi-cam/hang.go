package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
)

// The hang watchdog: the UI goroutine is pinged every 2 s; when it doesn't answer for
// hangAfter, the stacks of all goroutines are written to ~/.cache/xiaomi-cam/hang-*.txt
// (once per hang), so a "not responding" window can be traced afterwards.

const hangAfter = 5 * time.Second

func startHangWatchdog() {
	var alive atomic.Int64
	alive.Store(time.Now().UnixNano())
	var pinging atomic.Bool
	go func() {
		dumped := false
		for range time.Tick(2 * time.Second) {
			if appStopped.Load() {
				return
			}
			// fyne.Do may block while the UI is stuck: one ping at a time, in its own goroutine
			if pinging.CompareAndSwap(false, true) {
				go fyne.Do(func() {
					alive.Store(time.Now().UnixNano())
					pinging.Store(false)
				})
			}
			stall := time.Since(time.Unix(0, alive.Load()))
			if stall < hangAfter {
				dumped = false
				continue
			}
			if !dumped {
				dumped = true
				writeHangDump(stall)
			}
		}
	}()
}

func writeHangDump(stall time.Duration) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "xiaomi-cam")
	_ = os.MkdirAll(dir, 0o700)
	if old, _ := filepath.Glob(filepath.Join(dir, "hang-*.txt")); len(old) >= 10 {
		_ = os.Remove(old[0]) // keep the last 10
	}
	name := filepath.Join(dir, "hang-"+time.Now().Format("20060102-150405")+".txt")
	f, err := os.Create(name)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "Xiaomi Cam %s: the UI did not answer for %v\n\n", version, stall.Round(time.Second))
	_ = pprof.Lookup("goroutine").WriteTo(f, 2)
	fmt.Fprintf(os.Stderr, "UI not responding for %v, stacks in %s\n", stall.Round(time.Second), name)
}
