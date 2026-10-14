package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var smtpSecurity = []struct{ name, id string }{
	{"STARTTLS (587)", "starttls"},
	{"SSL/TLS (465)", "tls"},
	{"Brak (25)", "none"},
}

// notificationSettings - the "Notifications" tab: e-mail (SMTP) and Slack.
func (u *ui) notificationSettings() fyne.CanvasObject {
	cfg := u.ai.config()
	ec := cfg.Email

	host := widget.NewEntry()
	host.SetText(ec.Host)
	host.SetPlaceHolder("smtp.gmail.com")
	port := widget.NewEntry()
	if ec.Port > 0 {
		port.SetText(strconv.Itoa(ec.Port))
	}
	port.SetPlaceHolder("587")
	var secNames []string
	for _, s := range smtpSecurity {
		secNames = append(secNames, T(s.name))
	}
	security := widget.NewSelect(secNames, nil)
	security.SetSelectedIndex(0)
	for i, s := range smtpSecurity {
		if s.id == ec.Security {
			security.SetSelectedIndex(i)
		}
	}
	user := widget.NewEntry()
	user.SetText(ec.Username)
	pass := widget.NewPasswordEntry()
	pass.SetText(ec.Password)
	from := widget.NewEntry()
	from.SetText(ec.From)
	from.SetPlaceHolder("kamera@example.com")
	to := widget.NewEntry()
	to.SetText(ec.To)
	to.SetPlaceHolder(T("adres@example.com, drugi@example.com"))

	slack := widget.NewEntry()
	slack.SetText(cfg.SlackWebhook)
	slack.SetPlaceHolder("https://hooks.slack.com/services/...")
	slackHelp := widget.NewLabel(T("Slack → Twoja aplikacja → Incoming Webhooks → Add New Webhook to Workspace → wybierz kanał i skopiuj adres."))
	slackHelp.Wrapping = fyne.TextWrapWord
	slackHelp.Importance = widget.LowImportance

	current := func() (emailConfig, string) {
		p, _ := strconv.Atoi(strings.TrimSpace(port.Text))
		return emailConfig{
			Host: strings.TrimSpace(host.Text), Port: p,
			Security: smtpSecurity[max(security.SelectedIndex(), 0)].id,
			Username: strings.TrimSpace(user.Text), Password: pass.Text,
			From: strings.TrimSpace(from.Text), To: strings.TrimSpace(to.Text),
		}, strings.TrimSpace(slack.Text)
	}
	save := func() bool {
		e, s := current()
		if err := u.ai.setNotifications(e, s); err != nil {
			dialog.ShowError(err, u.win)
			return false
		}
		return true
	}
	test := func(send func() error) {
		u.status.SetText(T("Wysyłam test…"))
		go func() {
			err := send()
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, u.win)
					u.status.SetText("")
					return
				}
				u.status.SetText(T("Test wysłany"))
				dialog.ShowInformation(T("Test"), T("Wiadomość testowa wysłana."), u.win)
			})
		}()
	}
	testEmail := newButton(T("Wyślij test"), theme.MailSendIcon(), func() {
		if !save() {
			return
		}
		e, _ := current()
		test(func() error {
			return sendEmail(e, "Xiaomi Cam: test",
				T("To jest wiadomość testowa z Xiaomi Cam.")+"\n"+time.Now().Format("2006-01-02 15:04:05"), nil, "")
		})
	})
	testSlack := newButton(T("Wyślij test"), theme.MailSendIcon(), func() {
		if !save() {
			return
		}
		_, s := current()
		test(func() error {
			return sendSlack(s, "*Xiaomi Cam: test*\n"+T("To jest wiadomość testowa z Xiaomi Cam."))
		})
	})
	saveBtn := newButton(T("Zapisz"), theme.DocumentSaveIcon(), func() {
		if save() {
			u.status.SetText(T("Zapisano ustawienia powiadomień"))
		}
	})
	saveBtn.Importance = widget.HighImportance

	emailForm := widget.NewForm(
		widget.NewFormItem(T("Serwer SMTP"), container.NewBorder(nil, nil, nil,
			container.NewGridWrap(fyne.NewSize(90, 36), port), host)),
		widget.NewFormItem(T("Szyfrowanie"), security),
		widget.NewFormItem(T("Użytkownik"), user),
		widget.NewFormItem(T("Hasło"), pass),
		widget.NewFormItem(T("Od"), from),
		widget.NewFormItem(T("Do"), to),
	)
	slackForm := widget.NewForm(widget.NewFormItem(T("Webhook URL"), slack))

	heading := func(s string) *widget.Label {
		return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	return container.NewVScroll(container.NewVBox(
		heading(T("E-mail")), emailForm, container.NewHBox(layout.NewSpacer(), testEmail),
		widget.NewSeparator(),
		heading("Slack"), slackForm, slackHelp, container.NewHBox(layout.NewSpacer(), testSlack),
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), saveBtn),
	))
}

// notificationsHint - what the e-mail/Slack checkboxes in a task need
func notificationsHint() string {
	return fmt.Sprintf(T("E-mail i Slack konfigurujesz w Ustawienia → %s."), T("Powiadomienia"))
}
