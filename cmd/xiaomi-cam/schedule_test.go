package main

import (
	"testing"
	"time"
)

func TestScheduleSeconds(t *testing.T) {
	for _, spec := range []string{"@every 10s", "@every 30s", "@every 5m", "@hourly", "*/10 7-22 * * *"} {
		s, err := cronParser.Parse(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)
		t.Logf("%-16s next: %s", spec, s.Next(now).Format("15:04:05"))
	}
}
