package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// The UI of the camera events: the detection settings of the camera (☰ → Event detection),
// the list of the newest events (☰ → Events) and Settings → Events (the watcher and its
// actions).

func helpLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	rtlAlign(l)
	l.Importance = widget.LowImportance
	return l
}

// quietCheck sets a check without calling its OnChanged (an undone change).
func quietCheck(c *ptrCheck, on bool) {
	f := c.OnChanged
	c.OnChanged = nil
	c.SetChecked(on)
	c.OnChanged = f
}

func boldLabel(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

var sensitivities = []string{"Niska", "Średnia", "Wysoka"} // 5.3: 1-3

var alarmIntervals = []int{1, 3, 5, 10, 15, 30} // 5.2, minutes

// showDetection - the detection settings of the selected camera; every change is sent to
// the camera right away (like Mi Home).
func (u *ui) showDetection() {
	name := u.camera.Selected
	if name == "" || len(cameraEvents(name)) == 0 {
		return
	}
	body := container.NewVBox(widget.NewLabel(T("Czytam ustawienia kamery…")))
	d := dialog.NewCustom(fmt.Sprintf(T("Wykrywanie zdarzeń – %s"), name), T("Zamknij"),
		container.NewVScroll(body), u.win)
	d.Resize(fyne.NewSize(560, 640))
	d.Show()
	u.detectionDlg = d
	go func() {
		det, err := getDetection(name)
		fyne.Do(func() {
			if err != nil {
				body.Objects = []fyne.CanvasObject{widget.NewLabel(err.Error())}
				body.Refresh()
				return
			}
			body.Objects = u.detectionForm(name, det).Objects
			body.Refresh()
		})
	}()
}

// closeDetection hides the detection dialog (the area editor opens over the picture).
func (u *ui) closeDetection() {
	if u.detectionDlg != nil {
		u.detectionDlg.Hide()
		u.detectionDlg = nil
	}
}

func (u *ui) detectionForm(name string, det *xccore.Detection) *fyne.Container {
	kinds := map[eventKind]bool{}
	for _, k := range cameraEvents(name) {
		kinds[k] = true
	}
	// set writes a property; on an error the control goes back (undo)
	set := func(p xccore.MiotProp, value any, undo func()) {
		go func() {
			err := setProp(name, p, value)
			if code, ok := err.(xccore.CodeError); ok && code == 1 {
				err = nil // being applied
			}
			fyne.Do(func() {
				if err != nil {
					undo()
					dialog.ShowError(err, u.win)
				}
			})
		}()
	}
	check := func(label string, on bool, changed func(c *ptrCheck, on bool)) *ptrCheck {
		var c *ptrCheck
		c = newCheck(label, nil)
		c.SetChecked(on)
		c.OnChanged = func(v bool) { changed(c, v) }
		return c
	}
	// boolProp - a check of a bool property
	boolProp := func(label string, p xccore.MiotProp, on bool, apply func(bool)) *ptrCheck {
		return check(label, on, func(c *ptrCheck, v bool) {
			apply(v)
			set(p, v, func() {
				apply(!v)
				quietCheck(c, !v)
			})
		})
	}

	box := container.NewVBox()
	add := func(o ...fyne.CanvasObject) { box.Objects = append(box.Objects, o...) }

	var detection *ptrCheck
	if det.Has[xccore.PropDetection] {
		detection = boolProp(T("Wykrywanie zdarzeń włączone"), xccore.PropDetection, det.On,
			func(v bool) { det.On = v }).withTip(T("Kamera wykrywa zdarzenia i zapisuje ich rodzaj przy nagraniach na karcie"))
		add(detection)
	}
	if det.Has[xccore.PropSensitivity] {
		var names []string
		for _, s := range sensitivities {
			names = append(names, T(s))
		}
		sel := newSelect(names, nil)
		sel.SetSelectedIndex(min(max(det.Sensitivity, 1), 3) - 1)
		prev := sel.SelectedIndex()
		sel.OnChanged = func(string) {
			i := sel.SelectedIndex()
			set(xccore.PropSensitivity, i+1, func() { sel.SetSelectedIndex(prev) })
			prev = i
		}
		add(container.NewBorder(nil, nil, widget.NewLabel(T("Czułość")), nil, sel))
	}
	if det.Has[xccore.PropAlarmInterval] {
		var names []string
		at := 0
		for i, m := range alarmIntervals {
			names = append(names, fmt.Sprintf(T("%d min"), m))
			if m <= det.Interval {
				at = i
			}
		}
		sel := newSelect(names, nil)
		sel.SetSelectedIndex(at)
		prev := at
		sel.OnChanged = func(string) {
			i := sel.SelectedIndex()
			set(xccore.PropAlarmInterval, alarmIntervals[i], func() { sel.SetSelectedIndex(prev) })
			prev = i
		}
		add(container.NewBorder(nil, nil, widget.NewLabel(T("Odstęp między alarmami")), nil,
			sel.withTip(T("Najkrótszy czas między dwoma alarmami tego samego rodzaju"))))
	}

	add(widget.NewSeparator(), boldLabel(T("Rodzaje zdarzeń")))
	if det.Has[xccore.PropMotionDetect] && kinds[xccore.EventMotion] {
		add(boolProp(eventName(xccore.EventMotion), xccore.PropMotionDetect, det.Motion, func(v bool) { det.Motion = v }).
			withTip(T("Każdy ruch w obrazie")))
	}
	if det.Has[xccore.PropPersonDetect] && kinds[xccore.EventPerson] {
		add(boolProp(eventName(xccore.EventPerson), xccore.PropPersonDetect, det.Person, func(v bool) { det.Person = v }).
			withTip(T("Sylwetka człowieka w obrazie")))
	}
	var tracking, humanTrack *ptrCheck
	// areaCheck - one event (bit) of the car detection or the virtual fence (property 13.1)
	areaCheck := func(k eventKind, area *xccore.AIArea, bit int, tip string) {
		if !det.Has[xccore.PropAIArea] || !kinds[k] {
			return
		}
		add(check(eventName(k), area.Switch && area.Event&bit != 0, func(c *ptrCheck, v bool) {
			old := *area
			a := *area
			if v {
				a.Event |= bit
			} else {
				a.Event &^= bit
			}
			if a.Event == 0 {
				a.Switch = false
			} else if !a.Switch {
				a.Switch, a.Event = true, bit // like Mi Home: switching it on starts with this event
			}
			write := func() {
				*area = a
				go func() {
					err := xccore.SetArea(name, a)
					// the camera turns the patrol / tracking off: show what it did
					fresh, _ := getDetection(name)
					fyne.Do(func() {
						if err != nil {
							*area = old
							quietCheck(c, !v)
							dialog.ShowError(err, u.win)
							return
						}
						if fresh != nil {
							det.Tracking, det.HumanTrack, det.Cruise = fresh.Tracking, fresh.HumanTrack, fresh.Cruise
							if tracking != nil {
								quietCheck(tracking, fresh.Tracking)
							}
							if humanTrack != nil {
								quietCheck(humanTrack, fresh.HumanTrack)
							}
						}
					})
				}()
			}
			if v && !old.Switch && (det.Cruise || det.Tracking) {
				dialog.ShowConfirm(eventName(k),
					T("Wykrywanie samochodów i wirtualne ogrodzenie działają przy nieruchomej kamerze: kamera wyłączy patrol i śledzenie ruchu. Włączyć?"),
					func(ok bool) {
						if ok {
							write()
							return
						}
						quietCheck(c, false)
					}, u.win)
				return
			}
			write()
		}).withTip(tip))
	}
	areaCheck(xccore.EventCar, &det.Car, 1, T("Samochód w obszarze wykrywania aut"))
	areaCheck(xccore.EventNearCar, &det.Car, 2, T("Ktoś podchodzi do samochodu w obszarze wykrywania aut"))
	areaCheck(xccore.EventFenceIn, &det.Fence, 1, T("Człowiek wchodzi do wirtualnego ogrodzenia"))
	areaCheck(xccore.EventFenceOut, &det.Fence, 2, T("Człowiek wychodzi z wirtualnego ogrodzenia"))
	if det.Has[xccore.PropAIArea] {
		areaBtn := func(label string, kind int, k eventKind) fyne.CanvasObject {
			if !kinds[k] {
				return layout.NewSpacer()
			}
			return newButton(label, lucide("scan"), func() {
				u.closeDetection()
				u.editArea(det, kind)
			})
		}
		add(container.NewGridWithColumns(2,
			areaBtn(T("Obszar wykrywania aut…"), xccore.AreaCar, xccore.EventCar),
			areaBtn(T("Wirtualne ogrodzenie…"), xccore.AreaFence, xccore.EventFenceIn)))
		if det.Has[xccore.PropAreaOSD] {
			add(boolProp(T("Kamera rysuje obszary na obrazie"), xccore.PropAreaOSD, det.AreaOSD,
				func(v bool) { det.AreaOSD = v }).withTip(T("Ramka obszaru aut i wirtualnego ogrodzenia widoczna w obrazie z kamery")))
		}
		add(helpLabel(T("Obszar zaznaczasz na obrazie na żywo; kamera pilnuje go w kierunku, w którym patrzy przy zapisie. Zapis obszaru włącza jego wykrywanie.")))
	}

	if det.LightAlarm != nil {
		add(widget.NewSeparator(), boldLabel(T("Alarm dźwiękowo-świetlny")),
			helpLabel(T("Kamera włącza reflektor i syrenę, gdy wykryje:")))
		grid := container.NewGridWithColumns(2)
		for _, k := range cameraEvents(name) {
			k := k
			grid.Add(check(eventName(k), det.LightAlarm[k], func(c *ptrCheck, v bool) {
				on := map[eventKind]bool{}
				for kk, vv := range det.LightAlarm {
					on[kk] = vv
				}
				on[k] = v
				go func() {
					err := xccore.SetLightAlarm(name, det, on)
					fyne.Do(func() {
						if err != nil {
							quietCheck(c, !v)
							dialog.ShowError(err, u.win)
						}
					})
				}()
			}))
		}
		add(grid)
	}

	if det.Has[xccore.PropMotionTrack] || det.Has[xccore.PropHumanTrack] {
		add(widget.NewSeparator(), boldLabel(T("Śledzenie")))
		if det.Has[xccore.PropMotionTrack] {
			tracking = boolProp(T("Kamera obraca się za ruchem"), xccore.PropMotionTrack, det.Tracking,
				func(v bool) { det.Tracking = v })
			add(tracking)
		}
		if det.Has[xccore.PropHumanTrack] {
			humanTrack = boolProp(T("Kamera obraca się za człowiekiem"), xccore.PropHumanTrack, det.HumanTrack,
				func(v bool) { det.HumanTrack = v })
			add(humanTrack)
		}
	}
	add(widget.NewSeparator(),
		helpLabel(T("Aplikacja czyta zdarzenia z karty kamery: są na osi czasu w historii i na liście zdarzeń. Powiadomienia o nowych zdarzeniach włączysz w Ustawieniach → Zdarzenia.")))
	return box
}

// --- the events list

// showEvents - the newest events of the cameras; a click plays the recording.
func (u *ui) showEvents() {
	list := recentEvents(300)
	var d dialog.Dialog
	status := widget.NewLabel("")
	rows := widget.NewList(func() int { return len(list) },
		func() fyne.CanvasObject {
			dot := canvas.NewCircle(palette.muted)
			return container.NewBorder(nil, nil,
				container.NewCenter(container.NewGridWrap(fyne.NewSize(12, 12), dot)), widget.NewLabel(""),
				widget.NewLabel(""))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			e := list[i]
			c := o.(*fyne.Container)
			kind := c.Objects[0].(*widget.Label)
			dot := c.Objects[1].(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*canvas.Circle)
			when := c.Objects[2].(*widget.Label)
			dot.FillColor = eventColor(e.Kind)
			dot.Refresh()
			kind.SetText(eventName(e.Kind) + " – " + e.Camera)
			when.SetText(e.Time.Format("02.01 15:04:05"))
		})
	rows.OnSelected = func(i widget.ListItemID) {
		e := list[i]
		d.Hide()
		u.playEvent(e)
	}
	empty := helpLabel(T("Brak zdarzeń. Włącz wykrywanie w kamerze (☰ → Wykrywanie zdarzeń) – zdarzenia pojawią się tu po sprawdzeniu kamery (Ustawienia → Zdarzenia) albo po otwarciu historii."))
	refresh := func() {
		list = recentEvents(300)
		rows.Refresh()
		if len(list) == 0 {
			empty.Show()
		} else {
			empty.Hide()
		}
	}
	var readBtn *ptrButton
	readBtn = newButton(T("Sprawdź kamery"), lucide("refresh-cw"), func() {
		done := u.busy(readBtn)
		go func() {
			var errs []string
			for _, name := range cameraNames() {
				if len(cameraEvents(name)) == 0 {
					continue
				}
				if _, err := readDayEvents(name, time.Now()); err != nil {
					errs = append(errs, name+": "+err.Error())
				}
			}
			evStore.save()
			fyne.Do(func() {
				done()
				refresh()
				if len(errs) > 0 {
					status.SetText(errs[0])
				} else {
					status.SetText(fmt.Sprintf(T("Sprawdzono %s"), time.Now().Format("15:04:05")))
				}
			})
		}()
	}).withTip(T("Czyta dzisiejsze zdarzenia z kamer (krótkie połączenie, gdy nie oglądasz kamery)"))
	refresh()
	top := container.NewBorder(nil, nil, nil, readBtn, status)
	d = dialog.NewCustom(T("Zdarzenia"), T("Zamknij"),
		container.NewBorder(top, nil, nil, nil, container.NewStack(rows, container.NewVBox(empty))), u.win)
	d.Resize(fyne.NewSize(560, 600))
	d.Show()
}

// playEvent shows the recording of an event in the history.
func (u *ui) playEvent(e namedEvent) {
	if e.Camera != u.camera.Selected {
		u.camera.SetSelected(e.Camera)
	}
	if ipCam(e.Camera) != nil {
		return // no recordings of IP cameras: the live picture
	}
	if !u.hist.on {
		u.openHistory()
	}
	u.playHistory(e.Time)
}

// --- Settings → Events

func (u *ui) eventsSettings() fyne.CanvasObject {
	cfg := u.ai.config().Events
	save := func() {
		if err := u.ai.setEvents(cfg); err != nil {
			dialog.ShowError(err, u.win)
		}
	}
	// kinds of the cameras (all the app knows when no camera has events)
	var kinds []eventKind
	have := map[eventKind]bool{}
	for _, name := range cameraNames() {
		for _, k := range cameraEvents(name) {
			have[k] = true
		}
	}
	for _, k := range xccore.AllEventKinds {
		if have[k] {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		return container.NewVBox(helpLabel(T("Żadna z kamer nie zgłasza zdarzeń (wykrywanie ruchu, ludzi i samochodów mają kamery Xiaomi z AI, np. Outdoor Camera CW700S).")))
	}

	var intNames []string
	intervals := []int{1, 2, 5, 10, 15, 30}
	at := 0
	for i, m := range intervals {
		intNames = append(intNames, fmt.Sprintf(T("co %d min"), m))
		if float64(m) <= cfg.interval().Minutes() {
			at = i
		}
	}
	interval := newSelect(intNames, nil)
	interval.SetSelectedIndex(at)
	interval.OnChanged = func(string) {
		cfg.IntervalMin = intervals[interval.SelectedIndex()]
		save()
	}
	watchOn := newCheck(T("Sprawdzaj kamery i reaguj na nowe zdarzenia"), nil)
	watchOn.SetChecked(cfg.Watch)
	watchOn.OnChanged = func(on bool) {
		cfg.Watch = on
		if cfg.IntervalMin == 0 {
			cfg.IntervalMin = intervals[interval.SelectedIndex()]
		}
		save()
	}

	kindGrid := container.NewGridWithColumns(2)
	for _, k := range kinds {
		k := k
		c := newCheck(eventName(k), nil)
		c.SetChecked(cfg.wants(k))
		c.OnChanged = func(on bool) {
			var out []eventKind
			for _, w := range cfg.Kinds {
				if w != k {
					out = append(out, w)
				}
			}
			if on {
				out = append(out, k)
			}
			cfg.Kinds = out
			save()
		}
		kindGrid.Add(c)
	}

	action := func(label string, v *bool) *ptrCheck {
		c := newCheck(label, nil)
		c.SetChecked(*v)
		c.OnChanged = func(on bool) {
			*v = on
			save()
		}
		return c
	}
	recSec := widget.NewEntry()
	if cfg.RecordSec > 0 {
		recSec.SetText(strconv.Itoa(cfg.RecordSec))
	}
	recSec.SetPlaceHolder("60")
	recSec.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.RecordSec = n
			save()
		}
	}
	actions := container.NewGridWithColumns(2,
		action(T("Powiadomienie na pulpicie"), &cfg.Notify),
		action(T("Zapisz zdjęcie"), &cfg.SaveSnapshot),
		action(T("E-mail ze zdjęciem"), &cfg.Email),
		action(T("Slack"), &cfg.Slack),
		action(T("ntfy (telefon) ze zdjęciem"), &cfg.Ntfy),
		action(T("Telegram ze zdjęciem"), &cfg.Telegram),
		container.NewBorder(nil, nil, action(T("Nagraj film"), &cfg.Record), widget.NewLabel(T("s")), recSec),
	)

	state := widget.NewLabel("")
	state.Wrapping = fyne.TextWrapWord
	rtlAlign(state)
	showState := func() {
		watch.mu.Lock()
		defer watch.mu.Unlock()
		var lines string
		for _, name := range cameraNames() {
			if len(cameraEvents(name)) == 0 {
				continue
			}
			line := name + ": "
			if t, ok := watch.checked[name]; ok {
				line += fmt.Sprintf(T("sprawdzono %s"), t.Format("15:04:05"))
			} else {
				line += T("jeszcze nie sprawdzono")
			}
			if e := watch.errs[name]; e != "" {
				line += " – " + e
			}
			lines += line + "\n"
		}
		state.SetText(lines)
	}
	showState()
	watch.mu.Lock()
	watch.onChange = func() { fyne.Do(showState) }
	watch.mu.Unlock()

	return container.NewVScroll(container.NewVBox(
		container.NewBorder(nil, nil, nil, interval, watchOn),
		helpLabel(T("Kamera zapisuje rodzaj zdarzenia przy nagraniu na karcie. Aplikacja czyta je co kilka minut: przez otwarte połączenie, gdy oglądasz kamerę, a inaczej krótkim połączeniem bez obrazu (1–3 s; nie działa, gdy ktoś ogląda kamerę w Mi Home). Zdarzenie dociera z opóźnieniem do odstępu sprawdzania i ok. minuty.")),
		widget.NewSeparator(),
		boldLabel(T("Reaguj na")),
		kindGrid,
		widget.NewSeparator(),
		boldLabel(T("Akcje")),
		actions,
		helpLabel(T("E-mail, Slack, ntfy i Telegram ustawisz na karcie Powiadomienia. Zdjęcie do wiadomości to aktualny obraz kamery.")),
		widget.NewSeparator(),
		boldLabel(T("Stan")),
		state,
		layout.NewSpacer(),
	))
}
