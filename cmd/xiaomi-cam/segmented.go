package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// segmented - a row of options with one selected (quality 2.5K | 720p | SD). The same
// fields and methods as the select it replaces (Options, Selected, SetSelectedIndex,
// SelectedIndex, Enable/Disable), so the code using it doesn't care which one it is.
type segmented struct {
	widget.DisableableWidget
	Options   []string
	Selected  string
	OnChanged func(string)
	tip       string

	row *fyne.Container
}

func newSegmented(changed func(string)) *segmented {
	s := &segmented{OnChanged: changed, row: container.NewHBox()}
	s.ExtendBaseWidget(s)
	return s
}

func (s *segmented) SelectedIndex() int {
	for i, o := range s.Options {
		if o == s.Selected {
			return i
		}
	}
	return -1
}

// SetSelectedIndex selects an option; OnChanged is called when it changes.
func (s *segmented) SetSelectedIndex(i int) {
	if i < 0 || i >= len(s.Options) {
		return
	}
	old := s.Selected
	s.Selected = s.Options[i]
	s.Refresh()
	if s.Selected != old && s.OnChanged != nil {
		s.OnChanged(s.Selected)
	}
}

func (s *segmented) Refresh() {
	s.rebuild()
	s.BaseWidget.Refresh()
}

// rebuild makes the option buttons again.
func (s *segmented) rebuild() {
	s.row.Objects = nil
	for i, o := range s.Options {
		g := &segment{s: s, i: i, text: o}
		g.ExtendBaseWidget(g)
		s.row.Add(g)
	}
	s.row.Refresh()
}

func (s *segmented) Disable() {
	s.DisableableWidget.Disable()
	s.Refresh()
}

func (s *segmented) Enable() {
	s.DisableableWidget.Enable()
	s.Refresh()
}

func (s *segmented) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(palette.bg)
	bg.StrokeColor, bg.StrokeWidth = palette.line, 1
	bg.CornerRadius = theme.InputRadiusSize()
	s.rebuild()
	return widget.NewSimpleRenderer(container.NewStack(bg, container.New(&insetLayout{3}, s.row)))
}

// segment - one option of a segmented
type segment struct {
	widget.BaseWidget
	s     *segmented
	i     int
	text  string
	hover bool
}

func (g *segment) selected() bool { return g.s.Selected == g.text }

func (g *segment) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(nil)
	bg.CornerRadius = theme.InputRadiusSize() - 3
	label := canvas.NewText(g.text, palette.text)
	label.TextSize = theme.TextSize()
	label.Alignment = fyne.TextAlignCenter
	r := &segmentRenderer{g: g, bg: bg, label: label}
	r.Refresh()
	return r
}

func (g *segment) Tapped(*fyne.PointEvent) {
	if !g.s.Disabled() {
		tipOut()
		g.s.SetSelectedIndex(g.i)
	}
}

func (g *segment) Cursor() desktop.Cursor {
	if g.s.Disabled() || g.selected() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

func (g *segment) MouseIn(e *desktop.MouseEvent) {
	g.hover = true
	g.Refresh()
	tipIn(g, g.s.tip, e)
}
func (g *segment) MouseMoved(*desktop.MouseEvent) {}
func (g *segment) MouseOut() {
	g.hover = false
	g.Refresh()
	tipOut()
}

type segmentRenderer struct {
	g     *segment
	bg    *canvas.Rectangle
	label *canvas.Text
}

func (r *segmentRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.label.Resize(size)
}

func (r *segmentRenderer) MinSize() fyne.Size {
	m := fyne.MeasureText(r.g.text, theme.TextSize(), r.label.TextStyle)
	return fyne.NewSize(m.Width+2*theme.Padding()+4, 28)
}

func (r *segmentRenderer) Refresh() {
	switch {
	case r.g.selected():
		r.bg.FillColor = palette.raised
		r.label.Color = palette.text
		r.label.TextStyle.Bold = true
	case r.g.hover && !r.g.s.Disabled():
		r.bg.FillColor = alpha(palette.raised, 0x80)
		r.label.Color = palette.text
		r.label.TextStyle.Bold = false
	default:
		r.bg.FillColor = nil
		r.label.Color = palette.muted
		r.label.TextStyle.Bold = false
	}
	if r.g.s.Disabled() {
		r.label.Color = theme.Color(theme.ColorNameDisabled)
	}
	r.bg.Refresh()
	r.label.Refresh()
}

func (r *segmentRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.label} }
func (r *segmentRenderer) Destroy()                     {}

// insetLayout - the content inset by a fixed margin on every side
type insetLayout struct{ m float32 }

func (l *insetLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(l.m, l.m))
		o.Resize(size.SubtractWidthHeight(2*l.m, 2*l.m))
	}
}

func (l *insetLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objs {
		s = s.Max(o.MinSize())
	}
	return s.AddWidthHeight(2*l.m, 2*l.m)
}
