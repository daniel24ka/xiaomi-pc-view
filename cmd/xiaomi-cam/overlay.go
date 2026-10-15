package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Controls lying on the picture: the status (top right; top left is the camera's own
// date), pan/tilt (bottom left), camera points (bottom centre), zoom, light and sound
// (bottom right). The bottom ones show when the mouse moves over the picture and hide after
// a while without movement, but not while the mouse is on them. They are plain translucent
// rectangles (no blur): drawing them costs next to nothing.

const (
	controlsIdle   = 3 * time.Second
	statusMaxWidth = 420 // the status chip; a longer text is truncated
)

// pill - a rounded translucent background with the content inside
func pill(content fyne.CanvasObject) *fyne.Container {
	bg := canvas.NewRectangle(overlayBg)
	bg.CornerRadius = 22
	return container.NewStack(bg, container.New(&insetLayout{4}, content))
}

// atBottom keeps an object at its minimum height at the bottom of its cell.
func atBottom(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), o)
}

// buildPictureControls builds the controls on the picture; pad - pan/tilt buttons
// (left, up, down, right), points and zoom: the button rows of these groups.
func (u *ui) buildPictureControls(pad [4]fyne.CanvasObject, points, zoom fyne.CanvasObject) *fyne.Container {
	gap := func() fyne.CanvasObject { return layout.NewSpacer() }
	dot := canvas.NewCircle(palette.raised)
	padGrid := container.NewGridWithColumns(3,
		gap(), pad[1], gap(),
		pad[0], container.NewCenter(container.NewGridWrap(fyne.NewSize(8, 8), dot)), pad[3],
		gap(), pad[2], gap())
	u.capPTZ = pill(padGrid)
	u.capPoints = pill(points)
	u.capZoom = zoom
	right := pill(container.NewHBox(zoom, u.camControls, u.soundBox))

	// the status chip: a dot (green: live picture) and the status text
	u.liveDot = canvas.NewCircle(palette.muted)
	u.status.changed = func() {
		if u.statusChip == nil {
			return
		}
		if u.status.Text == "" || u.viewOnly {
			u.statusChip.Hide() // nothing to say (grid): the chip doesn't cover a tile
			return
		}
		u.statusChip.Show()
		u.statusChip.Refresh() // the chip's width follows the text (up to its maximum)
	}
	statusRow := container.NewBorder(nil, nil, nil,
		container.New(&statusLayout{u.status, statusMaxWidth}, container.NewBorder(nil, nil,
			container.NewCenter(container.NewGridWrap(fyne.NewSize(8, 8), u.liveDot)), u.activity, u.status)))

	bottom := container.NewStack(
		container.NewBorder(nil, nil, atBottom(u.capPTZ), atBottom(right)),
		container.NewCenter(atBottom(u.capPoints)), // centred in the window
	)
	u.bottomControls = container.New(layout.NewCustomPaddedLayout(0, 16, 16, 16), bottom)
	u.statusChip = container.New(layout.NewCustomPaddedLayout(12, 0, 16, 16), container.NewHBox(layout.NewSpacer(), pill(statusRow)))
	return container.NewBorder(u.statusChip, u.bottomControls, nil, nil)
}

// pictureControlsActive - the mouse moved over the picture: show the controls, hide them
// after a while without movement.
func (u *ui) pictureControlsActive() {
	if u.viewOnly {
		u.showExitButton()
		return
	}
	if u.bottomControls == nil {
		return
	}
	if !u.bottomControls.Visible() {
		u.bottomControls.Show()
	}
	if u.controlsTimer != nil {
		u.controlsTimer.Stop()
	}
	u.controlsTimer = time.AfterFunc(controlsIdle, func() { fyne.Do(u.hidePictureControls) })
}

// hidePictureControls hides the bottom controls unless the mouse is on them or one of
// their panels is open.
func (u *ui) hidePictureControls() {
	if u.bottomControls == nil || !u.bottomControls.Visible() {
		return
	}
	if u.mouseOverControls() || len(u.win.Canvas().Overlays().List()) > 0 {
		u.pictureControlsActive() // ask again later
		return
	}
	u.bottomControls.Hide()
}

// mouseOverControls: the mouse cursor is on one of the bottom pills.
func (u *ui) mouseOverControls() bool {
	win := glfwWindow(u.win)
	if win == nil {
		return false
	}
	cx, cy := win.GetCursorPos()
	sc := u.win.Canvas().Scale()
	p := fyne.NewPos(float32(cx)/sc, float32(cy)/sc)
	d := fyne.CurrentApp().Driver()
	for _, o := range []fyne.CanvasObject{u.capPTZ, u.capPoints, u.capZoom, u.camControls, u.soundBox} {
		if o == nil || !o.Visible() {
			continue
		}
		at, s := d.AbsolutePositionForObject(o), o.Size()
		if p.X >= at.X && p.Y >= at.Y && p.X < at.X+s.Width && p.Y < at.Y+s.Height {
			return true
		}
	}
	// the right pill as a whole (gaps between its buttons)
	if u.soundBox != nil {
		at := d.AbsolutePositionForObject(u.soundBox)
		if p.Y >= at.Y-8 && p.Y < at.Y+u.soundBox.Size().Height+8 && p.X >= at.X-300 {
			return true
		}
	}
	return false
}

// statusLabel - the status text; a change of the text re-lays out the chip it is in (a
// truncated label alone keeps its minimum size, so its parents would not grow)
type statusLabel struct {
	widget.Label
	changed func()
}

func newStatusLabel() *statusLabel {
	l := &statusLabel{}
	l.ExtendBaseWidget(l)
	return l
}

func (l *statusLabel) SetText(text string) {
	if text == l.Text {
		return
	}
	full := fyne.MeasureText(text, theme.TextSize(), l.TextStyle).Width + 2*theme.InnerPadding()
	if full > statusMaxWidth-60 { // the dot and the spinner take the rest
		l.Truncation = fyne.TextTruncateEllipsis
	} else {
		l.Truncation = fyne.TextTruncateOff
	}
	l.Label.SetText(text)
	if l.changed != nil {
		l.changed()
	}
}

// setLive shows the live dot green (a picture) or grey (connecting, history...).
func (u *ui) setLive(on bool) {
	if u.liveDot == nil {
		return
	}
	c := palette.muted
	if on {
		c = palette.live
	}
	if u.liveDot.FillColor != c {
		u.liveDot.FillColor = c
		u.liveDot.Refresh()
	}
}

// overlayButton - an icon button lying on the picture (no own background: the pill's)
func overlayButton(icon fyne.Resource, tip string, tapped func()) *ptrButton {
	b := newButton("", icon, tapped).withTip(tip)
	b.Importance = widget.LowImportance
	return b
}

// statusLayout - the status chip as wide as its text needs, at most max (then truncated)
type statusLayout struct {
	label *statusLabel
	max   float32
}

func (l *statusLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(size)
	}
}

func (l *statusLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objs {
		s = s.Max(o.MinSize())
	}
	if l.label.Truncation != fyne.TextTruncateOff {
		s.Width = l.max // a long text: as wide as allowed, truncated in it
	}
	s.Width = min(s.Width, l.max)
	return s
}

// chipLayout - a chip with a truncatable label: as wide as its text, at most the space it
// has, and asking for almost no width itself (a narrow grid tile shortens the name)
type chipLayout struct {
	label *widget.Label
	text  string    // the text full was measured for
	full  fyne.Size // the label's size without truncation
}

func (l *chipLayout) natural(o fyne.CanvasObject) fyne.Size {
	if l.text != l.label.Text { // measured like Fyne does: an untruncated copy, once per text
		m := widget.NewLabelWithStyle(l.label.Text, l.label.Alignment, l.label.TextStyle)
		m.SizeName = l.label.SizeName
		l.text, l.full = l.label.Text, m.MinSize().AddWidthHeight(4, 0) // Fyne truncates at the exact width
	}
	s := o.MinSize()
	s.Width += max(l.full.Width-l.label.MinSize().Width, 0)
	return s
}

func (l *chipLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		n := l.natural(o)
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(min(n.Width, size.Width), n.Height))
	}
}

func (l *chipLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objs {
		s = s.Max(o.MinSize())
	}
	return s
}

// tightVBox - objects stacked with no gap (a number and a small name under it)
type tightVBox struct{}

func (tightVBox) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, m.Height))
		y += m.Height
	}
}

func (tightVBox) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objs {
		m := o.MinSize()
		if t, ok := o.(*canvas.Text); ok && t.Text == "" {
			continue // no name: the number alone is centred
		}
		s.Width = max(s.Width, m.Width)
		s.Height += m.Height
	}
	return s
}
