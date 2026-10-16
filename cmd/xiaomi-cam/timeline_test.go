package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// A drag moves the shown range (no pick), the playback position doesn't pull it back,
// a click picks the time and the range follows the playback again.
func TestTimelinePan(t *testing.T) {
	test.NewTempApp(t)
	tl := newTimeline()
	tl.Resize(fyne.NewSize(600, tlHeight))
	var picked time.Time
	tl.onPick = func(tm time.Time) { picked = tm }
	c0 := tl.center

	// drag 300 px to the right = half of the 2 h range back
	for range 3 {
		tl.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(300, 20)},
			Dragged: fyne.NewDelta(100, 0)})
	}
	tl.DragEnd()
	if d := c0.Sub(tl.center); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("drag moved the range by %v, want 1h back", d)
	}
	if !picked.IsZero() {
		t.Fatal("a drag picked a time")
	}
	moved := tl.center
	tl.setPos(c0) // the playback goes on at the old place
	if !tl.center.Equal(moved) {
		t.Fatal("the playback position pulled the browsed range back")
	}

	// no further than a bit after now
	tl.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-100000, 0)})
	if tl.center.After(time.Now().Add(tl.span / 2)) {
		t.Fatalf("the range went into the future: %v", tl.center)
	}

	// a horizontal scroll moves it too
	before := tl.center
	tl.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(60, 0)})
	if !tl.center.Before(before) {
		t.Fatal("a horizontal scroll didn't move the range back")
	}

	tl.Tapped(&fyne.PointEvent{Position: fyne.NewPos(0, 20)})
	if picked.IsZero() || tl.browsed {
		t.Fatal("a click didn't pick the time")
	}
}
