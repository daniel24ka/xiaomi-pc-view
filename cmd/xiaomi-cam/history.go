package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AlexxIT/go2rtc/internal/xccore"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

// History - playback of SD card recordings, like the timeline in Mi Home, or of the PC
// archive (archive.go) when it records the camera.
// The camera records ~1 min files; playback starts at the beginning of the file with the
// chosen time (up to a minute earlier), the date/time printed by the camera is exact.

var playbackSpeeds = []int{1, 2, 4, 8, 16}

type history struct {
	on     bool
	bar    *fyne.Container
	date   *datePicker
	tl     *timeline // the time axis with the scale and the recordings
	sel    time.Time // the chosen time
	label  *widget.Label
	speed  *ptrSelect
	button *ptrButton // "Historia" / "Na żywo" in the top bar

	target     time.Time // requested playback time
	firstFrame time.Time // when the first frame of this playback arrived
	listed     bool      // the SD card file list was read in this history session
	listing    bool

	kinds     map[eventKind]bool  // the shown events (nil: all)
	eventBtns []fyne.CanvasObject // the event filter and jumps (cameras with events)

	pc     bool                // playing the PC archive, not the SD card
	arch   archive             // the PC archive (archive.go)
	source *ptrButton          // switches between the two (when the camera has both)
	pause  *ptrButton          // the PC archive: pause / play
	pcOnly []fyne.CanvasObject // the controls of the PC archive
}

func (u *ui) buildHistory() {
	h := &u.hist
	h.date = newDatePicker(func(time.Time) {
		if !u.syncing {
			u.playHistory(u.historyTime())
		}
	})

	h.date.recorded = func(month time.Time) (map[int]bool, time.Time) {
		if h.pc {
			return h.arch.days(month)
		}
		did := cameraDid(cameraSource(u.camera.Selected))
		oldest, _, ok := miss.RecordingsSpan(did)
		if !ok {
			return nil, time.Time{}
		}
		days := map[int]bool{}
		for _, f := range miss.RecordingsBetween(did, month.Unix(), month.AddDate(0, 1, 0).Unix()) {
			if t := time.Unix(f.Start, 0); t.Month() == month.Month() {
				days[t.Day()] = true
			}
		}
		return days, time.Unix(oldest, 0)
	}

	h.tl = newTimeline()
	h.label = widget.NewLabel("")
	h.tl.files = func(from, to time.Time) []tlSpan {
		if h.pc {
			return h.arch.spans(from, to)
		}
		var out []tlSpan
		for _, f := range miss.RecordingsBetween(cameraDid(cameraSource(u.camera.Selected)), from.Unix(), to.Unix()) {
			out = append(out, tlSpan{from: time.Unix(f.Start, 0), to: time.Unix(f.Start+f.Duration, 0)})
		}
		return out
	}
	h.tl.events = u.historyEvents
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
			from := u.historyPosition() // on from what is being played
			if from.IsZero() {
				from = u.historyTime()
			}
			u.playHistory(from)
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
	var filter *ptrButton
	filter = newButton("", lucide("filter"), func() { u.showEventFilter(filter) }).
		withTip(T("Pokazywane zdarzenia: wszystkie albo wybrane rodzaje"))
	prevEv := newButton("", lucide("step-back"), func() { u.jumpEvent(-1) }).withTip(T("Poprzednie zdarzenie"))
	nextEv := newButton("", lucide("step-forward"), func() { u.jumpEvent(1) }).withTip(T("Następne zdarzenie"))
	h.eventBtns = []fyne.CanvasObject{filter, prevEv, nextEv}
	h.source = newButton("", theme.ComputerIcon(), func() {
		u.setHistorySource(!h.pc)
		u.playHistory(u.historyTime())
	})
	h.source.Hide()
	h.pause = newButton("", theme.MediaPauseIcon(), u.toggleArchivePause).withTip(T("Pauza / odtwarzaj (spacja)"))
	var save *ptrButton
	save = newButton("", theme.DownloadIcon(), func() { u.showArchiveMenu(save) }).
		withTip(T("Zapisz tę chwilę: plik MP4, pełna jakość na tydzień, zdjęcie klatki"))
	h.pcOnly = []fyne.CanvasObject{h.pause, save}
	h.bar = container.NewBorder(nil, nil,
		container.NewHBox(
			h.source,
			h.date.btn,
			filter,
			newButton("", theme.MediaFastRewindIcon(), step(-10*time.Minute)).withTip(T("10 minut wstecz")),
			newButton("", theme.MediaSkipPreviousIcon(), step(-time.Minute)).withTip(T("1 minuta wstecz")),
			prevEv,
		),
		container.NewHBox(
			h.label,
			nextEv,
			newButton("", theme.MediaSkipNextIcon(), step(time.Minute)).withTip(T("1 minuta do przodu")),
			newButton("", theme.MediaFastForwardIcon(), step(10*time.Minute)).withTip(T("10 minut do przodu")),
			h.speed,
			h.pause,
			save,
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
	if d := u.hist.date.date; !sameDay(d, t) {
		t = time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
	}
	return t
}

func (u *ui) setHistoryTime(t time.Time) {
	u.hist.sel = t
	u.hist.date.setDate(t)
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
	p.lastT, p.lastX = t, x
	p.label.Text = t.Format("15:04:05")
	if e, ok := u.eventAt(t); ok {
		p.label.Text += " · " + eventName(e.Kind)
		p.label.Color = eventColor(e.Kind)
		if l := u.hist.arch.markLabel(t); u.hist.pc && l != "" {
			if r := []rune(l); len(r) > 48 {
				l = string(r[:47]) + "…"
			}
			p.label.Text += " · " + l
		}
	} else {
		p.label.Color = palette.text
	}
	p.label.Refresh()
	path := thumbNear(cameraDid(cameraSource(u.camera.Selected)), t, 2)
	if path == "" && u.hist.pc {
		u.archiveThumb(t) // the archive makes it; the preview is shown again when it comes
	}
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
	lastT time.Time // the last time shown (the archive's thumbnail comes later)
	lastX float32
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
	if h.on && h.pc {
		return u.archivePosition()
	}
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
	h.on, h.listed = true, false
	h.bar.Show()
	h.button.SetIcon(theme.MediaRecordIcon()) // "back to live"
	h.button.Importance = widget.HighImportance
	h.button.Refresh()
	u.quality.Disable()
	u.connection.Disable()
	u.video.setStatus(T("Wczytuję nagranie…"))
	u.openArchiveHistory(time.Now().Add(-5 * time.Minute)) // the PC archive or the SD card
}

func (u *ui) playHistory(t time.Time) {
	if u.hist.pc {
		u.playArchive(t)
	} else {
		u.playSD(t)
	}
}

// playSD plays the SD card of the camera from t (the camera's session switches to it).
func (u *ui) playSD(t time.Time) {
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
	if !h.pc { // the PC archive never took the camera's session
		_ = setPlayback(u.current.Load().(string), 0, 0)
	}
	h.arch.paused = false
	u.restart()
}

// historyFrame is called for every frame (UI thread)
func (u *ui) historyFrame(gen int64) {
	h := &u.hist
	if h.on && h.pc && h.arch.active && gen != h.arch.oldGen {
		h.arch.frames++ // frames of the clip just started (not of the one it replaces)
	}
	if h.on && h.firstFrame.IsZero() {
		h.firstFrame = time.Now()
		if !h.listed && !h.listing && !h.pc {
			h.listing = true
			go u.listSDFiles(u.camera.Selected)
		}
	}
}

// listSDFiles reads the list of the SD card files over the playback session (~0.5 s): the
// timeline shows all recordings, the calendar only the days with recordings, and playback
// requests the exact file starts.
func (u *ui) listSDFiles(name string) {
	files, err := sdFiles(name)
	histDebug("SD card files: %d, err %v", len(files), err)
	if err == nil {
		saveRecordings()
	}
	fyne.Do(func() {
		h := &u.hist
		h.listing = false
		if err != nil || name != u.camera.Selected {
			return
		}
		h.listed = true
		h.tl.Refresh()
		h.date.refresh()
	})
	if err == nil && len(cameraEvents(name)) > 0 {
		u.loadHistoryEvents(name)
	}
}

// loadHistoryEvents reads the events of the days on the SD card (~0.15 s a day; the past
// days only once, they are kept), today first.
func (u *ui) loadHistoryEvents(name string) {
	did := cameraDid(cameraSource(name))
	oldest, _, ok := miss.RecordingsSpan(did)
	if !ok {
		return
	}
	first := dayStart(time.Unix(oldest, 0))
	for day := dayStart(time.Now()); !day.Before(first); day = day.AddDate(0, 0, -1) {
		if evStore.complete(did, day) {
			continue
		}
		if !u.hist.on || name != u.camera.Selected {
			break
		}
		list, err := readDayEvents(name, day)
		histDebug("SD card events of %s: %d, err %v", day.Format("2006-01-02"), len(list), err)
		if err != nil {
			break
		}
		fyne.Do(u.hist.tl.Refresh)
	}
	evStore.save()
}

// historyEvents - the shown events of the selected camera (the filter of the history bar)
func (u *ui) historyEvents(from, to time.Time) []xccore.Event {
	var list []xccore.Event
	if u.hist.pc {
		list = u.hist.arch.events(from, to)
	} else {
		list = evStore.between(cameraDid(cameraSource(u.camera.Selected)), from, to)
	}
	if u.hist.kinds == nil {
		return list
	}
	out := list[:0]
	for _, e := range list {
		if u.hist.kinds[e.Kind] {
			out = append(out, e)
		}
	}
	return out
}

// eventAt - the shown event of the recording with time t
func (u *ui) eventAt(t time.Time) (xccore.Event, bool) {
	list := u.historyEvents(t.Add(-time.Minute), t.Add(time.Second))
	for i := len(list) - 1; i >= 0; i-- {
		if !list[i].Time.After(t) && t.Sub(list[i].Time) < time.Minute {
			return list[i], true
		}
	}
	return xccore.Event{}, false
}

// jumpEvent plays the previous (dir -1) or the next (1) shown event.
func (u *ui) jumpEvent(dir int) {
	from := u.historyPosition()
	if from.IsZero() {
		from = u.historyTime()
	}
	var list []xccore.Event
	if dir < 0 {
		list = u.historyEvents(from.AddDate(0, 0, -31), from.Add(-5*time.Second))
		if len(list) > 0 {
			u.playHistory(list[len(list)-1].Time)
			return
		}
	} else {
		list = u.historyEvents(from.Add(5*time.Second), time.Now())
		if len(list) > 0 {
			u.playHistory(list[0].Time)
			return
		}
	}
	u.status.SetText(T("Brak kolejnych zdarzeń"))
}

// showEventFilter - the menu of the event kinds shown on the timeline
func (u *ui) showEventFilter(b fyne.CanvasObject) {
	h := &u.hist
	all := fyne.NewMenuItem(TM("Wszystkie zdarzenia"), func() {
		h.kinds = nil
		h.tl.Refresh()
	})
	all.Checked = h.kinds == nil
	items := []*fyne.MenuItem{all, fyne.NewMenuItemSeparator()}
	kinds := cameraEvents(u.camera.Selected)
	if h.pc {
		kinds = archiveKinds
	}
	for _, k := range kinds {
		k := k
		it := fyne.NewMenuItem(eventName(k), func() {
			if h.kinds == nil {
				h.kinds = map[eventKind]bool{}
			}
			h.kinds[k] = !h.kinds[k]
			h.tl.Refresh()
		})
		it.Checked = h.kinds != nil && h.kinds[k]
		items = append(items, it)
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	pop := widget.NewPopUpMenu(fyne.NewMenu("", items...), c)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	pop.ShowAtPosition(fyne.NewPos(pos.X, max(pos.Y-pop.MinSize().Height-4, 0)))
}

// historyStatus - status line during playback, updated every second
func (u *ui) historyStatus() {
	h := &u.hist
	if !h.on || h.firstFrame.IsZero() {
		return
	}
	speed := playbackSpeeds[max(h.speed.SelectedIndex(), 0)]
	if h.pc {
		if h.arch.paused {
			return
		}
		if time.Since(h.arch.loaded) > 30*time.Second { // new marks and recordings
			h.arch.loaded = time.Now()
			go func() {
				if h.arch.refresh() == nil {
					fyne.Do(h.tl.Refresh)
				}
			}()
		}
	}
	pos := u.historyPosition()
	if h.pc && pos.IsZero() {
		return // a clip is loading (or there is none)
	}
	h.tl.setPos(pos)
	h.label.SetText(pos.Format("15:04:05"))
	if h.pc {
		if time.Now().Before(h.arch.noteUntil) {
			return // a message is shown (saved, kept)
		}
		tier := T("pełna jakość")
		if h.arch.tier != "full" {
			tier = T("przegląd 720p")
		}
		u.status.SetText(fmt.Sprintf(T("Nagranie z komputera: %s (%d×, %s)"), pos.Format("02.01 15:04:05"), speed, tier))
		return
	}
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
