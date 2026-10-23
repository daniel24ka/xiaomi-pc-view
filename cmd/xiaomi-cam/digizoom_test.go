package main

import (
	"image"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestDigitalZoom(t *testing.T) {
	test.NewTempApp(t)
	v := newVideoView()
	v.Resize(fyne.NewSize(960, 540))
	v.frame = image.NewRGBA(image.Rect(0, 0, 1920, 1080))

	// 2x around the centre: the middle quarter of the frame
	v.setDigitalZoom(2, nil)
	if r := v.dz.cropRect(1920, 1080); r != image.Rect(480, 270, 1440, 810) {
		t.Fatalf("crop %v", r)
	}

	// the wheel at a point: that point of the frame stays under the mouse
	v.setDigitalZoom(1, nil)
	pos := fyne.NewPos(720, 135) // 3/4 across, 1/4 down
	before := framePointAt(v, pos)
	v.setDigitalZoom(3, &pos)
	after := framePointAt(v, pos)
	if math.Abs(before[0]-after[0]) > 3 || math.Abs(before[1]-after[1]) > 3 {
		t.Fatalf("the point moved: %v -> %v", before, after)
	}

	// dragging far past the edge stops at the edge of the frame
	v.panBy(-100000, -100000)
	if r := v.dz.cropRect(1920, 1080); r.Max.X != 1920 || r.Max.Y != 1080 {
		t.Fatalf("pan not clamped: %v", r)
	}
	if !v.zoomed() {
		t.Fatal("zoomed")
	}
	v.setDigitalZoom(1, nil)
	if v.zoomed() {
		t.Fatal("not reset")
	}
}

// framePointAt: the frame pixel shown at a view position.
func framePointAt(v *videoView, pos fyne.Position) [2]float64 {
	sx, sy, sw, sh := v.shownRect()
	r := v.dz.cropRect(1920, 1080)
	if !v.zoomed() {
		r = image.Rect(0, 0, 1920, 1080)
	}
	px, py := float64((pos.X-sx)/sw), float64((pos.Y-sy)/sh)
	return [2]float64{float64(r.Min.X) + px*float64(r.Dx()), float64(r.Min.Y) + py*float64(r.Dy())}
}
