package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// waitTip hovers pos and waits until the tooltip is shown.
func waitTip(t *testing.T, c fyne.Canvas, pos fyne.Position) {
	t.Helper()
	test.MoveMouse(c, pos)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := c.Overlays().Top().(*tipOverlay); ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tooltip not shown")
}

// A click while the tooltip is shown must reach the button (it needed a second click).
func TestTooltipPassesClick(t *testing.T) {
	test.NewTempApp(t)
	taps := 0
	b := newButton("Settings", nil, func() { taps++ }).withTip("tip")
	w := test.NewWindow(container.NewWithoutLayout(b))
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	b.Resize(fyne.NewSize(100, 40))
	b.Move(fyne.NewPos(50, 50))
	c := w.Canvas()
	pos := fyne.NewPos(100, 70)

	waitTip(t, c, pos)
	test.TapCanvas(c, pos) // the overlay is on top: it gets the tap
	if taps != 1 {
		t.Fatalf("taps = %d, want 1", taps)
	}
	time.Sleep(100 * time.Millisecond) // fyne.Do(remove)
	if c.Overlays().Top() != nil {
		t.Fatal("tooltip still shown after the click")
	}
	// the tooltip doesn't come back while the mouse stays on the button
	test.MoveMouse(c, pos.AddXY(1, 0))
	time.Sleep(tipDelay + 200*time.Millisecond)
	if c.Overlays().Top() != nil {
		t.Fatal("tooltip shown again without leaving the button")
	}
}

// Press and release on the overlay reach a hold button (pan/tilt).
func TestTooltipPassesPress(t *testing.T) {
	test.NewTempApp(t)
	pressed, released := 0, 0
	b := newHoldButton(nil, func() { pressed++ }, func() { released++ }).withTip("tip")
	w := test.NewWindow(container.NewWithoutLayout(b))
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	b.Resize(fyne.NewSize(40, 40))
	b.Move(fyne.NewPos(50, 50))
	c := w.Canvas()
	pos := fyne.NewPos(70, 70)

	waitTip(t, c, pos)
	o := c.Overlays().Top().(*tipOverlay)
	ev := &desktop.MouseEvent{Button: desktop.MouseButtonPrimary}
	ev.AbsolutePosition = pos
	o.MouseDown(ev)
	if pressed != 1 {
		t.Fatalf("pressed = %d, want 1", pressed)
	}
	if c.Overlays().Top() != o {
		t.Fatal("overlay must stay until the click ends")
	}
	o.MouseUp(ev)
	if released != 1 {
		t.Fatalf("released = %d, want 1", released)
	}
}

// Moving the mouse away hides the tooltip.
func TestTooltipHidesOnMove(t *testing.T) {
	test.NewTempApp(t)
	b := newButton("Settings", nil, func() {}).withTip("tip")
	w := test.NewWindow(container.NewWithoutLayout(b))
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	b.Resize(fyne.NewSize(100, 40))
	b.Move(fyne.NewPos(50, 50))
	c := w.Canvas()

	waitTip(t, c, fyne.NewPos(100, 70))
	test.MoveMouse(c, fyne.NewPos(300, 250))
	time.Sleep(100 * time.Millisecond)
	if c.Overlays().Top() != nil {
		t.Fatal("tooltip still shown after moving away")
	}
}
