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
		car := &aiTask{ID: "test-car-rec", Name: "Test: samochód + nagranie", Provider: providerOpenAI,
			Prompt: "Czy na obrazie widać zaparkowany samochód?", Schedule: "@every 5m",
			FullRes: true, Record: true, RecordSec: 8}
		_ = u.ai.putTask(car)
		t0 := time.Now()
		u.ai.run(car.ID)
		for _, t := range u.ai.config().Tasks {
			if t.ID == car.ID {
				fmt.Printf("%s (%.1fs): %s\n", t.Name, time.Since(t0).Seconds(), taskStatus(t))
			}
		}
		if r := currentRecording(taskCamera(car)); r != nil {
			fmt.Println("recording:", r.path)
			<-r.done
			fmt.Println("recording finished after", time.Since(r.started).Round(time.Second))
		} else {
			fmt.Println("no recording")
		}
		_ = u.ai.deleteTask(car.ID)
		time.Sleep(2 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
