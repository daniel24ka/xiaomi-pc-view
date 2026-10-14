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
		// fake local SMTP (127.0.0.1:2525) and Slack webhook (127.0.0.1:2580), see scratch servers.py
		old := u.ai.config()
		_ = u.ai.setNotifications(emailConfig{Host: "127.0.0.1", Port: 2525, Security: "none",
			From: "kamera@test.local", To: "ja@test.local"}, "http://127.0.0.1:2580/hook")
		car := &aiTask{ID: "test-car-notify", Name: "Test: samochód (e-mail + Slack)", Provider: providerOpenAI,
			Prompt: "Czy na obrazie widać zaparkowany samochód?", Schedule: "@every 5m", Email: true, Slack: true}
		_ = u.ai.putTask(car)
		t0 := time.Now()
		u.ai.run(car.ID)
		for _, t := range u.ai.config().Tasks {
			if t.ID == car.ID {
				fmt.Printf("%s (%.1fs): %s | error: %q\n", t.Name, time.Since(t0).Seconds(), taskStatus(t), t.LastError)
			}
		}
		_ = u.ai.deleteTask(car.ID)
		_ = u.ai.setNotifications(old.Email, old.SlackWebhook)
		time.Sleep(2 * time.Second)
		fyne.Do(u.app.Quit)
	}
}
