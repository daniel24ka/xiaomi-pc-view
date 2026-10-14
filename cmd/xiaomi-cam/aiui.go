package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var schedulePresets = []struct{ name, spec string }{
	{"Co minutę", "@every 1m"},
	{"Co 5 minut", "@every 5m"},
	{"Co 15 minut", "@every 15m"},
	{"Co 30 minut", "@every 30m"},
	{"Co godzinę", "@hourly"},
	{"Własny (CRON)", ""},
}

var providers = []struct{ name, id string }{
	{"Claude (Anthropic)", providerClaude},
	{"OpenAI", providerOpenAI},
}

// aiSettings - the "AI" tab of the settings: providers and tasks.
func (u *ui) aiSettings() fyne.CanvasObject {
	cfg := u.ai.config()

	claudeKey := widget.NewPasswordEntry()
	claudeKey.SetText(cfg.AnthropicKey)
	claudeKey.SetPlaceHolder("sk-ant-...")
	claudeModel := widget.NewEntry()
	claudeModel.SetText(cfg.AnthropicModel)
	openaiKey := widget.NewPasswordEntry()
	openaiKey.SetText(cfg.OpenAIKey)
	openaiKey.SetPlaceHolder("sk-...")
	openaiModel := widget.NewEntry()
	openaiModel.SetText(cfg.OpenAIModel)
	saveKeys := newButton(T("Zapisz klucze"), theme.DocumentSaveIcon(), func() {
		if err := u.ai.setProviders(claudeKey.Text, claudeModel.Text, openaiKey.Text, openaiModel.Text); err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		u.status.SetText(T("Zapisano ustawienia AI"))
	})
	providersForm := widget.NewForm(
		widget.NewFormItem(T("Claude – klucz API"), claudeKey),
		widget.NewFormItem(T("Claude – model"), claudeModel),
		widget.NewFormItem(T("OpenAI – klucz API"), openaiKey),
		widget.NewFormItem(T("OpenAI – model"), openaiModel),
	)

	list := container.NewVBox()
	var refresh func()
	refresh = func() {
		list.RemoveAll()
		tasks := u.ai.config().Tasks
		if len(tasks) == 0 {
			l := widget.NewLabel(T("Brak zadań. Dodaj zadanie, aby AI sprawdzało obraz z kamery według harmonogramu."))
			l.Wrapping = fyne.TextWrapWord
			list.Add(l)
		}
		for _, t := range tasks {
			t := t
			enabled := newCheck("", func(on bool) {
				if err := u.ai.setEnabled(t.ID, on); err != nil {
					dialog.ShowError(err, u.win)
				}
			})
			enabled.SetChecked(t.Enabled)
			title := widget.NewLabelWithStyle(t.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			info := widget.NewLabel(fmt.Sprintf("%s · %s · %s", scheduleName(t.Schedule), providerName(t.Provider), cameraLabel(t.Camera)))
			info.Importance = widget.LowImportance
			status := widget.NewLabel(taskStatus(t))
			status.Wrapping = fyne.TextWrapWord
			buttons := container.NewHBox(
				newButton("", theme.MediaPlayIcon(), func() {
					u.ai.runNow(t.ID)
					u.status.SetText(fmt.Sprintf(T("Zadanie „%s” uruchomione"), t.Name))
				}),
				newButton("", theme.DocumentCreateIcon(), func() { u.editTask(t, refresh) }),
				newButton("", theme.DeleteIcon(), func() {
					dialog.ShowConfirm(T("Usuń zadanie"), fmt.Sprintf(T("Usunąć zadanie „%s”?"), t.Name), func(ok bool) {
						if ok {
							if err := u.ai.deleteTask(t.ID); err != nil {
								dialog.ShowError(err, u.win)
							}
							refresh()
						}
					}, u.win)
				}),
			)
			list.Add(container.NewBorder(nil, nil, enabled, buttons,
				container.NewVBox(container.NewHBox(title, info), status)))
			list.Add(widget.NewSeparator())
		}
	}
	refresh()
	u.ai.onChange = func() { fyne.Do(refresh) }

	add := newButton(T("Dodaj zadanie"), theme.ContentAddIcon(), func() { u.editTask(nil, refresh) })
	add.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(providersForm, container.NewHBox(layout.NewSpacer(), saveKeys), widget.NewSeparator(),
			widget.NewLabelWithStyle(T("Zadania"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
		container.NewHBox(layout.NewSpacer(), add), nil, nil,
		container.NewVScroll(list),
	)
}

func taskStatus(t *aiTask) string {
	switch {
	case t.LastRun.IsZero():
		return T("Jeszcze nie uruchomione")
	case t.LastError != "":
		return fmt.Sprintf("%s – ⚠ %s", t.LastRun.Format("02.01 15:04"), t.LastError)
	case t.LastDetected:
		return fmt.Sprintf("%s – ✔ %s: %s", t.LastRun.Format("02.01 15:04"), T("wykryto"), t.LastAnswer)
	default:
		return fmt.Sprintf("%s – ✘ %s: %s", t.LastRun.Format("02.01 15:04"), T("nie wykryto"), t.LastAnswer)
	}
}

func scheduleName(spec string) string {
	for _, p := range schedulePresets {
		if p.spec == spec {
			return T(p.name)
		}
	}
	return "CRON " + spec
}

func providerName(id string) string {
	for _, p := range providers {
		if p.id == id {
			return p.name
		}
	}
	return providers[0].name
}

func cameraLabel(name string) string {
	if name == "" {
		return T("pierwsza kamera")
	}
	return name
}

// editTask - form to add (t == nil) or edit an AI task.
func (u *ui) editTask(t *aiTask, done func()) {
	task := &aiTask{Enabled: true, Provider: providerClaude, Schedule: "@every 5m", Notify: true, CooldownMin: 10, RecordSec: 60}
	if t != nil {
		c := *t
		task = &c
	}

	name := widget.NewEntry()
	name.SetText(task.Name)
	name.SetPlaceHolder(T("np. Samochód przed bramą"))

	cams := append([]string{T("pierwsza kamera")}, cameraNames()...)
	camera := widget.NewSelect(cams, nil)
	camera.SetSelected(cameraLabel(task.Camera))

	var provNames []string
	for _, p := range providers {
		provNames = append(provNames, p.name)
	}
	provider := widget.NewSelect(provNames, nil)
	provider.SetSelected(providerName(task.Provider))

	prompt := widget.NewMultiLineEntry()
	prompt.SetText(task.Prompt)
	prompt.SetPlaceHolder(T("np. Czy przed bramą stoi samochód dostawczy? Czy na posesji jest człowiek?"))
	prompt.Wrapping = fyne.TextWrapWord
	prompt.SetMinRowsVisible(4)

	cronEntry := widget.NewEntry()
	cronEntry.SetPlaceHolder("*/10 7-22 * * *")
	var presetNames []string
	for _, p := range schedulePresets {
		presetNames = append(presetNames, T(p.name))
	}
	preset := widget.NewSelect(presetNames, func(s string) {
		for _, p := range schedulePresets {
			if T(p.name) == s {
				if p.spec == "" {
					cronEntry.Enable()
				} else {
					cronEntry.SetText(p.spec)
					cronEntry.Disable()
				}
			}
		}
	})
	selected := len(schedulePresets) - 1
	for i, p := range schedulePresets {
		if p.spec == task.Schedule {
			selected = i
		}
	}
	cronEntry.SetText(task.Schedule)
	preset.SetSelectedIndex(selected)
	cronHelp := widget.NewLabel(T("CRON: minuta godzina dzień miesiąc dzień-tygodnia, np. „*/10 7-22 * * *” – co 10 min od 7 do 22. Każde sprawdzenie to jedno zapytanie do AI ze zdjęciem (koszt)."))
	cronHelp.Wrapping = fyne.TextWrapWord
	cronHelp.Importance = widget.LowImportance

	fullRes := widget.NewCheck(T("Pełna rozdzielczość (2560×1440)"), nil)
	fullRes.SetChecked(task.FullRes)
	fullResHelp := widget.NewLabel(T("Lepiej rozpoznaje szczegóły (marki, tablice), ale zapytanie jest droższe. Domyślnie 1280 px."))
	fullResHelp.Wrapping = fyne.TextWrapWord
	fullResHelp.Importance = widget.LowImportance
	notify := widget.NewCheck(T("Powiadomienie systemowe"), nil)
	notify.SetChecked(task.Notify)
	email := widget.NewCheck(T("E-mail (ze zdjęciem)"), nil)
	email.SetChecked(task.Email)
	slack := widget.NewCheck("Slack", nil)
	slack.SetChecked(task.Slack)
	channelsHint := widget.NewLabel(notificationsHint())
	channelsHint.Importance = widget.LowImportance
	saveSnap := widget.NewCheck(T("Zapisz zdjęcie"), nil)
	saveSnap.SetChecked(task.SaveSnapshot)
	recordSec := widget.NewEntry()
	if task.RecordSec <= 0 {
		task.RecordSec = 60
	}
	recordSec.SetText(strconv.Itoa(task.RecordSec))
	record := widget.NewCheck(T("Nagraj film"), func(on bool) {
		if on {
			recordSec.Enable()
		} else {
			recordSec.Disable()
		}
	})
	record.SetChecked(task.Record)
	if !task.Record {
		recordSec.Disable()
	}
	cooldown := widget.NewEntry()
	cooldown.SetText(strconv.Itoa(task.CooldownMin))

	form := widget.NewForm(
		widget.NewFormItem(T("Nazwa"), name),
		widget.NewFormItem(T("Kamera"), camera),
		widget.NewFormItem(T("Model AI"), provider),
		widget.NewFormItem(T("Polecenie (prompt)"), prompt),
		widget.NewFormItem(T("Zdjęcie"), container.NewVBox(fullRes, fullResHelp)),
		widget.NewFormItem(T("Harmonogram"), container.NewVBox(preset, cronEntry, cronHelp)),
		widget.NewFormItem(T("Gdy wykryje"), container.NewVBox(notify, email, slack, channelsHint, saveSnap,
			container.NewBorder(nil, nil, record, widget.NewLabel(T("s")), recordSec))),
		widget.NewFormItem(T("Nie powtarzaj przez (min)"), cooldown),
	)

	title := T("Nowe zadanie AI")
	if t != nil {
		title = T("Edytuj zadanie AI")
	}
	var d dialog.Dialog
	save := func(ok bool) {
		if !ok {
			return
		}
		task.Name = strings.TrimSpace(name.Text)
		task.Prompt = strings.TrimSpace(prompt.Text)
		task.Schedule = strings.TrimSpace(cronEntry.Text)
		task.Camera = ""
		if camera.SelectedIndex() > 0 {
			task.Camera = camera.Selected
		}
		task.Provider = providers[max(provider.SelectedIndex(), 0)].id
		task.Notify, task.SaveSnapshot, task.Record = notify.Checked, saveSnap.Checked, record.Checked
		task.FullRes = fullRes.Checked
		task.Email, task.Slack = email.Checked, slack.Checked
		task.RecordSec, _ = strconv.Atoi(strings.TrimSpace(recordSec.Text))
		if task.RecordSec <= 0 {
			task.RecordSec = 60
		}
		task.CooldownMin, _ = strconv.Atoi(strings.TrimSpace(cooldown.Text))
		if task.Name == "" || task.Prompt == "" {
			dialog.ShowError(fmt.Errorf("%s", T("Podaj nazwę i polecenie.")), u.win)
			d.Show()
			return
		}
		if err := u.ai.putTask(task); err != nil {
			dialog.ShowError(err, u.win)
			d.Show()
			return
		}
		done()
	}
	d = dialog.NewCustomConfirm(title, T("Zapisz"), T("Anuluj"), container.NewVScroll(form), save, u.win)
	d.Resize(fyne.NewSize(720, 640))
	d.Show()
}
