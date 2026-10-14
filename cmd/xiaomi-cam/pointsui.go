package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Camera points 1-3 (preset positions stored in the camera): buttons in the bottom bar,
// keys 1-3, and a dialog to save the current position / rename / remove.

func (u *ui) buildPoints() fyne.CanvasObject {
	box := container.NewHBox()
	for i := 0; i < presetSlots; i++ {
		slot := i + 1
		b := newButton(fmt.Sprint(slot), nil, func() { u.pointPressed(slot) })
		u.pointButtons[i] = b
		box.Add(b)
	}
	box.Add(newButton("", theme.SettingsIcon(), u.showPoints))
	return box
}

// loadPoints reads the points of the selected camera (in the background) and updates the buttons.
func (u *ui) loadPoints() {
	name := u.camera.Selected
	if name == "" {
		return
	}
	go func() {
		list, err := getPresets(name)
		fyne.Do(func() {
			if name != u.camera.Selected {
				return // another camera was selected meanwhile
			}
			u.points = presetSlotsOf(list) // empty on error (e.g. a camera without points)
			_ = err
			for i, b := range u.pointButtons {
				if b == nil {
					continue
				}
				if p := u.points[i]; p != nil {
					label := p.Label()
					if r := []rune(label); len(r) > 10 {
						label = string(r[:10]) + "…"
					}
					b.SetText(fmt.Sprintf("%d %s", i+1, label))
					b.Importance = widget.MediumImportance
				} else {
					b.SetText(fmt.Sprint(i + 1))
					b.Importance = widget.LowImportance
				}
				b.Refresh()
			}
		})
	}()
}

// pointPressed turns to a defined point, or offers to save the current position.
func (u *ui) pointPressed(slot int) {
	name := u.camera.Selected
	if name == "" {
		return
	}
	if p := u.points[slot-1]; p != nil {
		u.status.SetText(fmt.Sprintf(T("Obracam do punktu %d (%s)…"), slot, p.Label()))
		idx := p.Idx
		go func() {
			err := gotoPreset(name, idx)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, u.win)
				}
			})
		}()
		return
	}
	u.savePointDialog(slot)
}

func (u *ui) savePointDialog(slot int) {
	entry := widget.NewEntry()
	if p := u.points[slot-1]; p != nil {
		entry.SetText(p.Label())
	}
	entry.SetPlaceHolder(T("np. auto, podjazd, brama"))
	dialog.ShowForm(fmt.Sprintf(T("Punkt %d – zapisz bieżącą pozycję"), slot), T("Zapisz"), T("Anuluj"),
		[]*widget.FormItem{widget.NewFormItem(T("Nazwa"), entry)}, func(ok bool) {
			if !ok {
				return
			}
			name, label := u.camera.Selected, strings.TrimSpace(entry.Text)
			go func() {
				err := savePresetSlot(name, slot, label)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, u.win)
						return
					}
					u.status.SetText(fmt.Sprintf(T("Zapisano punkt %d"), slot))
					u.loadPoints()
				})
			}()
		}, u.win)
}

// showPoints - manage points 1-3: save the current position, remove.
func (u *ui) showPoints() {
	rows := container.NewVBox()
	var d dialog.Dialog
	for i := 0; i < presetSlots; i++ {
		slot := i + 1
		desc := T("(pusty)")
		if p := u.points[i]; p != nil {
			desc = p.Label()
		}
		del := newButton("", theme.DeleteIcon(), func() {
			p := u.points[slot-1]
			if p == nil {
				return
			}
			name, idx := u.camera.Selected, p.Idx
			go func() {
				err := deletePreset(name, idx)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, u.win)
					}
					d.Hide()
					u.loadPoints()
				})
			}()
		})
		if u.points[i] == nil {
			del.Disable()
		}
		rows.Add(container.NewBorder(nil, nil,
			widget.NewLabelWithStyle(fmt.Sprintf(T("Punkt %d"), slot), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(
				newButton(T("Zapisz bieżącą pozycję"), theme.DocumentSaveIcon(), func() {
					d.Hide()
					u.savePointDialog(slot)
				}),
				del),
			widget.NewLabel(desc)))
	}
	help := widget.NewLabel(T("Ustaw kamerę (strzałki, zoom) i zapisz pozycję jako punkt. Kliknięcie punktu lub klawisz 1–3 obraca kamerę do niego. Punkty są zapisane w kamerze (widoczne też w Mi Home)."))
	help.Wrapping = fyne.TextWrapWord
	help.Importance = widget.LowImportance
	d = dialog.NewCustom(T("Punkty kamery"), T("Zamknij"), container.NewVBox(rows, help), u.win)
	d.Resize(fyne.NewSize(620, 0))
	d.Show()
}
