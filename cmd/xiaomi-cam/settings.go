package main

import (
	"fmt"
	"path/filepath"
	"strings"
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
		choose.tip = T("Wybierz folder")
		reset.tip = T("Przywróć domyślny folder systemowy")
		return container.NewBorder(nil, nil, nil, container.NewHBox(choose, reset), label)
	}
	modes := []struct{ name, id string }{
		{"Wybrana kamera", recordSelected},
		{"Każda kamera osobno (osobne pliki)", recordEach},
		{"Siatka wszystkich kamer (jeden plik)", recordGrid},
	}
	var modeNames []string
	for _, m := range modes {
		modeNames = append(modeNames, T(m.name))
	}
	mode := widget.NewSelect(modeNames, nil)
	for i, m := range modes {
		if m.id == recordMode() {
			mode.SetSelectedIndex(i)
		}
	}
	if mode.SelectedIndex() < 0 {
		mode.SetSelectedIndex(0)
	}
	// cameras recorded in the each/grid modes
	recorded := map[string]bool{}
	for _, n := range recordCameras() {
		recorded[n] = true
	}
	camChecks := container.NewVBox()
	for _, name := range cameraNames() {
		name := name
		c := newCheck(name, func(on bool) { setRecordCamera(name, on) })
		c.SetChecked(recorded[name])
		camChecks.Add(c)
	}
	camLabel := widget.NewLabel(T("Nagrywane kamery:"))
	showCams := func() {
		if id := modes[max(mode.SelectedIndex(), 0)].id; id == recordEach || id == recordGrid {
			camLabel.Show()
			camChecks.Show()
		} else {
			camLabel.Hide()
			camChecks.Hide()
		}
	}
	mode.OnChanged = func(string) {
		u.app.Preferences().SetString(prefRecordMode, modes[max(mode.SelectedIndex(), 0)].id)
		showCams()
	}
	showCams()
	modeHelp := widget.NewLabel(T("Wybrana kamera i osobne pliki kopiują obraz z kamer bez przekodowania (prawie bez obciążenia). Siatka składa kamery w jeden film H.265 – to przekodowanie obciąża procesor."))
	modeHelp.Wrapping = fyne.TextWrapWord
	modeHelp.Importance = widget.LowImportance

	autostart := newCheck(T("Uruchamiaj przy logowaniu (schowana w zasobniku)"), func(on bool) {
		if err := setAutostart(on); err != nil {
			dialog.ShowError(err, u.win)
		}
	}).withTip(T("Aplikacja startuje po zalogowaniu i czeka w zasobniku – zadania AI i powiadomienia działają od razu"))
	autostart.SetChecked(autostartEnabled())

	form := widget.NewForm(
		widget.NewFormItem(T("Start"), autostart),
		widget.NewFormItem(T("Tryb nagrywania"), container.NewVBox(mode, camLabel, camChecks, modeHelp)),
		widget.NewFormItem(T("Nagrania"), folderRow(prefRecordingsDir, recordingsDir)),
		widget.NewFormItem(T("Zdjęcia"), folderRow(prefSnapshotsDir, snapshotsDir)),
	)
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(T("Ogólne"), theme.SettingsIcon(), container.NewVBox(form)),
		container.NewTabItemWithIcon("AI", theme.ComputerIcon(), u.aiSettings()),
		container.NewTabItemWithIcon(T("Zdarzenia"), lucide("bell"), u.eventsSettings()),
		container.NewTabItemWithIcon(T("Powiadomienia"), theme.MailComposeIcon(), u.notificationSettings()),
	)
	d := dialog.NewCustom(T("Ustawienia"), T("Zamknij"), tabs, u.win)
	d.SetOnClosed(func() {
		watch.mu.Lock()
		watch.onChange = nil
		watch.mu.Unlock()
		u.ai.onChange = nil
		u.stopAIRunSpinners()
	})
	d.Resize(fyne.NewSize(860, 640))
	d.Show()
}

// toggleRecording starts recording in the mode from the settings, or stops all recordings.
func (u *ui) toggleRecording() {
	name := u.camera.Selected
	if name == "" {
		return
	}
	if u.recButton.isBusy() {
		return
	}
	done := u.busy(u.recButton)
	if len(activeRecordings()) > 0 {
		go func() {
			stopped := stopAll()
			fyne.Do(func() {
				done()
				u.updateRecordButton()
				var files []string
				for _, r := range stopped {
					files = append(files, filepath.Base(r.path))
				}
				u.status.SetText(T("Zapisano ") + strings.Join(files, ", "))
			})
		}()
		return
	}
	mode := recordMode()
	go func() {
		_, err := startRecordings(mode, name, nil, 0)
		fyne.Do(func() {
			done()
			u.updateRecordButton()
			if err != nil {
				dialog.ShowError(err, u.win)
			}
		})
	}()
}

// updateRecordButton - record button look and the status line while recording
// (UI thread, called every second and when a recording starts/stops).
func (u *ui) updateRecordButton() {
	if u.recButton == nil {
		return
	}
	active := activeRecordings()
	if len(active) == 0 {
		if u.recButton.Importance != widget.MediumImportance {
			u.recButton.setIdleIcon(recordIcon)
			u.recButton.SetText(T("Nagraj"))
			u.recButton.Importance = widget.MediumImportance
			u.recButton.Refresh()
		}
		return
	}
	if u.recButton.Importance != widget.DangerImportance {
		u.recButton.setIdleIcon(theme.MediaStopIcon())
		u.recButton.SetText(T("Stop"))
		u.recButton.Importance = widget.DangerImportance
		u.recButton.Refresh()
	}
	d := time.Since(active[0].started).Truncate(time.Second)
	if len(active) > 1 {
		u.status.SetText(fmt.Sprintf(T("● Nagrywanie %s (%d pliki)"), d, len(active)))
	} else {
		u.status.SetText(fmt.Sprintf(T("● Nagrywanie %s"), d))
	}
}
