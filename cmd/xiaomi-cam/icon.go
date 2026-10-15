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

var moonIcon = theme.NewThemedResource(fyne.NewStaticResource("moon.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M12.3 2a10 10 0 1 0 9.7 12.4A8 8 0 0 1 12.3 2z"/></svg>`)))

// lightIcon - a light bulb (the spotlight / siren / night mode panel)
var lightIcon = theme.NewThemedResource(fyne.NewStaticResource("light.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M9 21h6v-1.5H9V21zm3-19a7 7 0 0 0-4 12.74V17a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1v-2.26A7 7 0 0 0 12 2zm2.85 11.1-.85.6V16h-4v-2.3l-.85-.6a5 5 0 1 1 5.7 0z"/></svg>`)))
