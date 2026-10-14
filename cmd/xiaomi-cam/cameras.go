package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showCameras - camera management window: list with rename/remove, and "Add camera".
func (u *ui) showCameras() {
	list := container.NewVBox()
	var d *dialog.CustomDialog

	var refresh func()
	refresh = func() {
		list.RemoveAll()
		names := cameraNames()
		if len(names) == 0 {
			list.Add(widget.NewLabel(T("Brak kamer.")))
		}
		for _, name := range names {
			name := name
			model := sourceParam(cameraSource(name), "model")
			title := widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			info := widget.NewLabel(model)
			info.Importance = widget.LowImportance
			list.Add(container.NewBorder(nil, nil, nil,
				container.NewHBox(
					newButton("", theme.DocumentCreateIcon(), func() { u.renameCamera(name, refresh) }),
					newButton("", theme.DeleteIcon(), func() { u.deleteCamera(name, refresh) }),
				),
				container.NewHBox(title, info),
			))
		}
	}
	refresh()

	add := newButton(T("Dodaj kamerę"), theme.ContentAddIcon(), func() {
		d.Hide()
		showAddCamera(u.win, u.selectCamera)
	})
	add.Importance = widget.HighImportance

	content := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), add), nil, nil,
		container.NewVScroll(list))
	d = dialog.NewCustom(T("Kamery"), T("Zamknij"), content, u.win)
	d.Resize(fyne.NewSize(520, 320))
	d.Show()
}

func (u *ui) renameCamera(name string, done func()) {
	entry := widget.NewEntry()
	entry.SetText(name)
	dialog.ShowForm(T("Zmień nazwę"), T("Zapisz"), T("Anuluj"),
		[]*widget.FormItem{widget.NewFormItem(T("Nazwa"), entry)}, func(ok bool) {
			newName := strings.TrimSpace(entry.Text)
			if !ok || newName == "" || newName == name {
				return
			}
			for _, n := range cameraNames() {
				if n == newName {
					dialog.ShowError(fmt.Errorf(T("Kamera „%s” już istnieje"), newName), u.win)
					return
				}
			}
			selected := u.camera.Selected == name
			if selected {
				u.player.stop()
			}
			src := cameraSource(name)
			if err := addCamera(newName, src); err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			if err := removeCamera(name); err != nil {
				dialog.ShowError(err, u.win)
			}
			u.camera.Options = cameraNames()
			if selected {
				u.camera.SetSelected(newName) // restarts video
			} else {
				u.camera.Refresh()
			}
			done()
		}, u.win)
}

func (u *ui) deleteCamera(name string, done func()) {
	dialog.ShowConfirm(T("Usuń kamerę"), fmt.Sprintf(T("Usunąć „%s” z aplikacji?"), name), func(ok bool) {
		if !ok {
			return
		}
		selected := u.camera.Selected == name
		if selected {
			u.player.stop()
		}
		if err := removeCamera(name); err != nil {
			dialog.ShowError(err, u.win)
		}
		u.camera.Options = cameraNames()
		if selected {
			u.camera.ClearSelected()
			if len(u.camera.Options) > 0 {
				u.camera.SetSelectedIndex(0)
			} else {
				u.restart()
			}
		} else {
			u.camera.Refresh()
		}
		done()
	}, u.win)
}
