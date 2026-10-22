package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Compact bars: less used controls are in a menu (top bar) and a panel (bottom bar), so the
// window can be narrow.

// appMenuButton - the ☰ menu of the top bar: view modes, cameras, event detection and the
// events (cameras with events), language, shortcuts and version.
func (u *ui) appMenuButton() *ptrButton {
	var b *ptrButton
	b = newButton("", theme.MenuIcon(), func() {
		items := []*fyne.MenuItem{
			fyne.NewMenuItemWithIcon(TM("Tylko obraz – ukrywa panele i ramkę okna (H, powrót: Esc)"), theme.VisibilityOffIcon(), func() { u.setViewOnly(true) }),
			fyne.NewMenuItemWithIcon(TM("Pełny ekran (F, F11, podwójne kliknięcie)"), theme.ViewFullScreenIcon(), u.toggleFullScreen),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItemWithIcon(TM("Kamery – dodawanie, zmiana nazwy, usuwanie"), cameraIcon, u.showCameras),
		}
		if len(cameraEvents(u.camera.Selected)) > 0 {
			items = append(items,
				fyne.NewMenuItemWithIcon(TM("Wykrywanie zdarzeń – ruch, ludzie, samochody"), lucide("scan-eye"), u.showDetection))
		}
		if hasEventCameras() {
			items = append(items, fyne.NewMenuItemWithIcon(TM("Zdarzenia – ostatnie wykrycia"), lucide("bell"), u.showEvents))
		}
		items = append(items, fyne.NewMenuItemSeparator())
		for _, l := range languages {
			code := l.code
			it := fyne.NewMenuItem(l.name, func() { u.setLanguage(code) })
			it.Checked = code == lang
			items = append(items, it)
		}
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItemWithIcon(TM("Skróty klawiaturowe i wersja"), theme.HelpIcon(), u.showShortcuts))
		showMenuBelow(b, fyne.NewMenu("", items...))
	}).withTip(T("Menu: tylko obraz, pełny ekran, kamery, zdarzenia, język, skróty klawiaturowe i wersja"))
	return b
}

// showMenuBelow opens a menu under the button, aligned to its right edge.
func showMenuBelow(b fyne.CanvasObject, m *fyne.Menu) {
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	pop := widget.NewPopUpMenu(m, c)
	w := pop.MinSize().Width
	pop.ShowAtPosition(fyne.NewPos(max(pos.X+b.Size().Width-w, 0), pos.Y+b.Size().Height))
}

// cameraControlsButton - the bottom bar button of the panel with the spotlight, siren and
// night mode (the controls the camera has, caps.go).
func (u *ui) cameraControlsButton() *ptrButton {
	var b *ptrButton
	b = newButton("", lightIcon, func() {
		c := u.win.Canvas()
		panel := container.NewVBox(
			container.NewGridWrap(fyne.NewSize(300, u.capLight.MinSize().Height), u.capLight),
			u.siren,
			u.capNight,
		)
		pop := widget.NewPopUp(container.NewPadded(panel), c)
		// above the button, kept inside the window
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
		size := pop.MinSize()
		x := min(pos.X, c.Size().Width-size.Width)
		pop.ShowAtPosition(fyne.NewPos(max(x, 0), max(pos.Y-size.Height-4, 0)))
	}).withTip(T("Reflektor, jasność, syrena i tryb nocny kamery"))
	return b
}
