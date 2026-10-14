package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// "Action in progress" indicators for controls whose action takes a while (camera
// commands, network): the clicked button shows a spinning icon and ignores clicks, and a
// spinner is shown next to the status line until the action ends.

// spinnerFrames - a 270° arc rotated in 30° steps (themed: follows the icon color);
// videoSpinnerFrames - light gray, for the black video background
var spinnerFrames, videoSpinnerFrames = makeSpinner("spinner", ""), makeSpinner("vspinner", ` fill="#c8c8c8"`)

func makeSpinner(name, fill string) []fyne.Resource {
	var frames []fyne.Resource
	for deg := 0; deg < 360; deg += 30 {
		svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g transform="rotate(%d 12 12)"><path%s d="M12 3A9 9 0 1 1 3 12L5.5 12A6.5 6.5 0 1 0 12 5.5Z"/></g></svg>`, deg, fill)
		var r fyne.Resource = fyne.NewStaticResource(fmt.Sprintf("%s%d.svg", name, deg), []byte(svg))
		if fill == "" {
			r = theme.NewThemedResource(r)
		}
		frames = append(frames, r)
	}
	return frames
}

// spinner - a rotating arc icon, hidden while stopped
type spinner struct {
	widget.Icon
	frames []fyne.Resource
	stop   chan struct{}
}

func newSpinner(frames []fyne.Resource) *spinner {
	s := &spinner{frames: frames}
	s.Resource = frames[0]
	s.ExtendBaseWidget(s)
	s.Hide()
	return s
}

func (s *spinner) running() bool { return s.stop != nil }

// run shows and animates the spinner, or stops and hides it. UI thread.
func (s *spinner) run(on bool) {
	if on == s.running() {
		return
	}
	if !on {
		close(s.stop)
		s.stop = nil
		s.Hide()
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	s.Show()
	animate(stop, s.frames, func(f fyne.Resource) {
		if s.stop == stop {
			s.SetResource(f)
		}
	})
}

// animate calls set (on the UI thread) with the next frame every 80 ms until stop is closed.
func animate(stop chan struct{}, frames []fyne.Resource, set func(fyne.Resource)) {
	go func() {
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 1; ; i++ {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			frame := frames[i%len(frames)]
			fyne.Do(func() { set(frame) })
		}
	}()
}

// setBusy shows a spinning icon on the button (instead of its icon) and ignores clicks
// until setBusy(false). UI thread.
func (b *ptrButton) setBusy(on bool) {
	if on == (b.busyStop != nil) {
		return
	}
	if !on {
		close(b.busyStop)
		b.busyStop = nil
		b.SetIcon(b.busyIcon)
		return
	}
	stop := make(chan struct{})
	b.busyStop, b.busyIcon = stop, b.Icon
	b.SetIcon(spinnerFrames[0])
	animate(stop, spinnerFrames, func(f fyne.Resource) {
		if b.busyStop == stop { // not finished meanwhile
			b.SetIcon(f)
		}
	})
}

func (b *ptrButton) isBusy() bool { return b.busyStop != nil }

// SetIcon while busy changes the icon restored at the end (e.g. the record button).
func (b *ptrButton) setIdleIcon(icon fyne.Resource) {
	if b.isBusy() {
		b.busyIcon = icon
		return
	}
	b.SetIcon(icon)
}

func (b *ptrButton) Tapped(e *fyne.PointEvent) {
	if b.isBusy() {
		return
	}
	b.Button.Tapped(e)
}

func (b *ptrButton) Cursor() desktop.Cursor {
	if b.isBusy() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// busy marks an action in progress: spinner on the button b (nil: none), spinner next to
// the status line, and the widgets ws disabled (no repeated clicks / toggles). The
// returned func ends it; call it on the UI thread (more calls are ignored).
func (u *ui) busy(b *ptrButton, ws ...fyne.Disableable) (done func()) {
	if b != nil {
		b.setBusy(true)
	}
	for _, w := range ws {
		w.Disable()
	}
	u.busyCount++
	u.activity.run(true)
	ended := false
	return func() {
		if ended {
			return
		}
		ended = true
		if b != nil {
			b.setBusy(false)
		}
		for _, w := range ws {
			w.Enable()
		}
		if u.busyCount--; u.busyCount <= 0 {
			u.busyCount = 0
			u.activity.run(false)
		}
	}
}
