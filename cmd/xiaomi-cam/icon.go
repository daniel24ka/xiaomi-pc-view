package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

// appIcon - the app icon (packaging/xiaomi-cam.svg rendered to 256 px PNG): window, tray,
// notifications.
//
//go:embed icon.png
var iconPNG []byte

var appIcon = fyne.NewStaticResource("xiaomi-cam.png", iconPNG)

// cameraIcon - "manage cameras" button (theme colored)
var cameraIcon = lucide("cctv")

// recordIcon - red dot "record" button
var recordIcon = fyne.NewStaticResource("record.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="6" fill="#EF4444"/></svg>`))

var moonIcon = lucide("moon")

// lightIcon - a light bulb (the spotlight / siren / night mode panel)
var lightIcon = lucide("lightbulb")
