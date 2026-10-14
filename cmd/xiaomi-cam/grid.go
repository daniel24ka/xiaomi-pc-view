package main

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Grid view: all cameras at once. Click a tile to select the camera (all controls work on
// the selected one), double click to open it alone. Sound plays from the selected camera.

const prefLayout = "layout" // "grid" or "" (one camera)

type tile struct {
	name   string
	view   *videoView
	border *canvas.Rectangle
	player player
	obj    fyne.CanvasObject
}

var selectedColor = color.NRGBA{R: 0xff, G: 0x67, B: 0x00, A: 0xff}

func newTile(u *ui, name string) *tile {
	t := &tile{name: name, view: newVideoView()}
	t.border = canvas.NewRectangle(color.Transparent)
	t.border.StrokeWidth = 3
	label := canvas.NewText(" "+name+" ", color.White)
	label.TextSize = 12
	labelBg := canvas.NewRectangle(color.NRGBA{A: 120})
	t.view.onTap = func() { u.selectTile(name) }
	t.view.onDoubleTap = func() {
		u.selectTile(name)
		u.setGrid(false)
	}
	t.view.onScroll = u.video.onScroll // camera zoom of the selected camera
	t.view.onSecondary = u.video.onSecondary
	t.view.onMouseMove = u.video.onMouseMove
	t.player.onFrame = func(img *image.RGBA, gen int64) {
		fyne.Do(func() {
			if gen == t.player.gen.Load() {
				t.view.setFrame(img)
			}
		})
	}
	t.player.onStatus = func(s string) { fyne.Do(func() { t.view.setStatus(s) }) }
	t.obj = container.NewStack(t.view, t.border,
		container.NewVBox(container.NewHBox(container.NewStack(labelBg, label))))
	return t
}

// start plays the tile's camera; seamless keeps the current session (sound change).
func (u *ui) startTile(t *tile, seamless bool) {
	src := cameraSource(t.name)
	q := qualities[1]
	for _, qq := range qualities {
		if qq.subtype == sourceParam(src, "subtype") {
			q = qq
		}
	}
	// tiles are small: decode to 640 px (1280 px when there is one column)
	w := 640
	if len(u.tiles) <= 1 {
		w = 1280
	}
	w = min(w, q.w)
	h := w * q.h / q.w
	audio := u.sound.Checked && t.name == u.camera.Selected
	if seamless {
		go t.player.switchTo(rtspURL(t.name), w, h, audio)
	} else {
		go t.player.start(rtspURL(t.name), w, h, audio)
	}
}

func (u *ui) tileOf(name string) *tile {
	for _, t := range u.tiles {
		if t.name == name {
			return t
		}
	}
	return nil
}

func (u *ui) stopTiles() {
	for _, t := range u.tiles {
		go t.player.stop()
	}
}

// setGrid switches between the grid and one camera.
func (u *ui) setGrid(on bool) {
	if on && len(cameraNames()) < 2 {
		on = false
	}
	if on && u.hist.on {
		u.goLive() // history works with one camera
	}
	u.app.Preferences().SetString(prefLayout, map[bool]string{true: "grid", false: ""}[on])
	if on == u.grid && (!on || len(u.tiles) == len(cameraNames())) {
		return
	}
	u.grid = on
	u.updateGridButton()
	if !on {
		u.stopTiles()
		u.tiles = nil
		u.videoArea.Objects = []fyne.CanvasObject{u.video, u.exitView}
		u.videoArea.Refresh()
		u.restart()
		return
	}
	u.player.stop()
	u.tiles = nil
	var objs []fyne.CanvasObject
	for _, name := range cameraNames() {
		t := newTile(u, name)
		u.tiles = append(u.tiles, t)
		objs = append(objs, t.obj)
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(u.tiles)))))
	u.videoArea.Objects = []fyne.CanvasObject{container.NewGridWithColumns(cols, objs...), u.exitView}
	u.videoArea.Refresh()
	u.highlightTile()
	if u.visible {
		for _, t := range u.tiles {
			u.startTile(t, false)
		}
	}
	u.status.SetText(T("Siatka kamer – kliknij kamerę, aby nią sterować; podwójne kliknięcie – pełny widok"))
}

// selectTile makes the camera the selected one (controls, sound).
func (u *ui) selectTile(name string) {
	prev, _ := u.current.Load().(string)
	if name != prev {
		u.syncing = true
		u.camera.SetSelected(name)
		u.syncing = false
		u.cameraSelected(name)
		if u.sound.Checked { // sound follows the selected camera
			if t := u.tileOf(prev); t != nil {
				u.startTile(t, true)
			}
			if t := u.tileOf(name); t != nil {
				u.startTile(t, true)
			}
		}
	}
	u.highlightTile()
}

func (u *ui) highlightTile() {
	for _, t := range u.tiles {
		if t.name == u.camera.Selected {
			t.border.StrokeColor = selectedColor
		} else {
			t.border.StrokeColor = color.Transparent
		}
		t.border.Refresh()
	}
}

// cameraSelected updates the controls for the selected camera (without restarting video).
func (u *ui) cameraSelected(name string) {
	u.syncControls(name)
	u.camZoom = currentZoom(name)
	u.showZoom(u.camZoom)
	go u.loadCameraState()
	u.updateCaps()
	u.loadPoints()
}

// updateGridButton - the grid button is shown with 2 or more cameras.
func (u *ui) updateGridButton() {
	if u.gridButton == nil {
		return
	}
	if len(cameraNames()) < 2 {
		u.gridButton.Hide()
		return
	}
	u.gridButton.Show()
	if u.grid {
		u.gridButton.SetIcon(theme.ViewFullScreenIcon()) // back to one camera
		u.gridButton.Importance = widget.HighImportance
	} else {
		u.gridButton.SetIcon(theme.GridIcon())
		u.gridButton.Importance = widget.MediumImportance
	}
	u.gridButton.Refresh()
}

// camerasChanged - cameras were added, renamed or removed.
func (u *ui) camerasChanged() {
	u.camera.Options = cameraNames()
	u.camera.Refresh()
	u.updateGridButton()
	if u.grid {
		u.stopTiles()
		u.tiles = nil
		u.grid = false
		u.setGrid(true)
	}
}
