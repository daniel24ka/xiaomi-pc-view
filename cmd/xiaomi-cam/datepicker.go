package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// datePicker - a button with the date and a month calendar in the app language (Fyne's
// DateEntry uses the system locale and always opens below itself: off the window in the
// bottom bar). The calendar opens above the button if there is room, inside the window;
// future days are disabled, the week starts on Monday.
type datePicker struct {
	btn       *ptrButton
	date      time.Time
	onChanged func(time.Time) // a day picked by the user (not called by setDate)

	pop   *widget.PopUp
	body  *fyne.Container
	month time.Time // the shown month (its first day)
}

var monthNames = []string{"Styczeń", "Luty", "Marzec", "Kwiecień", "Maj", "Czerwiec", "Lipiec",
	"Sierpień", "Wrzesień", "Październik", "Listopad", "Grudzień"}

var weekdayNames = []string{"Pn", "Wt", "Śr", "Cz", "Pt", "So", "Nd"}

func newDatePicker(onChanged func(time.Time)) *datePicker {
	p := &datePicker{onChanged: onChanged}
	p.btn = newButton("", theme.CalendarIcon(), p.open).withTip(T("Wybierz datę"))
	p.setDate(time.Now())
	return p
}

func (p *datePicker) setDate(t time.Time) {
	p.date = t
	p.btn.SetText(t.Format("02.01.2006"))
}

func (p *datePicker) open() {
	c := fyne.CurrentApp().Driver().CanvasForObject(p.btn)
	if c == nil {
		return
	}
	p.month = time.Date(p.date.Year(), p.date.Month(), 1, 0, 0, 0, 0, time.Local)
	p.body = container.NewVBox()
	p.pop = widget.NewPopUp(p.body, c)
	p.fill()
	p.place()
}

// place puts the calendar above the button (below if there is no room), inside the window.
func (p *datePicker) place() {
	c := fyne.CurrentApp().Driver().CanvasForObject(p.btn)
	if c == nil || p.pop == nil {
		return
	}
	size := p.pop.MinSize()
	at := fyne.CurrentApp().Driver().AbsolutePositionForObject(p.btn)
	y := at.Y - size.Height - 4
	if y < 0 {
		y = min(at.Y+p.btn.Size().Height+4, c.Size().Height-size.Height)
	}
	x := min(at.X, c.Size().Width-size.Width)
	p.pop.ShowAtPosition(fyne.NewPos(max(x, 0), max(y, 0)))
	p.pop.Resize(size)
}

// fill builds the shown month: a header with the arrows, the weekdays and the days.
func (p *datePicker) fill() {
	now := time.Now()
	next := p.month.AddDate(0, 1, 0)
	prevB := newButton("", theme.NavigateBackIcon(), func() { p.show(p.month.AddDate(0, -1, 0)) }).
		withTip(T("Poprzedni miesiąc"))
	nextB := newButton("", theme.NavigateNextIcon(), func() { p.show(next) }).withTip(T("Następny miesiąc"))
	if next.After(now) {
		nextB.Disable()
	}
	prevB.Importance, nextB.Importance = widget.LowImportance, widget.LowImportance
	title := widget.NewLabelWithStyle(fmt.Sprintf("%s %d", T(monthNames[p.month.Month()-1]), p.month.Year()),
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	header := container.NewBorder(nil, nil, prevB, nextB, title)

	days := container.NewGridWithColumns(7)
	for _, d := range weekdayNames {
		days.Add(widget.NewLabelWithStyle(T(d), fyne.TextAlignCenter, fyne.TextStyle{}))
	}
	for range (int(p.month.Weekday()) + 6) % 7 { // Monday first
		days.Add(layout.NewSpacer())
	}
	for d := p.month; d.Month() == p.month.Month(); d = d.AddDate(0, 0, 1) {
		day := d
		b := newButton(fmt.Sprint(d.Day()), nil, func() { p.pick(day) })
		switch {
		case sameDay(d, p.date):
			b.Importance = widget.HighImportance
		case sameDay(d, now):
			b.Importance = widget.MediumImportance
		default:
			b.Importance = widget.LowImportance
		}
		if d.After(now) {
			b.Disable()
		}
		days.Add(b)
	}
	today := newButton(T("Dzisiaj"), nil, func() { p.pick(now) })
	today.Importance = widget.LowImportance
	p.body.Objects = []fyne.CanvasObject{header, days, container.NewCenter(today)}
	p.body.Refresh()
}

func (p *datePicker) show(month time.Time) {
	p.month = month
	p.fill()
	p.place() // a month with 6 weeks is taller
}

func (p *datePicker) pick(d time.Time) {
	p.pop.Hide()
	p.pop = nil
	p.setDate(d)
	if p.onChanged != nil {
		p.onChanged(d)
	}
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
