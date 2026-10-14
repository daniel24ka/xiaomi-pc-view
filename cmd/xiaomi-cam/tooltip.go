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
// a small box with a description when the mouse rests on them.
//
// The box is a canvas overlay (to be shown over dialogs too). Fyne sends all mouse events
// to the top overlay, so a widget.PopUp would take the next click for itself (the button
// needed a second click). tipOverlay hides itself and passes the click, press and drag on to
// the tooltip owner, and hides when the mouse moves. It stays (with the box hidden) until
// the click ends, because Fyne taps the object that got both the press and the release.

const tipDelay = 600 * time.Millisecond

var tips struct {
	sync.Mutex
	timer *time.Timer
	shown *tipOverlay
	owner fyne.CanvasObject
	// the tooltip of this widget was shown since the mouse came in: not again until it leaves
	shownFor fyne.CanvasObject
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
	if tips.shownFor == obj {
		return
	}
	tips.shownFor = nil
	tips.timer = time.AfterFunc(tipDelay, func() {
		fyne.Do(func() {
			tips.Lock()
			defer tips.Unlock()
			if tips.owner != obj || tips.shown != nil {
				return
			}
			c := fyne.CurrentApp().Driver().CanvasForObject(obj)
			if c == nil {
				return
			}
			tips.shown = newTipOverlay(c, obj, text, pos)
			tips.shownFor = obj
		})
	})
}

// tipOut cancels / hides the tooltip (the mouse left the widget, or it was clicked).
func tipOut() {
	tips.Lock()
	if tips.timer != nil {
		tips.timer.Stop()
		tips.timer = nil
	}
	tips.owner = nil
	o := tips.shown
	tips.shown = nil
	if o == nil {
		tips.shownFor = nil
	}
	// else the overlay took the mouse (MouseOut of the owner): the overlay decides
	// whether the mouse really left the owner
	tips.Unlock()
	if o != nil {
		fyne.Do(o.remove) // not under the lock: remove takes it
	}
}

// tipOverlay - the tooltip box; the overlay covers the canvas and passes mouse events on
// to the tooltip owner
type tipOverlay struct {
	widget.BaseWidget
	c       fyne.Canvas
	owner   fyne.CanvasObject
	box     fyne.CanvasObject
	at      fyne.Position // mouse position when shown
	pressed bool          // a click / drag started on the overlay
	removed bool
}

func newTipOverlay(c fyne.Canvas, owner fyne.CanvasObject, text string, pos fyne.Position) *tipOverlay {
	label := widget.NewLabel(text)
	bg := canvas.NewRectangle(color.NRGBA{R: 0x20, G: 0x22, B: 0x28, A: 0xf0})
	bg.StrokeColor = color.NRGBA{R: 0x60, G: 0x60, B: 0x68, A: 0xff}
	bg.StrokeWidth = 1
	bg.CornerRadius = 4
	o := &tipOverlay{c: c, owner: owner, box: container.NewStack(bg, label), at: pos}
	o.ExtendBaseWidget(o)
	// below the cursor, kept inside the window
	size, cs := o.box.MinSize(), c.Size()
	p := pos.Add(fyne.NewPos(8, 20))
	if p.X+size.Width > cs.Width {
		p.X = max(cs.Width-size.Width, 0)
	}
	if p.Y+size.Height > cs.Height {
		p.Y = max(pos.Y-size.Height-8, 0)
	}
	o.box.Resize(size)
	o.box.Move(p)
	o.Resize(cs)
	c.Overlays().Add(o)
	return o
}

func (o *tipOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewWithoutLayout(o.box))
}

// press hides the box; the overlay stays until the click ends.
func (o *tipOverlay) press() {
	o.pressed = true
	o.box.Hide()
}

func (o *tipOverlay) remove() {
	if !o.removed {
		o.removed = true
		o.c.Overlays().Remove(o)
	}
	tips.Lock()
	if tips.shown == o {
		tips.shown = nil
	}
	tips.Unlock()
}

// target returns the owner and the event position in it, if the mouse is over the owner.
func (o *tipOverlay) target(abs fyne.Position) (fyne.CanvasObject, fyne.Position, bool) {
	p := fyne.CurrentApp().Driver().AbsolutePositionForObject(o.owner)
	rel := abs.Subtract(p)
	s := o.owner.Size()
	return o.owner, rel, rel.X >= 0 && rel.Y >= 0 && rel.X < s.Width && rel.Y < s.Height
}

// mouse moved: hide; outside the owner the tooltip may show again next time
func (o *tipOverlay) moved(abs fyne.Position) {
	if o.pressed {
		return
	}
	if _, _, over := o.target(abs); !over {
		tips.Lock()
		if tips.shownFor == o.owner {
			tips.shownFor = nil
		}
		tips.Unlock()
	} else if d := abs.Subtract(o.at); d.X*d.X+d.Y*d.Y < 4 {
		return // a tiny move: keep the tooltip
	}
	o.remove()
}

func (o *tipOverlay) MouseIn(e *desktop.MouseEvent)    { o.moved(e.AbsolutePosition) }
func (o *tipOverlay) MouseMoved(e *desktop.MouseEvent) { o.moved(e.AbsolutePosition) }
func (o *tipOverlay) MouseOut()                        {}

func (o *tipOverlay) Cursor() desktop.Cursor {
	if c, ok := o.owner.(desktop.Cursorable); ok {
		return c.Cursor()
	}
	return desktop.DefaultCursor
}

func (o *tipOverlay) mouseEvent(e *desktop.MouseEvent) (desktop.Mouseable, *desktop.MouseEvent) {
	t, rel, over := o.target(e.AbsolutePosition)
	m, ok := t.(desktop.Mouseable)
	if !over || !ok {
		return nil, nil
	}
	ev := *e
	ev.Position = rel
	return m, &ev
}

func (o *tipOverlay) MouseDown(e *desktop.MouseEvent) {
	o.press()
	if m, ev := o.mouseEvent(e); m != nil {
		m.MouseDown(ev)
	}
}

func (o *tipOverlay) MouseUp(e *desktop.MouseEvent) {
	fyne.Do(o.remove) // after Tapped, which follows in the same event
	if m, ev := o.mouseEvent(e); m != nil {
		m.MouseUp(ev)
	}
}

func (o *tipOverlay) Tapped(e *fyne.PointEvent) {
	o.remove()
	if t, rel, over := o.target(e.AbsolutePosition); over {
		if tp, ok := t.(fyne.Tappable); ok {
			tp.Tapped(&fyne.PointEvent{Position: rel, AbsolutePosition: e.AbsolutePosition})
		}
	}
}

func (o *tipOverlay) TappedSecondary(e *fyne.PointEvent) {
	o.remove()
	if t, rel, over := o.target(e.AbsolutePosition); over {
		if tp, ok := t.(fyne.SecondaryTappable); ok {
			tp.TappedSecondary(&fyne.PointEvent{Position: rel, AbsolutePosition: e.AbsolutePosition})
		}
	}
}

func (o *tipOverlay) Dragged(e *fyne.DragEvent) {
	o.press()
	if t, rel, _ := o.target(e.AbsolutePosition); t != nil {
		if d, ok := t.(fyne.Draggable); ok {
			ev := *e
			ev.Position = rel
			d.Dragged(&ev)
		}
	}
}

func (o *tipOverlay) DragEnd() {
	o.remove()
	if d, ok := o.owner.(fyne.Draggable); ok {
		d.DragEnd()
	}
}

func (o *tipOverlay) Scrolled(e *fyne.ScrollEvent) {
	o.remove()
	if t, rel, over := o.target(e.AbsolutePosition); over {
		if sc, ok := t.(fyne.Scrollable); ok {
			ev := *e
			ev.Position = rel
			sc.Scrolled(&ev)
		}
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
