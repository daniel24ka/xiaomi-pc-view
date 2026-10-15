package main

import (
	"reflect"
	"sync"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// mover drives the pan/tilt motor like the Mi Home plugin does: a direction command
// (operation 1-4) starts a continuous movement and is repeated every 200 ms while the button
// or key is held; on release the stop command (operation -1001) is sent - right away after
// a long press, 200 ms later after a short click (so a click moves the camera a bit).
type mover struct {
	mu      sync.Mutex
	held    int       // direction being held, 0 - none
	gen     int       // press counter: a stop belongs to the press it was scheduled for
	since   time.Time // when the current press started
	onError func(error)
	camera  func() string
}

const (
	motorStop     = -1001 // CMD_OFF in the Mi Home plugin
	motorRepeat   = 200 * time.Millisecond
	motorLongHold = 800 * time.Millisecond
)

func newMover(camera func() string, onError func(error)) *mover {
	return &mover{camera: camera, onError: onError}
}

func (m *mover) send(op int) {
	if err := motor(m.camera(), op); err != nil && m.onError != nil {
		m.onError(err)
	}
}

// press starts moving in the direction (button down / key down / drag).
func (m *mover) press(op int) {
	m.mu.Lock()
	if m.held == op {
		m.mu.Unlock()
		return
	}
	m.gen++
	gen := m.gen
	m.held, m.since = op, time.Now()
	m.mu.Unlock()

	go func() {
		for {
			m.send(op)
			time.Sleep(motorRepeat)
			m.mu.Lock()
			still := m.gen == gen && m.held == op
			m.mu.Unlock()
			if !still {
				return
			}
		}
	}()
}

// release stops the movement (button up / key up / end of drag).
func (m *mover) release() {
	m.mu.Lock()
	if m.held == 0 {
		m.mu.Unlock()
		return
	}
	gen := m.gen
	delay := motorRepeat // short click: let it move a bit
	if time.Since(m.since) > motorLongHold {
		delay = 0
	}
	m.held = 0
	m.mu.Unlock()

	time.AfterFunc(delay, func() {
		m.mu.Lock()
		current := m.gen == gen && m.held == 0 // not pressed again meanwhile
		m.mu.Unlock()
		if current {
			m.send(motorStop)
		}
	})
}

// pulse moves in the direction for d, then stops (API, MCP).
func (m *mover) pulse(op int, d time.Duration) {
	m.press(op)
	time.Sleep(d)
	m.mu.Lock()
	m.since = time.Now().Add(-time.Hour) // stop right away
	m.mu.Unlock()
	m.release()
}

// holdButton - button that reports press and release (moves the camera while held)
type holdButton struct {
	widget.Button
	tip                string
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
	tipOut()
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
			if !iconified || !wmOnCurrentDesktop(w) {
				return
			}
			// the WM also reports a short minimize when it re-maps the window (frame on/off,
			// moving a window without frame): only a window still minimized is hidden
			time.AfterFunc(700*time.Millisecond, func() {
				fyne.Do(func() {
					if win.GetAttrib(glfw.Iconified) == glfw.True {
						f()
					}
				})
			})
		})
	}
}

// onCursorLeave calls f when the mouse cursor leaves the window (GLFW cursor enter
// callback, not used by Fyne).
func onCursorLeave(w fyne.Window, f func()) {
	if win := glfwWindow(w); win != nil {
		win.SetCursorEnterCallback(func(_ *glfw.Window, entered bool) {
			if !entered {
				f()
			}
		})
	}
}

// moveHandle - drag it to move the window (the window has no title bar in video-only mode)
type moveHandle struct {
	widget.Button
	tip string
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

var moveIcon = lucide("move")

// Clickable widgets with the pointer (hand) cursor on hover; Fyne uses the default arrow.

type ptrButton struct {
	widget.Button
	tip string // tooltip

	busyStop chan struct{} // action in progress (busy.go)
	busyIcon fyne.Resource // icon shown again when the action ends
}

func newButton(label string, icon fyne.Resource, tapped func()) *ptrButton {
	b := &ptrButton{}
	b.Text, b.Icon, b.OnTapped = label, icon, tapped
	b.ExtendBaseWidget(b)
	return b
}

type ptrSelect struct {
	widget.Select
	tip string
}

func (*ptrSelect) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newSelect(options []string, changed func(string)) *ptrSelect {
	s := &ptrSelect{}
	s.Options, s.OnChanged = options, changed
	s.ExtendBaseWidget(s)
	return s
}

type ptrCheck struct {
	widget.Check
	tip string
}

func (*ptrCheck) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newCheck(label string, changed func(bool)) *ptrCheck {
	c := &ptrCheck{}
	c.Text, c.OnChanged = label, changed
	c.ExtendBaseWidget(c)
	return c
}

type ptrSlider struct {
	widget.Slider
	tip string
}

func (*ptrSlider) Cursor() desktop.Cursor { return desktop.PointerCursor }

func newSlider(min, max float64) *ptrSlider {
	s := &ptrSlider{}
	s.Min, s.Max, s.Step = min, max, 1
	s.Orientation = widget.Horizontal
	s.ExtendBaseWidget(s)
	return s
}
