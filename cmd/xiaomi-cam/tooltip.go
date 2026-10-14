package main

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Tooltips: Fyne has none, so the app's widgets (ptrButton, holdButton, ptrCheck...) show
// a small popup with a description when the mouse rests on them.

const tipDelay = 600 * time.Millisecond

var tips struct {
	sync.Mutex
	timer *time.Timer
	popup *widget.PopUp
	owner fyne.CanvasObject
}

// tipIn schedules the tooltip of obj at the mouse position.
func tipIn(obj fyne.CanvasObject, text string, e *desktop.MouseEvent) {
	if text == "" {
		return
	}
	pos := e.AbsolutePosition
	tips.Lock()
	defer tips.Unlock()
	if tips.timer != nil {
		tips.timer.Stop()
	}
	tips.owner = obj
	tips.timer = time.AfterFunc(tipDelay, func() {
		fyne.Do(func() {
			tips.Lock()
			defer tips.Unlock()
			if tips.owner != obj {
				return
			}
			c := fyne.CurrentApp().Driver().CanvasForObject(obj)
			if c == nil {
				return
			}
			if tips.popup != nil {
				tips.popup.Hide()
			}
			label := widget.NewLabel(text)
			bg := canvas.NewRectangle(color.NRGBA{R: 0x20, G: 0x22, B: 0x28, A: 0xf0})
			bg.StrokeColor = color.NRGBA{R: 0x60, G: 0x60, B: 0x68, A: 0xff}
			bg.StrokeWidth = 1
			bg.CornerRadius = 4
			tips.popup = widget.NewPopUp(container.NewStack(bg, label), c)
			// below the cursor, kept inside the window
			size := tips.popup.MinSize()
			p := pos.Add(fyne.NewPos(8, 20))
			if cs := c.Size(); p.X+size.Width > cs.Width {
				p.X = max(cs.Width-size.Width, 0)
			}
			if cs := c.Size(); p.Y+size.Height > cs.Height {
				p.Y = max(pos.Y-size.Height-8, 0)
			}
			tips.popup.ShowAtPosition(p)
		})
	})
}

// tipOut hides the tooltip.
func tipOut() {
	tips.Lock()
	defer tips.Unlock()
	if tips.timer != nil {
		tips.timer.Stop()
		tips.timer = nil
	}
	tips.owner = nil
	if tips.popup != nil {
		p := tips.popup
		tips.popup = nil
		fyne.Do(p.Hide)
	}
}

// --- hover handling of the app widgets

func (b *ptrButton) MouseIn(e *desktop.MouseEvent) {
	b.Button.MouseIn(e)
	tipIn(b, b.tip, e)
}
func (b *ptrButton) MouseMoved(e *desktop.MouseEvent) { b.Button.MouseMoved(e) }
func (b *ptrButton) MouseOut() {
	b.Button.MouseOut()
	tipOut()
}
func (b *ptrButton) withTip(s string) *ptrButton { b.tip = s; return b }

func (b *holdButton) MouseIn(e *desktop.MouseEvent) {
	b.Button.MouseIn(e)
	tipIn(b, b.tip, e)
}
func (b *holdButton) MouseMoved(e *desktop.MouseEvent) { b.Button.MouseMoved(e) }
func (b *holdButton) withTip(s string) *holdButton     { b.tip = s; return b }

func (h *moveHandle) MouseIn(e *desktop.MouseEvent) {
	h.Button.MouseIn(e)
	tipIn(h, h.tip, e)
}
func (h *moveHandle) MouseMoved(e *desktop.MouseEvent) { h.Button.MouseMoved(e) }
func (h *moveHandle) MouseOut() {
	h.Button.MouseOut()
	tipOut()
}

func (c *ptrCheck) MouseIn(e *desktop.MouseEvent) {
	c.Check.MouseIn(e)
	tipIn(c, c.tip, e)
}
func (c *ptrCheck) MouseMoved(e *desktop.MouseEvent) { c.Check.MouseMoved(e) }
func (c *ptrCheck) MouseOut() {
	c.Check.MouseOut()
	tipOut()
}
func (c *ptrCheck) withTip(s string) *ptrCheck { c.tip = s; return c }

func (s *ptrSelect) MouseIn(e *desktop.MouseEvent) {
	s.Select.MouseIn(e)
	tipIn(s, s.tip, e)
}
func (s *ptrSelect) MouseMoved(e *desktop.MouseEvent) { s.Select.MouseMoved(e) }
func (s *ptrSelect) MouseOut() {
	s.Select.MouseOut()
	tipOut()
}
func (s *ptrSelect) withTip(t string) *ptrSelect { s.tip = t; return s }

func (s *ptrSlider) MouseIn(e *desktop.MouseEvent) {
	s.Slider.MouseIn(e)
	tipIn(s, s.tip, e)
}
func (s *ptrSlider) MouseMoved(e *desktop.MouseEvent) { s.Slider.MouseMoved(e) }
func (s *ptrSlider) MouseOut() {
	s.Slider.MouseOut()
	tipOut()
}
func (s *ptrSlider) withTip(t string) *ptrSlider { s.tip = t; return s }

func (z *zoomSlider) MouseIn(e *desktop.MouseEvent)    { tipIn(z, z.tip, e) }
func (z *zoomSlider) MouseMoved(e *desktop.MouseEvent) {}
func (z *zoomSlider) MouseOut()                        { tipOut() }
