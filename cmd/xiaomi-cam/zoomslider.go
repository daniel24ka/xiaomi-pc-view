package main

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// zoomSlider - vertical camera zoom slider with a scale (ticks every 0.5×, labels every 1×),
// used over the picture in video-only mode. Min (1×) is at the bottom.
type zoomSlider struct {
	widget.BaseWidget
	min, max, step float64
	value          float64
	onChanged      func(v float64) // while dragging
	onChangeEnded  func(v float64) // on release / tap
}

const (
	zsWidth  = 64
	zsHeight = 240
	zsPad    = 14 // track margin top/bottom
	zsTrackX = 46 // x of the track, ticks and labels are left of it
)

func newZoomSlider(min, max, step float64) *zoomSlider {
	s := &zoomSlider{min: min, max: max, step: step, value: min}
	s.ExtendBaseWidget(s)
	return s
}

func (s *zoomSlider) SetValue(v float64) {
	if v != s.value {
		s.value = v
		s.Refresh()
	}
}

func (s *zoomSlider) yOf(v float64) float32 {
	h := s.Size().Height - 2*zsPad
	return zsPad + h*float32(1-(v-s.min)/(s.max-s.min))
}

func (s *zoomSlider) valueAt(y float32) float64 {
	h := s.Size().Height - 2*zsPad
	f := 1 - float64((y-zsPad)/h)
	v := s.min + f*(s.max-s.min)
	v = float64(int(v/s.step+0.5)) * s.step
	return min(max(v, s.min), s.max)
}

func (s *zoomSlider) Tapped(e *fyne.PointEvent) {
	s.SetValue(s.valueAt(e.Position.Y))
	if s.onChangeEnded != nil {
		s.onChangeEnded(s.value)
	}
}

func (s *zoomSlider) Dragged(e *fyne.DragEvent) {
	if v := s.valueAt(e.Position.Y); v != s.value {
		s.SetValue(v)
		if s.onChanged != nil {
			s.onChanged(v)
		}
	}
}

func (s *zoomSlider) DragEnd() {
	if s.onChangeEnded != nil {
		s.onChangeEnded(s.value)
	}
}

func (*zoomSlider) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *zoomSlider) MinSize() fyne.Size { return fyne.NewSize(zsWidth, zsHeight) }

func (s *zoomSlider) CreateRenderer() fyne.WidgetRenderer {
	r := &zoomSliderRenderer{s: s}
	r.track = canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 120})
	r.fill = canvas.NewRectangle(color.NRGBA{R: 0xff, G: 0x67, B: 0x00, A: 255})
	r.thumb = canvas.NewCircle(color.White)
	r.thumb.StrokeColor = color.NRGBA{R: 0xff, G: 0x67, B: 0x00, A: 255}
	r.thumb.StrokeWidth = 3
	for v := s.min; v <= s.max+1e-9; v += s.step {
		r.ticks = append(r.ticks, canvas.NewLine(color.NRGBA{R: 255, G: 255, B: 255, A: 200}))
		if v == float64(int(v)) {
			t := canvas.NewText(fmt.Sprintf("%.0f×", v), color.White)
			t.TextSize = 11
			t.Alignment = fyne.TextAlignTrailing
			r.labels = append(r.labels, t)
		}
	}
	return r
}

type zoomSliderRenderer struct {
	s           *zoomSlider
	track, fill *canvas.Rectangle
	thumb       *canvas.Circle
	ticks       []*canvas.Line
	labels      []*canvas.Text
}

func (r *zoomSliderRenderer) Layout(size fyne.Size) {
	s := r.s
	top, bottom := s.yOf(s.max), s.yOf(s.min)
	r.track.Move(fyne.NewPos(zsTrackX-2, top))
	r.track.Resize(fyne.NewSize(4, bottom-top))
	y := s.yOf(s.value)
	r.fill.Move(fyne.NewPos(zsTrackX-2, y))
	r.fill.Resize(fyne.NewSize(4, bottom-y))
	r.thumb.Move(fyne.NewPos(zsTrackX-8, y-8))
	r.thumb.Resize(fyne.NewSize(16, 16))
	li := 0
	for i, l := range r.ticks {
		v := s.min + float64(i)*s.step
		ty := s.yOf(v)
		long := v == float64(int(v))
		x0 := float32(zsTrackX - 7)
		if long {
			x0 = zsTrackX - 12
		}
		l.Position1 = fyne.NewPos(x0, ty)
		l.Position2 = fyne.NewPos(zsTrackX-3, ty)
		if long && li < len(r.labels) {
			t := r.labels[li]
			t.Move(fyne.NewPos(0, ty-8))
			t.Resize(fyne.NewSize(zsTrackX-15, 16))
			li++
		}
	}
}

func (r *zoomSliderRenderer) MinSize() fyne.Size { return r.s.MinSize() }
func (r *zoomSliderRenderer) Refresh() {
	r.Layout(r.s.Size())
	canvas.Refresh(r.s)
}
func (r *zoomSliderRenderer) Destroy() {}
func (r *zoomSliderRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.track, r.fill}
	for _, l := range r.ticks {
		objs = append(objs, l)
	}
	for _, t := range r.labels {
		objs = append(objs, t)
	}
	return append(objs, r.thumb)
}
