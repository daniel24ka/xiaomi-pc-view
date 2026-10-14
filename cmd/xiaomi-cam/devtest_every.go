//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_EVERYTEST=1: AI task every 10 s with the window hidden in the tray, for 50 s.
func init() {
	if os.Getenv("XC_EVERYTEST") == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(5 * time.Second)
		fyne.Do(u.hide)
		start := time.Now()
		var last time.Time
		u.ai.onChange = func() {
			for _, t := range u.ai.config().Tasks {
				if t.ID == "test-every" && t.LastRun != last {
					last = t.LastRun
					fmt.Printf("+%4.1fs %s\n", time.Since(start).Seconds(), taskStatus(t))
				}
			}
		}
		_ = u.ai.putTask(&aiTask{ID: "test-every", Name: "Test co 10 s", Enabled: true, Provider: providerOpenAI,
			Prompt: "Czy na obrazie widać zaparkowany samochód?", Schedule: "@every 10s"})
		time.Sleep(50 * time.Second)
		_ = u.ai.deleteTask("test-every")
		fmt.Println("session open after test:", hasSession(cameraNames()[0]))
		fyne.Do(u.app.Quit)
	}
}
