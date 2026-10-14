package main

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// --- player: go2rtc RTSP -> ffmpeg -> raw RGBA frames (+ audio to PulseAudio/PipeWire)

type player struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}

	onFrame  func(img *image.RGBA)
	onStatus func(text string)
}

// start plays url with frames of given size; it reconnects until stop is called.
func (p *player) start(url string, w, h int, audio bool) {
	p.stop()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	p.mu.Lock()
	p.cancel, p.done = cancel, done
	p.mu.Unlock()

	go func() {
		defer close(done)
		for ctx.Err() == nil {
			p.onStatus(T("Łączenie z kamerą…"))
			err := p.run(ctx, url, w, h, audio)
			if ctx.Err() != nil {
				return
			}
			p.onStatus(fmt.Sprintf(T("Rozłączono (%v) – ponawiam za 3 s…"), err))
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}()
}

func (p *player) stop() {
	p.mu.Lock()
	cancel, done := p.cancel, p.done
	p.cancel, p.done = nil, nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (p *player) run(ctx context.Context, url string, w, h int, audio bool) error {
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-rtsp_transport", "tcp", "-timeout", "15000000", "-i", url,
		"-map", "0:v:0", "-vf", fmt.Sprintf("scale=%d:%d", w, h),
		"-pix_fmt", "rgba", "-f", "rawvideo", "pipe:1",
	}
	if audio {
		args = append(args, "-map", "0:a:0?", "-f", "pulse", "-name", "Xiaomi Cam", "default")
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return fmt.Errorf(T("ffmpeg: %w (zainstaluj: sudo apt install ffmpeg)"), err)
	}

	// ring of buffers, so the frame being drawn is not overwritten
	const n = 3
	var frames [n]*image.RGBA
	for i := range frames {
		frames[i] = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	r := bufio.NewReaderSize(stdout, w*h*4)
	for i := 0; ; i = (i + 1) % n {
		if _, err = io.ReadFull(r, frames[i].Pix); err != nil {
			break
		}
		p.onFrame(frames[i])
	}
	_ = cmd.Wait()
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		if lines := strings.Split(msg, "\n"); len(lines) > 0 {
			return fmt.Errorf("%s", lines[len(lines)-1])
		}
	}
	return err
}

// --- video view widget: mouse wheel - camera zoom, drag - pan/tilt, edges - window resize

type videoView struct {
	widget.BaseWidget

	img    *canvas.Image
	bg     *canvas.Rectangle
	status *canvas.Text

	frame *image.RGBA

	onDoubleTap  func()
	onSecondary  func()
	onMouseMove  func()
	onCameraDrag func(dx, dy float32) // dragging the picture moves the camera
	onScroll     func(up bool)        // mouse wheel - camera zoom

	// window resize by edges (window without frame)
	canResize   func() bool
	onEdgePress func(dir int) bool   // window manager resize, wm* direction; false - not available
	onResize    func(dx, dy float32) // fallback: resize by our own drag (right/bottom edges)
	edge        int                  // wm* direction under the mouse, -1 - none
	dragging    bool                 // drag in progress
	dragEdge    int                  // edge grabbed when the drag started
	wmGrab      bool                 // the window manager is resizing, ignore drags
}

const edgeSize = 12 // px from the window edge that grab the border

// edgeAt returns the wm* resize direction for a position, or -1.
func (v *videoView) edgeAt(pos fyne.Position) int {
	if v.canResize == nil || !v.canResize() {
		return -1
	}
	size := v.Size()
	left, right := pos.X < edgeSize, pos.X >= size.Width-edgeSize
	top, bottom := pos.Y < edgeSize, pos.Y >= size.Height-edgeSize
	switch {
	case top && left:
		return wmTopLeft
	case top && right:
		return wmTopRight
	case bottom && left:
		return wmBottomLeft
	case bottom && right:
		return wmBottomRight
	case top:
		return wmTop
	case bottom:
		return wmBottom
	case left:
		return wmLeft
	case right:
		return wmRight
	}
	return -1
}

// Cursor shows resize arrows over the edges of a window without frame.
func (v *videoView) Cursor() desktop.Cursor {
	switch v.edge {
	case wmLeft, wmRight:
		return desktop.HResizeCursor
	case wmTop, wmBottom:
		return desktop.VResizeCursor
	case wmTopLeft, wmBottomRight:
		return desktop.NWSEResizeCursor
	case wmTopRight, wmBottomLeft:
		return desktop.NESWResizeCursor
	}
	return desktop.DefaultCursor
}

func (v *videoView) MouseDown(e *desktop.MouseEvent) {
	v.wmGrab = false
	if e.Button != desktop.MouseButtonPrimary {
		return
	}
	if dir := v.edgeAt(e.Position); dir >= 0 && v.onEdgePress != nil && v.onEdgePress(dir) {
		v.wmGrab = true
	}
}

func (v *videoView) MouseUp(*desktop.MouseEvent) {}

func newVideoView() *videoView {
	v := &videoView{edge: -1}
	v.img = canvas.NewImageFromImage(nil)
	v.img.FillMode = canvas.ImageFillContain
	v.img.ScaleMode = canvas.ImageScaleSmooth
	v.bg = canvas.NewRectangle(color.Black)
	v.status = canvas.NewText("", color.Gray{Y: 200})
	v.status.Alignment = fyne.TextAlignCenter
	v.ExtendBaseWidget(v)
	return v
}

func (v *videoView) CreateRenderer() fyne.WidgetRenderer {
	return &videoRenderer{v: v}
}

// setFrame must be called from the UI goroutine.
func (v *videoView) setFrame(f *image.RGBA) {
	v.frame = f
	if v.status.Text != "" {
		v.status.Text = ""
		v.status.Refresh()
	}
	v.updateImage()
}

func (v *videoView) setStatus(text string) {
	v.status.Text = text
	v.status.Refresh()
	if text != "" && v.frame == nil {
		v.img.Image = nil
		v.img.Refresh()
	}
}

func (v *videoView) clear(text string) {
	v.frame = nil
	v.setStatus(text)
}

func (v *videoView) updateImage() {
	if v.frame == nil {
		return
	}
	v.img.Image = v.frame
	v.img.Refresh()
}

func (v *videoView) Scrolled(e *fyne.ScrollEvent) {
	if v.onScroll != nil && e.Scrolled.DY != 0 {
		v.onScroll(e.Scrolled.DY > 0)
	}
}

// Dragged moves the camera ("grab the picture") or resizes the window by its edge.
func (v *videoView) Dragged(e *fyne.DragEvent) {
	dx, dy := e.Dragged.DX, e.Dragged.DY
	if v.wmGrab {
		return
	}
	if !v.dragging {
		v.dragging = true
		v.dragEdge = v.edgeAt(e.Position.Subtract(fyne.NewPos(dx, dy)))
	}
	switch v.dragEdge { // fallback when the window manager can't resize
	case wmRight:
		v.onResize(dx, 0)
		return
	case wmBottom:
		v.onResize(0, dy)
		return
	case wmBottomRight:
		v.onResize(dx, dy)
		return
	}
	if v.onCameraDrag != nil && (dx != 0 || dy != 0) {
		v.onCameraDrag(dx, dy)
	}
}

func (v *videoView) DragEnd() {
	v.dragging = false
	v.wmGrab = false
}

func (v *videoView) TappedSecondary(*fyne.PointEvent) {
	if v.onSecondary != nil {
		v.onSecondary()
	}
}

func (v *videoView) MouseIn(e *desktop.MouseEvent) { v.MouseMoved(e) }
func (v *videoView) MouseMoved(e *desktop.MouseEvent) {
	v.edge = v.edgeAt(e.Position)
	v.mouseMoved()
}
func (v *videoView) MouseOut() {}

func (v *videoView) mouseMoved() {
	if v.onMouseMove != nil {
		v.onMouseMove()
	}
}

func (v *videoView) DoubleTapped(*fyne.PointEvent) {
	if v.onDoubleTap != nil {
		v.onDoubleTap()
	}
}

type videoRenderer struct{ v *videoView }

func (r *videoRenderer) Layout(size fyne.Size) {
	for _, o := range r.Objects() {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
	r.v.status.Move(fyne.NewPos(0, size.Height/2-10))
	r.v.status.Resize(fyne.NewSize(size.Width, 20))
}

func (r *videoRenderer) MinSize() fyne.Size { return fyne.NewSize(480, 270) }
func (r *videoRenderer) Refresh()           { canvas.Refresh(r.v) }
func (r *videoRenderer) Destroy()           {}
func (r *videoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.v.bg, r.v.img, r.v.status}
}
