package main

import (
	"fmt"
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
	slider *ptrSlider // minutes of the day 0..1439
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

	h.slider = newSlider(0, 24*60-1).withTip(T("Godzina odtwarzania – przeciągnij i puść"))
	h.label = widget.NewLabel("")
	h.slider.OnChanged = func(v float64) { h.label.SetText(fmt.Sprintf("%02d:%02d", int(v)/60, int(v)%60)) }
	h.slider.OnChangeEnded = func(float64) {
		u.win.Canvas().Unfocus()
		u.playHistory(u.historyTime())
	}

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
		return func() { u.playHistory(u.historyTime().Add(d)) }
	}
	h.bar = container.NewBorder(nil, nil,
		container.NewHBox(
			widget.NewLabel(T("Historia:")),
			container.NewGridWrap(fyne.NewSize(150, 36), h.date),
			newButton("", theme.MediaFastRewindIcon(), step(-10*time.Minute)).withTip(T("10 minut wstecz")),
			newButton("", theme.MediaSkipPreviousIcon(), step(-time.Minute)).withTip(T("1 minuta wstecz")),
		),
		container.NewHBox(
			h.label,
			newButton("", theme.MediaSkipNextIcon(), step(time.Minute)).withTip(T("1 minuta do przodu")),
			newButton("", theme.MediaFastForwardIcon(), step(10*time.Minute)).withTip(T("10 minut do przodu")),
			widget.NewLabel(T("Prędkość:")), h.speed,
		),
		h.slider,
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

// historyTime - time chosen with the date entry and the slider
func (u *ui) historyTime() time.Time {
	d := time.Now()
	if u.hist.date.Date != nil {
		d = *u.hist.date.Date
	}
	m := int(u.hist.slider.Value)
	return time.Date(d.Year(), d.Month(), d.Day(), m/60, m%60, 0, 0, time.Local)
}

func (u *ui) setHistoryTime(t time.Time) {
	u.hist.date.SetDate(&t)
	m := float64(t.Hour()*60 + t.Minute())
	u.hist.slider.Value = m
	u.hist.slider.Refresh()
	u.hist.label.SetText(t.Format("15:04"))
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

	u.player.stop() // one camera session: the live/previous playback must end first
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
	pos := time.Duration(float64(time.Since(h.firstFrame)) * float64(speed)).Truncate(time.Second)
	u.status.SetText(fmt.Sprintf(T("Historia: od %s, +%s (%d×)"), h.target.Format("02.01 15:04"), pos, speed))
}
