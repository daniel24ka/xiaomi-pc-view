package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestAreaEditor(t *testing.T) {
	test.NewTempApp(t)
	e := newAreaEditor()
	e.Resize(fyne.NewSize(1000, 360)) // the 640x360 picture in the middle: x 180..820
	e.r = [4]float32{80, 45, 560, 310}
	round := func() [4]float32 {
		var r [4]float32
		for i, v := range e.r {
			r[i] = float32(int(v + 0.5))
		}
		return r
	}
	drag := func(from, to fyne.Position) {
		e.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: to}, Dragged: fyne.NewDelta(to.X-from.X, to.Y-from.Y)})
		e.DragEnd()
	}
	// the bottom right corner (820-80, 310) to (700, 200)
	drag(fyne.NewPos(740, 310), fyne.NewPos(700, 200))
	if round() != [4]float32{80, 45, 520, 200} {
		t.Fatalf("corner: %v", e.r)
	}
	// move: inside, 500 px right is limited by the picture edge
	drag(fyne.NewPos(400, 100), fyne.NewPos(900, 100))
	if r := round(); r[2] != areaW || r[2]-r[0] != 440 {
		t.Fatalf("move: %v", e.r)
	}
	// a new rectangle outside the old one, drawn up-left
	drag(fyne.NewPos(300, 340), fyne.NewPos(200, 300))
	if round() != [4]float32{20, 300, 120, 340} {
		t.Fatalf("new: %v", e.r)
	}
	// too small: at least areaMin
	drag(fyne.NewPos(200, 20), fyne.NewPos(202, 22))
	if e.r[2]-e.r[0] < areaMin || e.r[3]-e.r[1] < areaMin {
		t.Fatalf("min size: %v", e.r)
	}
}
