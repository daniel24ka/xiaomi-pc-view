package main

import (
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2/canvas"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
	"image"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// History - playback of SD card recordings, like the timeline in Mi Home.
// The camera records ~1 min files; playback starts at the beginning of the file with the
// chosen time (up to a minute earlier), the date/time printed by the camera is exact.

var playbackSpeeds = []int{1, 2, 4, 8, 16}

type history struct {
	on     bool
	bar    *fyne.Container
	date   *widget.DateEntry
	tl     *timeline // the time axis with the scale and the recordings
	sel    time.Time // the chosen time
	label  *widget.Label
	speed  *ptrSelect
	button *ptrButton // "Historia" / "Na żywo" in the top bar

	target     time.Time // requested playback time
	firstFrame time.Time // when the first frame of this playback arrived
}

func (u *ui) buildHistory() {
	h := &u.hist
	h.date = widget.NewDateEntry()
	today := time.Now()
	h.date.SetDate(&today)
	h.date.OnChanged = func(*time.Time) {
		if !u.syncing {
			u.playHistory(u.historyTime())
		}
	}

	h.tl = newTimeline()
	h.label = widget.NewLabel("")
	h.tl.files = func(from, to time.Time) [][2]time.Time {
		var out [][2]time.Time
		for _, f := range miss.RecordingsBetween(cameraDid(cameraSource(u.camera.Selected)), from.Unix(), to.Unix()) {
			out = append(out, [2]time.Time{time.Unix(f.Start, 0), time.Unix(f.Start+f.Duration, 0)})
		}
		return out
	}
	h.tl.onPick = func(t time.Time) {
		u.win.Canvas().Unfocus()
		u.showHistoryPreview(time.Time{}, 0)
		u.playHistory(t)
	}
	h.tl.onHover = u.showHistoryPreview

	var names []string
	for _, s := range playbackSpeeds {
		names = append(names, fmt.Sprintf("%d×", s))
	}
	h.speed = newSelect(names, func(string) {
		if h.on {
			u.playHistory(u.historyTime())
		}
	})
	h.speed.SetSelectedIndex(0)
	h.speed.tip = T("Prędkość odtwarzania")

	step := func(d time.Duration) func() {
		return func() {
			from := u.historyPosition() // from what is being played
			if from.IsZero() {
				from = u.historyTime()
			}
			u.playHistory(from.Add(d))
		}
	}
	h.bar = container.NewBorder(nil, nil,
		container.NewHBox(
			container.NewGridWrap(fyne.NewSize(150, 36), h.date),
			newButton("", theme.MediaFastRewindIcon(), step(-10*time.Minute)).withTip(T("10 minut wstecz")),
			newButton("", theme.MediaSkipPreviousIcon(), step(-time.Minute)).withTip(T("1 minuta wstecz")),
		),
		container.NewHBox(
			h.label,
			newButton("", theme.MediaSkipNextIcon(), step(time.Minute)).withTip(T("1 minuta do przodu")),
			newButton("", theme.MediaFastForwardIcon(), step(10*time.Minute)).withTip(T("10 minut do przodu")),
			h.speed,
		),
		h.tl,
	)
	h.bar.Hide()

	h.button = newButton("", theme.HistoryIcon(), func() {
		if h.on {
			u.goLive()
		} else {
			u.openHistory()
		}
	})
}

// historyTime - the chosen time (the date entry moves it to another day)
func (u *ui) historyTime() time.Time {
	t := u.hist.sel
	if t.IsZero() {
		t = time.Now().Add(-5 * time.Minute)
	}
	if d := u.hist.date.Date; d != nil && (d.Year() != t.Year() || d.YearDay() != t.YearDay()) {
		t = time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
	}
	return t
}

func (u *ui) setHistoryTime(t time.Time) {
	u.hist.sel = t
	u.hist.date.SetDate(&t)
	u.hist.tl.center, u.hist.tl.browsed = t, false
	u.hist.tl.setPos(t)
	u.hist.label.SetText(t.Format("15:04"))
}

// showHistoryPreview shows the time under the mouse over the timeline, with the
// thumbnail of that minute if there is one (zero t: hide).
func (u *ui) showHistoryPreview(t time.Time, x float32) {
	p := u.histPreview
	if p == nil {
		return
	}
	if t.IsZero() {
		p.box.Hide()
		return
	}
	p.label.Text = t.Format("15:04:05")
	p.label.Refresh()
	path := thumbNear(cameraDid(cameraSource(u.camera.Selected)), t, 2)
	if path != p.path {
		p.path = path
		if path == "" {
			p.img.File = ""
			p.img.Resource = nil
			p.img.Image = nil
			p.img.Hide()
		} else {
			p.img.File = path
			p.img.Show()
		}
		p.img.Refresh()
	}
	// above the timeline, at the mouse, kept inside the picture area
	d := fyne.CurrentApp().Driver()
	ax := d.AbsolutePositionForObject(u.hist.tl).X + x - d.AbsolutePositionForObject(u.videoArea).X
	size := p.box.MinSize()
	area := u.videoArea.Size()
	p.box.Resize(size)
	p.box.Move(fyne.NewPos(min(max(ax-size.Width/2, 4), area.Width-size.Width-4), area.Height-size.Height-6))
	p.box.Show()
}

// histPreview - the box over the picture shown while the mouse is on the timeline
type histPreview struct {
	box   *fyne.Container
	img   *canvas.Image
	label *canvas.Text
	path  string
}

func (u *ui) buildHistoryPreview() fyne.CanvasObject {
	img := canvas.NewImageFromFile("")
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(thumbW, thumbH))
	img.Hide()
	label := canvas.NewText("", palette.text)
	label.TextStyle.Bold = true
	label.Alignment = fyne.TextAlignCenter
	box := pill(container.NewVBox(img, label))
	box.Hide()
	u.histPreview = &histPreview{box: box, img: img, label: label}
	return container.NewWithoutLayout(box)
}

// captureThumb keeps a thumbnail of the shown picture once a minute (live: now, history:
// the time played) for the timeline preview.
func (u *ui) captureThumb(img *image.RGBA) {
	src := cameraSource(u.camera.Selected)
	if src == "" {
		return
	}
	t := time.Now()
	if u.hist.on {
		if t = u.historyPosition(); t.IsZero() {
			return
		}
	}
	m := t.Unix() / 60
	if m == u.lastThumb {
		return
	}
	u.lastThumb = m
	saveThumb(cameraDid(src), m, img)
}

// historyPosition - the time being played: the start of the file with the chosen time
// (playback starts there) plus the time played (zero: no picture yet).
func (u *ui) historyPosition() time.Time {
	h := &u.hist
	if !h.on || h.firstFrame.IsZero() {
		return time.Time{}
	}
	start := h.target
	did := cameraDid(cameraSource(u.camera.Selected))
	for _, f := range miss.RecordingsBetween(did, h.target.Unix(), h.target.Unix()+1) {
		start = time.Unix(f.Start, 0)
	}
	speed := playbackSpeeds[max(h.speed.SelectedIndex(), 0)]
	return start.Add(time.Duration(float64(time.Since(h.firstFrame)) * float64(speed)))
}

func (u *ui) openHistory() {
	if u.grid {
		u.setGrid(false) // history works with one camera
	}
	h := &u.hist
	h.on = true
	h.bar.Show()
	h.button.SetIcon(theme.MediaRecordIcon()) // "back to live"
	h.button.Importance = widget.HighImportance
	h.button.Refresh()
	u.quality.Disable()
	u.connection.Disable()
	u.playHistory(time.Now().Add(-5 * time.Minute))
}

func (u *ui) playHistory(t time.Time) {
	h := &u.hist
	if !h.on || u.camera.Selected == "" {
		return
	}
	if t.After(time.Now()) {
		t = time.Now().Add(-time.Minute)
	}
	u.syncing = true
	u.setHistoryTime(t)
	u.syncing = false
	h.target, h.firstFrame = t, time.Time{}
	speed := playbackSpeeds[max(h.speed.SelectedIndex(), 0)]

	ts := time.Now()
	u.player.stop() // one camera session: the live/previous playback must end first
	histDebug("player stopped in %v", time.Since(ts))
	if err := setPlayback(u.camera.Selected, t.Unix(), speed); err != nil {
		u.status.SetText(err.Error())
		return
	}
	u.restart()
	u.video.setStatus(T("Szukam nagrania…"))
}

func (u *ui) goLive() {
	h := &u.hist
	if !h.on {
		return
	}
	h.on = false
	go saveRecordings()
	h.bar.Hide()
	h.button.SetIcon(theme.HistoryIcon())
	h.button.Importance = widget.MediumImportance
	h.button.Refresh()
	u.quality.Enable()
	u.connection.Enable()
	u.player.stop()
	_ = setPlayback(u.current.Load().(string), 0, 0)
	u.restart()
}

// historyFrame is called for every frame (UI thread)
func (u *ui) historyFrame() {
	if u.hist.on && u.hist.firstFrame.IsZero() {
		u.hist.firstFrame = time.Now()
	}
}

// historyStatus - status line during playback, updated every second
func (u *ui) historyStatus() {
	h := &u.hist
	if !h.on || h.firstFrame.IsZero() {
		return
	}
	speed := playbackSpeeds[max(h.speed.SelectedIndex(), 0)]
	pos := u.historyPosition()
	h.tl.setPos(pos)
	h.label.SetText(pos.Format("15:04:05"))
	u.status.SetText(fmt.Sprintf(T("Historia: %s (%d×)"), pos.Format("02.01 15:04:05"), speed))
}

// histDebug - timing of the history steps (devtest builds set it)
var histDebugf func(format string, args ...any)

func histDebug(format string, args ...any) {
	if histDebugf != nil {
		histDebugf(format, args...)
	}
}

// The SD card files found while browsing the history are kept between runs: going back to
// a time seen before needs one request to the camera (miss.Recordings).

func recordingsFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "xiaomi-cam", "recordings.json")
}

func loadRecordings() {
	b, err := os.ReadFile(recordingsFile())
	if err != nil {
		return
	}
	var all map[string][]miss.Recording
	if json.Unmarshal(b, &all) == nil {
		miss.AddRecordings(all)
	}
}

func saveRecordings() {
	path := recordingsFile()
	if path == "" {
		return
	}
	b, err := json.Marshal(miss.Recordings())
	if err != nil || len(b) < 3 {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, b, 0o600)
}
