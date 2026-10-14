package main

import (
	"reflect"
	"sync/atomic"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// mover sends pan/tilt steps to the camera one by one. One step is small (~1-2°),
// so like in Mi Home the camera keeps moving while a button/key is held or the
// picture is dragged; each step waits for camera ack, so commands never pile up.
type mover struct {
	ops     chan int
	held    atomic.Int32 // op of the held button, 0 - none
	onError func(error)
	camera  func() string
}

func newMover(camera func() string, onError func(error)) *mover {
	m := &mover{ops: make(chan int, 4), camera: camera, onError: onError}
	go m.worker()
	return m
}

func (m *mover) worker() {
	for op := range m.ops {
		for {
			if err := motor(m.camera(), op); err != nil {
				m.onError(err)
				break
			}
			// continue while the button is still held
			if m.held.Load() != int32(op) {
				break
			}
		}
	}
}

// step queues one step, extra steps are dropped while the queue is full
func (m *mover) step(op int) {
	select {
	case m.ops <- op:
	default:
	}
}

func (m *mover) press(op int) {
	m.held.Store(int32(op))
	m.step(op)
}

func (m *mover) release() {
	m.held.Store(0)
}

// holdButton - button that reports press and release (moves the camera while held)
type holdButton struct {
	widget.Button
	onPress, onRelease func()
}

func newHoldButton(icon fyne.Resource, onPress, onRelease func()) *holdButton {
	b := &holdButton{onPress: onPress, onRelease: onRelease}
	b.Icon = icon
	b.ExtendBaseWidget(b)
	return b
}

func (b *holdButton) MouseDown(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonPrimary {
		b.onPress()
	}
}

func (b *holdButton) MouseUp(*desktop.MouseEvent) { b.onRelease() }
func (*holdButton) Cursor() desktop.Cursor        { return desktop.PointerCursor }
func (b *holdButton) MouseOut() {
	b.Button.MouseOut()
	b.onRelease()
}

// glfwWindow returns the GLFW window of a Fyne window. Fyne has no API for window
// frame, position and minimize events, so it is taken from the private field "viewport".
// GLFW calls must be done on the main thread (fyne.Do / UI events).
func glfwWindow(w fyne.Window) *glfw.Window {
	v := reflect.ValueOf(w)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	f := v.Elem().FieldByName("viewport")
	if !f.IsValid() || f.Type() != reflect.TypeOf((*glfw.Window)(nil)) || f.IsNil() {
		return nil
	}
	return (*glfw.Window)(unsafe.Pointer(f.Pointer()))
}

// setDecorated shows or hides the window frame (title bar and borders).
func setDecorated(w fyne.Window, on bool) {
	if win := glfwWindow(w); win != nil {
		attr := glfw.False
		if on {
			attr = glfw.True
		}
		win.SetAttrib(glfw.Decorated, attr)
	}
}

// onMinimize calls f when the window is minimized.
func onMinimize(w fyne.Window, f func()) {
	if win := glfwWindow(w); win != nil {
		win.SetIconifyCallback(func(_ *glfw.Window, iconified bool) {
			// switching to another workspace also reports "iconified" - not a minimize
			if iconified && wmOnCurrentDesktop(w) {
				f()
			}
		})
	}
}

// moveHandle - drag it to move the window (the window has no title bar in video-only mode)
type moveHandle struct {
	widget.Button
	win fyne.Window
	wm  bool // moved by the window manager
}

func newMoveHandle(win fyne.Window, importance widget.Importance) *moveHandle {
	h := &moveHandle{win: win}
	h.Icon = moveIcon
	h.Importance = importance
	h.ExtendBaseWidget(h)
	return h
}

func (*moveHandle) Cursor() desktop.Cursor { return desktop.PointerCursor }

// MouseDown hands moving over to the window manager (smooth, follows the mouse itself).
func (h *moveHandle) MouseDown(e *desktop.MouseEvent) {
	h.wm = e.Button == desktop.MouseButtonPrimary && wmMoveResize(h.win, wmMove)
}

func (*moveHandle) MouseUp(*desktop.MouseEvent) {}

// Dragged - fallback when the window manager can't move the window (not X11).
func (h *moveHandle) Dragged(e *fyne.DragEvent) {
	if h.wm {
		return
	}
	if win := glfwWindow(h.win); win != nil {
		scale := h.win.Canvas().Scale()
		x, y := win.GetPos()
		win.SetPos(x+int(e.Dragged.DX*scale), y+int(e.Dragged.DY*scale))
	}
}

func (*moveHandle) DragEnd() {}

var moveIcon = theme.NewThemedResource(fyne.NewStaticResource("move.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M12 2l3.5 3.5h-2.5v5.5h5.5v-2.5l3.5 3.5-3.5 3.5v-2.5h-5.5v5.5h2.5l-3.5 3.5-3.5-3.5h2.5v-5.5h-5.5v2.5l-3.5-3.5 3.5-3.5v2.5h5.5v-5.5h-2.5z"/></svg>`)))

// Clickable widgets with the pointer (hand) cursor on hover; Fyne uses the default arrow.

type ptrButton struct{ widget.Button }

func (*ptrButton) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newButton(label string, icon fyne.Resource, tapped func()) *ptrButton {
	b := &ptrButton{}
	b.Text, b.Icon, b.OnTapped = label, icon, tapped
	b.ExtendBaseWidget(b)
	return b
}

type ptrSelect struct{ widget.Select }

func (*ptrSelect) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newSelect(options []string, changed func(string)) *ptrSelect {
	s := &ptrSelect{}
	s.Options, s.OnChanged = options, changed
	s.ExtendBaseWidget(s)
	return s
}

type ptrCheck struct{ widget.Check }

func (*ptrCheck) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newCheck(label string, changed func(bool)) *ptrCheck {
	c := &ptrCheck{}
	c.Text, c.OnChanged = label, changed
	c.ExtendBaseWidget(c)
	return c
}

type ptrSlider struct{ widget.Slider }

func (*ptrSlider) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newSlider(min, max float64) *ptrSlider {
	s := &ptrSlider{}
	s.Min, s.Max, s.Step = min, max, 1
	s.Orientation = widget.Horizontal
	s.ExtendBaseWidget(s)
	return s
}
