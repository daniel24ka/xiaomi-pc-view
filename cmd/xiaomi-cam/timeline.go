package main

import (
	"time"

	"github.com/AlexxIT/go2rtc/internal/xccore"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// timeline - the history time axis: a scale (hours with labels, minor ticks), the known
// recorded files, the events (colored by kind), the playback position; the mouse shows the time under it (with a
// thumbnail, see onHover), a click picks the time, a drag (or a horizontal scroll) moves the
// shown range, the wheel zooms (15 min – 24 h).
// It draws a few dozen rectangles and texts, rebuilt only when the view changes.
type timeline struct {
	widget.BaseWidget

	center time.Time     // the middle of the shown range
	span   time.Duration // the shown range
	pos    time.Time     // the playback position (zero: none)
	hover  time.Time     // the time under the mouse (zero: none)

	files    func(from, to time.Time) [][2]time.Time // known recordings in the range
	events   func(from, to time.Time) []xccore.Event // known events in the range (events.go)
	onPick   func(t time.Time)                       // a time chosen (click, drag released)
	onHover  func(t time.Time, x float32)            // the mouse over the axis (zero t: left)
	dragging bool
	browsed  bool // the user moved or zoomed the range: it doesn't follow the playback
}

const (
	tlMinSpan = 15 * time.Minute
	tlMaxSpan = 24 * time.Hour
	tlHeight  = 46
)

func newTimeline() *timeline {
	t := &timeline{center: time.Now(), span: 2 * time.Hour}
	t.ExtendBaseWidget(t)
	return t
}

func (t *timeline) from() time.Time { return t.center.Add(-t.span / 2) }

// timeAt - the time at x (widget coordinates)
func (t *timeline) timeAt(x float32) time.Time {
	w := t.Size().Width
	if w <= 0 {
		return t.center
	}
	f := float64(min(max(x, 0), w) / w)
	return t.from().Add(time.Duration(f * float64(t.span)))
}

// xAt - the x of a time
func (t *timeline) xAt(tm time.Time) float32 {
	return float32(float64(tm.Sub(t.from())) / float64(t.span) * float64(t.Size().Width))
}

// show makes the time visible (the range moves when it is near an edge), unless the user
// is browsing another range.
func (t *timeline) show(tm time.Time) {
	if !t.browsed && (tm.Before(t.from().Add(t.span/10)) || tm.After(t.from().Add(t.span*9/10))) {
		t.center = tm
	}
	t.Refresh()
}

func (t *timeline) setPos(tm time.Time) {
	t.pos = tm
	t.show(tm)
}

// --- mouse

func (t *timeline) Tapped(e *fyne.PointEvent) {
	t.browsed = false
	if t.onPick != nil {
		t.onPick(t.timeAt(e.Position.X))
	}
}

// Dragged moves the shown range with the mouse (the scale follows the pointer).
func (t *timeline) Dragged(e *fyne.DragEvent) {
	t.dragging = true
	t.pan(-e.Dragged.DX)
	t.hover = t.timeAt(e.Position.X)
	t.Refresh()
	if t.onHover != nil {
		t.onHover(t.hover, e.Position.X)
	}
}

func (t *timeline) DragEnd() { t.dragging = false }

// pan moves the range by dx pixels (positive: later), not further than a bit after now.
func (t *timeline) pan(dx float32) {
	w := max(t.Size().Width, 1)
	t.center = t.center.Add(time.Duration(float64(dx) / float64(w) * float64(t.span)))
	if latest := time.Now().Add(t.span * 4 / 10); t.center.After(latest) {
		t.center = latest
	}
	t.browsed = true
}

func (t *timeline) MouseIn(e *desktop.MouseEvent) { t.MouseMoved(e) }
func (t *timeline) MouseMoved(e *desktop.MouseEvent) {
	t.hover = t.timeAt(e.Position.X)
	t.Refresh()
	if t.onHover != nil {
		t.onHover(t.hover, e.Position.X)
	}
}
func (t *timeline) MouseOut() {
	if t.dragging {
		return
	}
	t.hover = time.Time{}
	t.Refresh()
	if t.onHover != nil {
		t.onHover(time.Time{}, 0)
	}
}

func (t *timeline) Cursor() desktop.Cursor { return desktop.PointerCursor }

// Scrolled zooms around the time under the mouse; a horizontal scroll (touchpad) moves
// the range.
func (t *timeline) Scrolled(e *fyne.ScrollEvent) {
	if dx, dy := e.Scrolled.DX, e.Scrolled.DY; dx != 0 && abs32(dx) > abs32(dy) {
		t.pan(-dx)
		t.hover = t.timeAt(e.Position.X)
		t.Refresh()
		return
	}
	at := t.timeAt(e.Position.X)
	span := t.span
	if e.Scrolled.DY > 0 {
		span = span * 2 / 3
	} else {
		span = span * 3 / 2
	}
	span = min(max(span, tlMinSpan), tlMaxSpan)
	// keep the time under the mouse in place
	f := float64(e.Position.X / max(t.Size().Width, 1))
	t.span = span
	t.center = at.Add(-time.Duration(f*float64(span)) + span/2)
	t.browsed = true
	t.Refresh()
}

// --- drawing

// tickSteps - minor tick / labelled tick, chosen by the shown range
func (t *timeline) tickSteps() (minor, major time.Duration) {
	w := float64(max(t.Size().Width, 100))
	perMin := w / t.span.Minutes() // px per minute
	for _, s := range []struct{ minor, major time.Duration }{
		{time.Minute, 5 * time.Minute}, {5 * time.Minute, 15 * time.Minute},
		{5 * time.Minute, 30 * time.Minute}, {10 * time.Minute, time.Hour},
		{15 * time.Minute, time.Hour}, {30 * time.Minute, 2 * time.Hour},
		{time.Hour, 3 * time.Hour}, {time.Hour, 6 * time.Hour},
	} {
		if perMin*s.minor.Minutes() >= 6 && perMin*s.major.Minutes() >= 70 {
			return s.minor, s.major
		}
	}
	return time.Hour, 6 * time.Hour
}

func (t *timeline) MinSize() fyne.Size { return fyne.NewSize(120, tlHeight) }

func (t *timeline) CreateRenderer() fyne.WidgetRenderer {
	r := &timelineRenderer{t: t}
	r.rebuild(t.Size())
	return r
}

type timelineRenderer struct {
	t    *timeline
	objs []fyne.CanvasObject
}

func (r *timelineRenderer) Layout(size fyne.Size)        { r.rebuild(size) }
func (r *timelineRenderer) MinSize() fyne.Size           { return r.t.MinSize() }
func (r *timelineRenderer) Refresh()                     { r.rebuild(r.t.Size()); canvas.Refresh(r.t) }
func (r *timelineRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *timelineRenderer) Destroy()                     {}

func (r *timelineRenderer) rebuild(size fyne.Size) {
	t := r.t
	r.objs = r.objs[:0]
	if size.Width <= 0 {
		return
	}
	const barY, barH = 22, 12
	rect := func(x, y, w, h float32, c fyne.ThemeColorName) *canvas.Rectangle {
		o := canvas.NewRectangle(theme.Color(c))
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(w, h))
		r.objs = append(r.objs, o)
		return o
	}
	// the bar, the future greyed out
	bar := rect(0, barY, size.Width, barH, theme.ColorNameInputBackground)
	bar.CornerRadius = 4
	if now := t.xAt(time.Now()); now < size.Width {
		f := rect(max(now, 0), barY, size.Width-max(now, 0), barH, theme.ColorNameDisabledButton)
		f.CornerRadius = 4
	}
	// the known recordings
	if t.files != nil {
		for _, f := range t.files(t.from(), t.from().Add(t.span)) {
			x0, x1 := max(t.xAt(f[0]), 0), min(t.xAt(f[1]), size.Width)
			if x1 > x0 {
				seg := canvas.NewRectangle(alpha(palette.live, 0x70))
				seg.Move(fyne.NewPos(x0, barY+3))
				seg.Resize(fyne.NewSize(x1-x0, barH-6))
				r.objs = append(r.objs, seg)
			}
		}
	}
	// the events: a mark per recording with an event (the file of ~1 min), at least 2 px
	if t.events != nil {
		lastX := float32(-10)
		for _, e := range t.events(t.from().Add(-time.Minute), t.from().Add(t.span)) {
			x0, x1 := t.xAt(e.Time), t.xAt(e.Time.Add(time.Minute))
			x0, x1 = max(x0, 0), min(max(x1, x0+2), size.Width)
			if x1 <= x0 || x1 <= lastX { // hidden under the previous mark
				continue
			}
			x0 = max(x0, lastX)
			lastX = x1
			m := canvas.NewRectangle(eventColor(e.Kind))
			m.Move(fyne.NewPos(x0, barY))
			m.Resize(fyne.NewSize(x1-x0, barH))
			r.objs = append(r.objs, m)
		}
	}
	// the scale
	minor, major := t.tickSteps()
	start := t.from().Truncate(minor)
	for tm := start; tm.Before(t.from().Add(t.span)); tm = tm.Add(minor) {
		x := t.xAt(tm)
		if x < 0 {
			continue
		}
		local := tm.In(time.Local)
		big := (local.Hour()*60+local.Minute())%int(major.Minutes()) == 0
		h := float32(5)
		if big {
			h = 10
		}
		tick := canvas.NewRectangle(palette.muted)
		if !big {
			tick.FillColor = alpha(palette.muted, 0x80)
		}
		tick.Move(fyne.NewPos(x, barY-h))
		tick.Resize(fyne.NewSize(1, h))
		r.objs = append(r.objs, tick)
		if big {
			label := canvas.NewText(local.Format("15:04"), palette.muted)
			label.TextSize = 10
			m := fyne.MeasureText(label.Text, 10, label.TextStyle)
			label.Move(fyne.NewPos(min(max(x-m.Width/2, 0), size.Width-m.Width), 0))
			r.objs = append(r.objs, label)
		}
	}
	// the playback position and the time under the mouse
	if !t.pos.IsZero() {
		if x := t.xAt(t.pos); x >= 0 && x <= size.Width {
			p := canvas.NewRectangle(palette.accent)
			p.Move(fyne.NewPos(x-1, barY-4))
			p.Resize(fyne.NewSize(3, barH+8))
			r.objs = append(r.objs, p)
		}
	}
	if !t.hover.IsZero() {
		x := t.xAt(t.hover)
		hv := canvas.NewRectangle(palette.text)
		hv.Move(fyne.NewPos(x, barY-2))
		hv.Resize(fyne.NewSize(1, barH+4))
		r.objs = append(r.objs, hv)
	}
}
