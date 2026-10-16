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

const (
	prefLayout     = "layout"      // "grid" or "" (one camera): the last view
	prefStartView  = "start_view"  // "" - the grid of all cameras, "last" - the last view
	prefLastCamera = "last_camera" // the selected camera (shown in the "last" view, chosen in the grid)
)

// startInGrid - the window starts with the grid (Settings → General → View at start).
func (u *ui) startInGrid() bool {
	if u.app.Preferences().String(prefStartView) == "last" {
		return u.app.Preferences().String(prefLayout) == "grid"
	}
	return true
}

// videoAreaObjects - the layers of the video area: the picture (one camera or the grid),
// the controls on it, the area editor, the exit button of video-only mode.
func (u *ui) videoAreaObjects(picture fyne.CanvasObject) []fyne.CanvasObject {
	return []fyne.CanvasObject{picture, u.pictureControls, u.areaBox, u.exitView}
}

type tile struct {
	name   string
	view   *videoView
	border *canvas.Rectangle
	player player
	obj    fyne.CanvasObject
}

var selectedColor = palette.accent

func newTile(u *ui, name string) *tile {
	t := &tile{name: name, view: newVideoView()}
	t.view.small = true
	t.view.bg.FillColor = palette.surface // the card's color around the picture
	t.view.bg.CornerRadius = 12
	t.border = canvas.NewRectangle(color.Transparent)
	t.border.StrokeWidth = 2
	t.border.CornerRadius = 12
	// the name on the picture, in a chip with the live dot
	label := widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	label.Truncation = fyne.TextTruncateEllipsis // a narrow tile: the name is shortened
	label.SizeName = theme.SizeNameCaptionText
	dot := canvas.NewCircle(palette.live)
	nameChip := pill(container.NewBorder(nil, nil,
		container.NewCenter(container.NewGridWrap(fyne.NewSize(7, 7), dot)), nil, label))
	nameRow := container.New(&chipLayout{label: label}, nameChip)
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
				u.pictureShown(&t.player, gen)
			}
		})
	}
	t.player.onStatus = func(s string) { fyne.Do(func() { t.view.setStatus(s) }) }
	t.obj = container.New(&insetLayout{4}, container.NewStack(t.view, t.border,
		container.New(&insetLayout{8}, container.NewVBox(nameRow))))
	return t
}

// start plays the tile's camera; seamless keeps the current session (sound change).
func (u *ui) startTile(t *tile, seamless bool) {
	q := currentQuality(t.name)
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
		u.videoArea.Objects = u.videoAreaObjects(u.video)
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
	grid := container.New(&insetLayout{4}, container.NewGridWithColumns(cols, objs...))
	u.videoArea.Objects = u.videoAreaObjects(grid)
	u.videoArea.Refresh()
	u.highlightTile()
	if u.visible {
		for _, t := range u.tiles {
			u.startTile(t, false)
		}
	}
	u.status.SetText("") // the chip would cover a tile; the hint is the grid button's tooltip
	u.setLive(true)
}

// selectTile makes the camera the selected one (controls, sound).
func (u *ui) selectTile(name string) {
	prev, _ := u.current.Load().(string)
	if name != prev {
		u.app.Preferences().SetString(prefLastCamera, name)
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
