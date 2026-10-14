package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// appIcon - the app icon (packaging/xiaomi-cam.svg rendered to 256 px PNG): window, tray,
// notifications.
//
//go:embed icon.png
var iconPNG []byte

var appIcon = fyne.NewStaticResource("xiaomi-cam.png", iconPNG)

// cameraIcon - "manage cameras" button (theme colored)
var cameraIcon = theme.NewThemedResource(fyne.NewStaticResource("cameras.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z"/></svg>`)))

// recordIcon - red dot "record" button
var recordIcon = fyne.NewStaticResource("record.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="7" fill="#e53935"/></svg>`))
