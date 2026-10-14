package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showSettings - app settings: folders for recordings and snapshots.
func (u *ui) showSettings() {
	folderRow := func(pref string, current func() string) fyne.CanvasObject {
		label := widget.NewLabel(current())
		label.Truncation = fyne.TextTruncateEllipsis
		choose := newButton(T("Zmień…"), theme.FolderOpenIcon(), func() {
			d := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
				if err != nil || dir == nil {
					return
				}
				u.app.Preferences().SetString(pref, dir.Path())
				label.SetText(current())
			}, u.win)
			if l, err := storage.ListerForURI(storage.NewFileURI(current())); err == nil {
				d.SetLocation(l)
			}
			d.Resize(fyne.NewSize(700, 480))
			d.Show()
		})
		reset := newButton("", theme.ViewRefreshIcon(), func() {
			u.app.Preferences().RemoveValue(pref) // back to the system folder
			label.SetText(current())
		})
		return container.NewBorder(nil, nil, nil, container.NewHBox(choose, reset), label)
	}
	form := widget.NewForm(
		widget.NewFormItem(T("Nagrania"), folderRow(prefRecordingsDir, recordingsDir)),
		widget.NewFormItem(T("Zdjęcia"), folderRow(prefSnapshotsDir, snapshotsDir)),
	)
	d := dialog.NewCustom(T("Ustawienia"), T("Zamknij"), form, u.win)
	d.Resize(fyne.NewSize(640, 0))
	d.Show()
}

// toggleRecording starts/stops local recording of the selected camera.
func (u *ui) toggleRecording() {
	name := u.camera.Selected
	if name == "" {
		return
	}
	if currentRecording(name) != nil {
		go func() {
			r, err := stopRecording(name)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, u.win)
				} else {
					u.status.SetText(T("Zapisano ") + r.path)
				}
			})
		}()
		return
	}
	if _, err := startRecording(name, 0); err != nil {
		dialog.ShowError(err, u.win)
	}
}

// updateRecordButton - record button look and the status line while recording
// (UI thread, called every second and when a recording starts/stops).
func (u *ui) updateRecordButton() {
	if u.recButton == nil {
		return
	}
	r := currentRecording(u.camera.Selected)
	if r == nil {
		if u.recButton.Importance != widget.MediumImportance {
			u.recButton.SetIcon(recordIcon)
			u.recButton.Importance = widget.MediumImportance
			u.recButton.Refresh()
		}
		return
	}
	if u.recButton.Importance != widget.DangerImportance {
		u.recButton.SetIcon(theme.MediaStopIcon())
		u.recButton.Importance = widget.DangerImportance
		u.recButton.Refresh()
	}
	d := time.Since(r.started).Truncate(time.Second)
	u.status.SetText(fmt.Sprintf(T("● Nagrywanie %s"), d))
}
