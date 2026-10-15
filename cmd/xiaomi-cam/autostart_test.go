//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
)

func TestAutostart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if autostartEnabled() {
		t.Fatal("enabled at start")
	}
	if err := setAutostart(true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(autostartPath())
	if err != nil || !strings.Contains(string(b), "--tray") || !autostartEnabled() {
		t.Fatalf("%v %s", err, b)
	}
	if err = setAutostart(false); err != nil || autostartEnabled() {
		t.Fatal("not removed", err)
	}
}
