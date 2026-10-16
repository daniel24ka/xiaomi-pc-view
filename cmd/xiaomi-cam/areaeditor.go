package main

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// The area of the virtual fence or of the car detection, drawn on the live picture (like
// the Mi Home plugin): a rectangle in the coordinates of the whole frame scaled to 640x360,
// written to MIoT 13.1 with the camera position as read (the camera keeps the area for its
// current position; nothing moves the camera). Drag a corner to resize, inside to move, and
// outside to draw a new rectangle.

const (
	areaW, areaH = 640, 360
	areaMin      = 16  // the smallest side (in 640x360)
	areaGrab     = 18. // px around a corner that grab it
)

// areaEditor - the rectangle over the picture (in the video area, above the controls)
type areaEditor struct {
	widget.BaseWidget
	r      [4]float32 // left, top, right, bottom in 640x360
	aspect float32    // of the picture (width / height)
	color  color.NRGBA

	grab  int // the dragged corner 0-3 (lt, rt, rb, lb), 4 - the rectangle, 5 - a new one, -1 none
	start fyne.Position
}

func newAreaEditor() *areaEditor {
	e := &areaEditor{aspect: 16. / 9, grab: -1}
	e.ExtendBaseWidget(e)
	return e
}

// picture - the rectangle of the picture in the widget (the video is drawn "contain")
func (e *areaEditor) picture() (x, y, w, h float32) {
	s := e.Size()
	w, h = s.Width, s.Height
	if w/h > e.aspect {
		w = h * e.aspect
	} else {
		h = w / e.aspect
	}
	return (s.Width - w) / 2, (s.Height - h) / 2, w, h
}

// toWidget / toArea convert between the widget and 640x360
func (e *areaEditor) toWidget(ax, ay float32) fyne.Position {
	x, y, w, h := e.picture()
	return fyne.NewPos(x+ax/areaW*w, y+ay/areaH*h)
}

func (e *areaEditor) toArea(p fyne.Position) (float32, float32) {
	x, y, w, h := e.picture()
	return min(max((p.X-x)/w*areaW, 0), areaW), min(max((p.Y-y)/h*areaH, 0), areaH)
}

func (e *areaEditor) corners() [4]fyne.Position {
	return [4]fyne.Position{e.toWidget(e.r[0], e.r[1]), e.toWidget(e.r[2], e.r[1]),
		e.toWidget(e.r[2], e.r[3]), e.toWidget(e.r[0], e.r[3])}
}

func (e *areaEditor) Dragged(ev *fyne.DragEvent) {
	if e.grab < 0 {
		e.start = ev.Position.SubtractXY(ev.Dragged.DX, ev.Dragged.DY)
		e.grab = 5
		for i, c := range e.corners() {
			if abs32(c.X-e.start.X) < areaGrab && abs32(c.Y-e.start.Y) < areaGrab {
				e.grab = i
			}
		}
		if e.grab == 5 {
			lt, rb := e.toWidget(e.r[0], e.r[1]), e.toWidget(e.r[2], e.r[3])
			if e.start.X > lt.X && e.start.X < rb.X && e.start.Y > lt.Y && e.start.Y < rb.Y {
				e.grab = 4
			}
		}
	}
	ax, ay := e.toArea(ev.Position)
	switch e.grab {
	case 0:
		e.r[0], e.r[1] = min(ax, e.r[2]-areaMin), min(ay, e.r[3]-areaMin)
	case 1:
		e.r[2], e.r[1] = max(ax, e.r[0]+areaMin), min(ay, e.r[3]-areaMin)
	case 2:
		e.r[2], e.r[3] = max(ax, e.r[0]+areaMin), max(ay, e.r[1]+areaMin)
	case 3:
		e.r[0], e.r[3] = min(ax, e.r[2]-areaMin), max(ay, e.r[1]+areaMin)
	case 4: // move, kept inside the picture
		_, _, w, h := e.picture()
		dx, dy := ev.Dragged.DX/w*areaW, ev.Dragged.DY/h*areaH
		dx = min(max(dx, -e.r[0]), areaW-e.r[2])
		dy = min(max(dy, -e.r[1]), areaH-e.r[3])
		e.r[0], e.r[2], e.r[1], e.r[3] = e.r[0]+dx, e.r[2]+dx, e.r[1]+dy, e.r[3]+dy
	case 5: // a new rectangle from the press
		sx, sy := e.toArea(e.start)
		e.r = [4]float32{min(sx, ax), min(sy, ay), max(sx, ax), max(sy, ay)}
		e.r[2], e.r[3] = min(max(e.r[2], e.r[0]+areaMin), areaW), min(max(e.r[3], e.r[1]+areaMin), areaH)
	}
	e.Refresh()
}

func (e *areaEditor) DragEnd()                { e.grab = -1 }
func (e *areaEditor) Tapped(*fyne.PointEvent) {} // not to the picture below
func (e *areaEditor) Cursor() desktop.Cursor  { return desktop.CrosshairCursor }
func (e *areaEditor) MinSize() fyne.Size      { return fyne.NewSize(0, 0) }
func (e *areaEditor) CreateRenderer() fyne.WidgetRenderer {
	r := &areaRenderer{e: e}
	r.fill = canvas.NewRectangle(color.Transparent)
	r.fill.StrokeWidth = 2
	for i := range r.dots {
		r.dots[i] = canvas.NewCircle(color.White)
	}
	r.Refresh()
	return r
}

type areaRenderer struct {
	e    *areaEditor
	fill *canvas.Rectangle
	dots [4]*canvas.Circle
}

func (r *areaRenderer) Layout(fyne.Size) { r.Refresh() }
func (r *areaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}
func (r *areaRenderer) Destroy() {}
func (r *areaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fill, r.dots[0], r.dots[1], r.dots[2], r.dots[3]}
}

func (r *areaRenderer) Refresh() {
	e := r.e
	lt, rb := e.toWidget(e.r[0], e.r[1]), e.toWidget(e.r[2], e.r[3])
	r.fill.FillColor = alpha(e.color, 0x4D) // 0.3 like Mi Home
	r.fill.StrokeColor = e.color
	r.fill.Move(lt)
	r.fill.Resize(fyne.NewSize(rb.X-lt.X, rb.Y-lt.Y))
	r.fill.Refresh()
	for i, c := range e.corners() {
		d := r.dots[i]
		d.FillColor = e.color
		d.StrokeColor, d.StrokeWidth = color.White, 2
		d.Move(c.SubtractXY(7, 7))
		d.Resize(fyne.NewSize(14, 14))
		d.Refresh()
	}
}

// --- editing in the app

// editArea shows the editor of the car area (xccore.AreaCar) or of the virtual fence over
// the live picture of the selected camera; Save writes it and turns the detection on.
func (u *ui) editArea(det *xccore.Detection, kind int) {
	name := u.camera.Selected
	if u.grid {
		u.setGrid(false)
	}
	if u.hist.on {
		u.goLive()
	}
	area := det.Fence
	title := T("Wirtualne ogrodzenie: zaznacz obszar, w którym kamera pilnuje wejścia i wyjścia ludzi")
	col := eventColor(xccore.EventFenceIn)
	if kind == xccore.AreaCar {
		area = det.Car
		title = T("Obszar wykrywania aut: zaznacz miejsce, gdzie parkuje samochód")
		col = eventColor(xccore.EventCar)
	}
	ed := u.areaEdit
	ed.color = col
	ed.r = [4]float32{80, 45, 560, 310} // Mi Home's default for an empty area
	if len(area.LeftTop) == 2 && len(area.RightBottom) == 2 &&
		(area.LeftTop[0] != area.RightBottom[0] || area.LeftTop[1] != area.RightBottom[1]) {
		ed.r = [4]float32{float32(area.LeftTop[0]), float32(area.LeftTop[1]),
			float32(area.RightBottom[0]), float32(area.RightBottom[1])}
	}
	if f := u.video.frame; f != nil && f.Rect.Dy() > 0 {
		ed.aspect = float32(f.Rect.Dx()) / float32(f.Rect.Dy())
	}

	var save, cancel *ptrButton
	finish := func() {
		u.areaBox.Hide()
		u.pictureControls.Show()
		u.showDetection()
	}
	cancel = newButton(T("Anuluj"), nil, finish)
	save = newButton(T("Zapisz obszar"), nil, func() {
		a := area
		a.Switch = true
		if a.Event == 0 {
			a.Event = 3 // both events of the area
		}
		a.LeftTop = []int{int(ed.r[0] + 0.5), int(ed.r[1] + 0.5)}
		a.RightBottom = []int{int(ed.r[2] + 0.5), int(ed.r[3] + 0.5)}
		if len(a.Pos) != 2 {
			a.Pos = []int{0, 0}
		}
		write := func() {
			done := u.busy(save, cancel)
			go func() {
				err := xccore.SetArea(name, a)
				fyne.Do(func() {
					done()
					if err != nil {
						dialog.ShowError(err, u.win)
						return
					}
					u.status.SetText(fmt.Sprintf(T("Zapisano obszar %d,%d – %d,%d"), a.LeftTop[0], a.LeftTop[1],
						a.RightBottom[0], a.RightBottom[1]))
					finish()
				})
			}()
		}
		if !area.Switch && (det.Cruise || det.Tracking) {
			dialog.ShowConfirm(T("Zapisz obszar"),
				T("Wykrywanie samochodów i wirtualne ogrodzenie działają przy nieruchomej kamerze: kamera wyłączy patrol i śledzenie ruchu. Włączyć?"),
				func(ok bool) {
					if ok {
						write()
					}
				}, u.win)
			return
		}
		write()
	})
	save.Importance = widget.HighImportance
	u.areaTitle.SetText(title)
	u.areaButtons.Objects = []fyne.CanvasObject{cancel, save}
	u.areaButtons.Refresh()
	u.pictureControls.Hide()
	u.areaBox.Show()
	ed.Refresh()
}

// buildAreaEditor - the editor and its bar, hidden (in the video area stack)
func (u *ui) buildAreaEditor() fyne.CanvasObject {
	u.areaEdit = newAreaEditor()
	u.areaTitle = widget.NewLabel("")
	u.areaTitle.Wrapping = fyne.TextWrapWord
	u.areaButtons = container.NewHBox()
	help := widget.NewLabel(T("Przeciągnij róg, aby zmienić rozmiar, środek – aby przesunąć, poza prostokątem – aby narysować nowy. Obszar dotyczy obecnego kierunku kamery."))
	help.Wrapping = fyne.TextWrapWord
	help.Importance = widget.LowImportance
	bar := pill(container.NewBorder(nil, nil, nil, u.areaButtons, container.NewVBox(u.areaTitle, help)))
	u.areaBox = container.NewStack(u.areaEdit,
		container.New(layout.NewCustomPaddedLayout(10, 0, 60, 60), container.NewVBox(bar, layout.NewSpacer())))
	u.areaBox.Hide()
	return u.areaBox
}
