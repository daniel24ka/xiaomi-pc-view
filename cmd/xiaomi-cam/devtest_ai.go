//go:build devtest

package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

// XC_AITEST=1: run AI task "test-car" and a negative one, print results.
func init() {
	if os.Getenv("XC_AITEST") == "" {
		return
	}
	devHook = func(u *ui) {
		time.Sleep(8 * time.Second)
		_ = u.ai.putTask(&aiTask{ID: "test-elephant", Name: "Test: słoń", Provider: providerOpenAI,
			Prompt: "Czy na obrazie widać słonia?", Schedule: "@every 5m", Notify: true})
		for _, id := range []string{"test-car", "test-elephant"} {
			t0 := time.Now()
			u.ai.run(id)
			for _, t := range u.ai.config().Tasks {
				if t.ID == id {
					fmt.Printf("%s (%.1fs): %s\n", t.Name, time.Since(t0).Seconds(), taskStatus(t))
				}
			}
		}
		_ = u.ai.deleteTask("test-elephant")
		time.Sleep(2 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
