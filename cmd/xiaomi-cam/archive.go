package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// The PC archive: a recording service next to the app (cam-archive) keeps the on-air camera
// for a week in ~12 GB - the last hours in full quality, the minutes around sirens and
// detections kept in full quality, the rest as a 720p overview - and serves it over HTTP.
// The history bar plays it like the SD card, but without the camera: the camera's one
// session stays live for the broadcast while the past is watched, and the archive plays
// when the camera is offline. The recorder is a separate program so the archive has no
// gaps while the app restarts or updates.
//
// The service API: GET /api/range?from&to (coverage + marks), /api/clip?t&dur (an MP4 of
// the segments from t, copied), /api/thumb?t, /api/export?from&to, POST /api/pin.
//
// It is off until switched on in the settings (Settings -> General -> PC archive): without
// it the app never asks the service.

const (
	prefArchive    = "archive" // the PC archive is used (off by default)
	prefArchiveURL = "archive_url"
	archiveClipLen = 600                // seconds asked per clip; the next one follows at its end
	archiveWeek    = 8 * 24 * time.Hour // what the timeline loads
	saveBefore     = 30 * time.Second   // "save to a file": from a bit before the moment...
	saveAfter      = 150 * time.Second  // ...to a few minutes after
	keepAround     = 5 * time.Minute    // "keep in full quality": around the moment
)

// the marks of the archive on the timeline (event kinds like the camera's events)
const (
	eventSiren  eventKind = "archive-siren"
	eventSky    eventKind = "archive-sky"
	eventPinned eventKind = "archive-pin"
)

var archiveKindOf = map[string]eventKind{"siren": eventSiren, "event": eventSky, "manual": eventPinned}

var archiveKinds = []eventKind{eventSiren, eventSky, eventPinned}

const archiveDefault = "http://127.0.0.1:8114" // the service on this computer

var archiveHTTP = &http.Client{Timeout: 90 * time.Second}

var errArchiveOff = errors.New("the PC archive is off")

func archiveOn() bool { return appPrefs != nil && appPrefs.Bool(prefArchive) }

func archiveURL() string {
	if appPrefs != nil {
		if s := appPrefs.String(prefArchiveURL); s != "" {
			return strings.TrimRight(s, "/")
		}
	}
	return archiveDefault
}

func archiveGet(path string, q url.Values) (*http.Response, error) {
	res, err := archiveHTTP.Get(archiveURL() + path + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		var e struct{ Error string }
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return nil, errors.New(e.Error)
	}
	return res, nil
}

func unixf(f float64) time.Time { return time.Unix(0, int64(f*1e9)) }

type archSpan struct{ a, b time.Time }

type archMark struct {
	at, start, end time.Time
	kind           eventKind
	label          string
}

// archive - what the app knows of the PC archive and what it plays from it
type archive struct {
	mu       sync.Mutex
	camera   string // the camera the service records ("": no service)
	full     []archSpan
	overview []archSpan
	marks    []archMark
	loaded   time.Time

	// the playback (UI thread)
	seq       int       // the pick being loaded: a later pick wins
	clipStart time.Time // the clip played: its start and end, the position it began at
	clipEnd   time.Time
	from      time.Time
	tier      string
	retried   bool      // a failed start is tried once more
	active    bool      // a clip is playing (its frames count)
	frames    int       // the frames of this clip shown (the position)
	oldGen    int64     // the playback replaced by this clip: its late frames don't count
	noteUntil time.Time // a message (saved, kept) stays in the status line until then
	paused    bool
	pausedAt  time.Time
	fetching  map[int64]bool // minutes whose thumbnail is being fetched
}

// refresh loads the coverage and the marks of the last week (~10 ms on the same computer).
func (a *archive) refresh() error {
	now := time.Now()
	q := url.Values{"from": {fmt.Sprint(now.Add(-archiveWeek).Unix())}, "to": {fmt.Sprint(now.Add(time.Minute).Unix())}}
	res, err := archiveGet("/api/range", q)
	if err != nil {
		a.mu.Lock()
		a.camera = ""
		a.mu.Unlock()
		return err
	}
	defer res.Body.Close()
	var r struct {
		Camera   string       `json:"camera"`
		Full     [][2]float64 `json:"full"`
		Overview [][2]float64 `json:"overview"`
		Marks    []struct {
			At, Start, End float64
			Kind, Label    string
		} `json:"marks"`
	}
	if err = json.NewDecoder(res.Body).Decode(&r); err != nil {
		return err
	}
	spans := func(in [][2]float64) []archSpan {
		out := make([]archSpan, 0, len(in))
		for _, s := range in {
			out = append(out, archSpan{unixf(s[0]), unixf(s[1])})
		}
		return out
	}
	var marks []archMark
	for _, m := range r.Marks {
		if k, ok := archiveKindOf[m.Kind]; ok {
			marks = append(marks, archMark{unixf(m.At), unixf(m.Start), unixf(m.End), k, m.Label})
		}
	}
	a.mu.Lock()
	a.camera, a.full, a.overview, a.marks, a.loaded = r.Camera, spans(r.Full), spans(r.Overview), marks, now
	a.mu.Unlock()
	return nil
}

// records reports whether the archive has the camera.
func (a *archive) records(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.camera != "" && strings.TrimSpace(a.camera) == strings.TrimSpace(name)
}

// spans - the recordings between from and to for the timeline (dim: the 720p overview)
func (a *archive) spans(from, to time.Time) []tlSpan {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []tlSpan
	for _, l := range []struct {
		list []archSpan
		dim  bool
	}{{a.overview, true}, {a.full, false}} {
		for _, s := range l.list {
			if s.b.After(from) && s.a.Before(to) {
				out = append(out, tlSpan{s.a, s.b, l.dim})
			}
		}
	}
	return out
}

func (a *archive) events(from, to time.Time) []xccore.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []xccore.Event
	for _, m := range a.marks {
		if !m.at.Before(from) && m.at.Before(to) {
			out = append(out, xccore.Event{Time: m.at, Kind: m.kind})
		}
	}
	return out
}

// markLabel - the text of the mark at t (the cities of a siren, what the AI saw)
func (a *archive) markLabel(t time.Time) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range a.marks {
		if d := t.Sub(m.at); d >= 0 && d < time.Minute {
			return m.label
		}
	}
	return ""
}

// days - the days of a month with recordings, and the oldest recording
func (a *archive) days(month time.Time) (map[int]bool, time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	days := map[int]bool{}
	var oldest time.Time
	for _, list := range [][]archSpan{a.overview, a.full} {
		for _, s := range list {
			if oldest.IsZero() || s.a.Before(oldest) {
				oldest = s.a
			}
			for d := dayStart(s.a); d.Before(s.b); d = d.AddDate(0, 0, 1) {
				if d.Year() == month.Year() && d.Month() == month.Month() {
					days[d.Day()] = true
				}
			}
		}
	}
	return days, oldest
}

// --- the history bar with the PC archive

// setHistorySource switches the history bar between the PC archive and the SD card.
func (u *ui) setHistorySource(pc bool) {
	h := &u.hist
	h.pc = pc
	h.arch.paused = false
	if pc {
		h.source.SetIcon(theme.ComputerIcon())
		h.source.tip = T("Nagrania na komputerze (tydzień): kliknij, by przejść na kartę SD kamery")
	} else {
		h.source.SetIcon(theme.StorageIcon())
		h.source.tip = T("Nagrania z karty SD kamery: kliknij, by przejść na nagrania na komputerze")
	}
	for _, o := range h.pcOnly {
		if pc {
			o.Show()
		} else {
			o.Hide()
		}
	}
	h.pause.SetIcon(theme.MediaPauseIcon())
	showEvents := pc || len(cameraEvents(u.camera.Selected)) > 0
	for _, o := range h.eventBtns {
		if showEvents {
			o.Show()
		} else {
			o.Hide()
		}
	}
	h.kinds = nil
	h.tl.Refresh()
}

// openArchiveHistory decides the source of the history (the archive when it has the camera)
// and starts playing; the service is asked in the background.
func (u *ui) openArchiveHistory(t time.Time) {
	name := u.camera.Selected
	go func() {
		err := errArchiveOff // the SD card only
		if archiveOn() {
			err = u.hist.arch.refresh()
		}
		fyne.Do(func() {
			h := &u.hist
			if !h.on || name != u.camera.Selected {
				return
			}
			pc := err == nil && h.arch.records(name)
			// the source button only when there is a choice
			if pc && u.caps.History {
				h.source.Show()
			} else {
				h.source.Hide()
			}
			u.setHistorySource(pc)
			if pc {
				u.playArchive(t)
			} else {
				u.playSD(t)
			}
		})
	}()
}

// playArchive plays the PC archive from t: asks the service for the clip with t (the best
// quality there is) and plays it from t; at its end the next clip follows.
func (u *ui) playArchive(t time.Time) {
	h := &u.hist
	a := &h.arch
	name := u.camera.Selected
	if t.After(time.Now()) {
		t = time.Now().Add(-30 * time.Second)
	}
	u.syncing = true
	u.setHistoryTime(t)
	u.syncing = false
	h.target, h.firstFrame = t, time.Time{}
	a.paused, a.frames, a.active = false, 0, false
	h.pause.SetIcon(theme.MediaPauseIcon())
	a.seq++
	seq := a.seq
	speed := float64(playbackSpeeds[max(h.speed.SelectedIndex(), 0)])
	u.video.setStatus(T("Wczytuję nagranie…"))
	go func() {
		// next: from a gap in the recording, the recording after it
		q := url.Values{"t": {fmt.Sprintf("%.3f", float64(t.UnixNano())/1e9)}, "dur": {fmt.Sprint(archiveClipLen)}, "next": {"1"}}
		var clip struct {
			URL                string
			Tier               string
			Start, End, Offset float64
		}
		res, err := archiveGet("/api/clip", q)
		if err == nil {
			err = json.NewDecoder(res.Body).Decode(&clip)
			res.Body.Close()
		}
		fyne.Do(func() {
			if seq != a.seq || !h.on || !h.pc || name != u.camera.Selected {
				return // another pick, or the history was left
			}
			if err != nil {
				go u.player.stop()
				u.video.clear(T("Brak nagrania na komputerze w tym czasie"), false)
				u.status.SetText(T("Brak nagrania na komputerze w tym czasie"))
				return
			}
			a.clipStart, a.clipEnd, a.tier = unixf(clip.Start), unixf(clip.End), clip.Tier
			a.from = a.clipStart.Add(time.Duration(clip.Offset * float64(time.Second)))
			if a.from.Sub(t) > 5*time.Second {
				u.archiveNote(fmt.Sprintf(T("Brak nagrania o %s – od następnego: %s"), t.Format("15:04:05"), a.from.Format("15:04:05")))
				u.setHistoryTime(a.from)
			}
			// the decoded size: the camera's stream (full quality) or the 720p overview
			w, ht := currentQuality(name).w, currentQuality(name).h
			if clip.Tier != "full" || w == 0 {
				w, ht = 1280, 720
			}
			dw, dh := u.decodeSize(w, ht)
			u.decodeW = dw
			u.setEnhance()
			a.frames, a.oldGen, a.active = 0, u.player.gen.Load(), true
			go u.player.startFile(archiveURL()+clip.URL, clip.Offset, speed, dw, dh)
		})
	}()
}

// archiveEnded - the player finished a clip (player goroutine): on to the next one.
func (u *ui) archiveEnded(gen int64, err error) {
	fyne.Do(func() {
		h := &u.hist
		a := &h.arch
		if !h.on || !h.pc || a.paused || gen != u.player.gen.Load() {
			return
		}
		if err != nil {
			if !a.retried { // e.g. the GPU path failed: the player falls back to the CPU
				a.retried = true
				u.playArchive(u.archivePosition())
				return
			}
			u.status.SetText(TJ("Błąd odtwarzania nagrania: ", err.Error()))
			return
		}
		a.retried = false
		if next := a.clipEnd.Add(500 * time.Millisecond); next.Before(time.Now().Add(-5 * time.Second)) {
			u.playArchive(next)
		} else {
			u.goLive() // caught up with the present
		}
	})
}

// archivePosition - the time played: where the clip began plus the time played at its speed.
func (u *ui) archivePosition() time.Time {
	h := &u.hist
	a := &h.arch
	if a.paused {
		return a.pausedAt
	}
	if !a.active || a.frames == 0 {
		return time.Time{}
	}
	// the frames shown: fileFPS a second of the playback, each `speed` times longer
	speed := float64(playbackSpeeds[max(h.speed.SelectedIndex(), 0)])
	return a.from.Add(time.Duration(float64(a.frames-1) * speed / fileFPS * float64(time.Second)))
}

// toggleArchivePause stops the picture where it is (the last frame stays) or plays on.
func (u *ui) toggleArchivePause() {
	h := &u.hist
	a := &h.arch
	if !h.on || !h.pc {
		return
	}
	if a.paused {
		u.playArchive(a.pausedAt)
		return
	}
	pos := u.archivePosition()
	if pos.IsZero() {
		return
	}
	a.paused, a.pausedAt, a.active = true, pos, false
	a.seq++ // a clip still loading doesn't start
	go u.player.stop()
	h.pause.SetIcon(theme.MediaPlayIcon())
	h.tl.setPos(pos)
	h.label.SetText(pos.Format("15:04:05"))
	u.status.SetText(fmt.Sprintf(T("Pauza: %s"), pos.Format("02.01 15:04:05")))
}

// seekArchive moves the playback by d (the arrow keys in the history).
func (u *ui) seekArchive(d time.Duration) {
	pos := u.archivePosition()
	if pos.IsZero() {
		pos = u.historyTime()
	}
	u.playArchive(pos.Add(d))
}

// showArchiveMenu - saving the moment: a file, or full quality kept for the week.
func (u *ui) showArchiveMenu(b fyne.CanvasObject) {
	pos := u.archivePosition()
	if pos.IsZero() {
		pos = u.historyTime()
	}
	items := []*fyne.MenuItem{
		fyne.NewMenuItemWithIcon(TM("Zapisz 3 minuty od tej chwili do pliku MP4"), theme.DownloadIcon(), func() { u.saveArchiveClip(pos) }),
		fyne.NewMenuItemWithIcon(TM("Zachowaj w pełnej jakości na tydzień (10 minut wokół tej chwili)"), theme.ContentAddIcon(), func() { u.keepArchive(pos) }),
		fyne.NewMenuItemWithIcon(TM("Zdjęcie tej klatki"), theme.MediaPhotoIcon(), u.archiveSnapshot),
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	pop := widget.NewPopUpMenu(fyne.NewMenu("", items...), c)
	p := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	pop.ShowAtPosition(fyne.NewPos(max(p.X+b.Size().Width-pop.MinSize().Width, 0), max(p.Y-pop.MinSize().Height-4, 0)))
}

// saveArchiveClip saves the recording around pos to the recordings folder.
func (u *ui) saveArchiveClip(pos time.Time) {
	name := u.camera.Selected
	from, to := pos.Add(-saveBefore), pos.Add(saveAfter)
	u.archiveNote(T("Zapisuję nagranie…"))
	go func() {
		path, err := func() (string, error) {
			q := url.Values{"from": {fmt.Sprint(from.Unix())}, "to": {fmt.Sprint(to.Unix())}}
			res, err := archiveGet("/api/export", q)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			dir := recordingsDir()
			if err = os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			path := filepath.Join(dir, archiveFileName(name, from, "mp4"))
			f, err := os.Create(path)
			if err != nil {
				return "", err
			}
			if _, err = io.Copy(f, res.Body); err != nil {
				f.Close()
				os.Remove(path)
				return "", err
			}
			return path, f.Close()
		}()
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, u.win)
				u.status.SetText("")
				return
			}
			u.archiveNote(TJ("Zapisano ", filepath.Base(path)))
		})
	}()
}

// archiveNote shows a message in the status line for a while (the playback's own status
// waits).
func (u *ui) archiveNote(text string) {
	u.status.SetText(text)
	u.hist.arch.noteUntil = time.Now().Add(5 * time.Second)
}

// keepArchive keeps the minutes around pos in full quality for the week (while they are
// still in full quality: the last hours).
func (u *ui) keepArchive(pos time.Time) {
	from, to := pos.Add(-keepAround), pos.Add(keepAround)
	go func() {
		q := url.Values{"start": {fmt.Sprint(from.Unix())}, "end": {fmt.Sprint(to.Unix())}, "at": {fmt.Sprint(pos.Unix())},
			"label": {TN("Zachowane ręcznie")}}
		res, err := archiveHTTP.PostForm(archiveURL()+"/api/pin?"+q.Encode(), nil)
		var r struct {
			OK              bool    `json:"ok"`
			Error           string  `json:"error"`
			KeptIfNewerThan float64 `json:"kept_if_newer_than"`
		}
		if err == nil {
			_ = json.NewDecoder(res.Body).Decode(&r)
			res.Body.Close()
			if !r.OK {
				err = errors.New(r.Error)
			}
		}
		_ = u.hist.arch.refresh()
		fyne.Do(func() {
			switch {
			case err != nil:
				dialog.ShowError(err, u.win)
			case pos.Before(unixf(r.KeptIfNewerThan)):
				u.archiveNote(T("Zaznaczono, ale ta chwila jest już tylko w przeglądzie 720p (pełna jakość: ostatnie godziny)"))
			default:
				u.archiveNote(T("Zachowane: te 10 minut zostaną w pełnej jakości na tydzień"))
			}
			u.hist.tl.Refresh()
		})
	}()
}

// archiveSnapshot saves the frame shown (the decoded picture) to the photos folder.
func (u *ui) archiveSnapshot() {
	f := u.video.frame
	if f == nil {
		return
	}
	pos := u.archivePosition()
	img := image.NewRGBA(f.Rect)
	copy(img.Pix, f.Pix)
	name := u.camera.Selected
	go func() {
		dir := snapshotsDir()
		err := os.MkdirAll(dir, 0o755)
		path := filepath.Join(dir, archiveFileName(name, pos, "jpg"))
		if err == nil {
			var w *os.File
			if w, err = os.Create(path); err == nil {
				err = jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
				if cerr := w.Close(); err == nil {
					err = cerr
				}
			}
		}
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.archiveNote(TJ("Zapisano ", filepath.Base(path)))
		})
	}()
}

// archiveFileName - "<camera>_2026-10-23_05-40-12_archiwum.<ext>"
func archiveFileName(camera string, t time.Time, ext string) string {
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(camera))
	return fmt.Sprintf("%s_%s_%s.%s", safe, t.Format("2006-01-02_15-04-05"), TN("archiwum"), ext) // TN: no bidi marks in a file name
}

// archiveThumb fetches the thumbnail of a minute from the archive into the thumbnail cache
// (in the background), then shows the preview again if the mouse is still there.
func (u *ui) archiveThumb(t time.Time) {
	a := &u.hist.arch
	did := cameraDid(cameraSource(u.camera.Selected))
	minute := t.Unix() / 60
	if did == "" || hasThumb(did, minute) || a.fetching[minute] {
		return
	}
	if a.fetching == nil {
		a.fetching = map[int64]bool{}
	}
	a.fetching[minute] = true
	go func() {
		res, err := archiveGet("/api/thumb", url.Values{"t": {fmt.Sprint(minute*60 + 30)}})
		if err == nil {
			data, rerr := io.ReadAll(res.Body)
			res.Body.Close()
			if rerr == nil {
				addThumbFile(did, minute, data)
			}
		}
		fyne.Do(func() {
			delete(a.fetching, minute)
			if p := u.histPreview; p != nil && err == nil && u.hist.tl.hover.Unix()/60 == minute {
				u.showHistoryPreview(p.lastT, p.lastX)
			}
		})
	}()
}
