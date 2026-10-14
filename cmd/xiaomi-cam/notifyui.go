package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
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

var ntfyPriorities = []struct{ name, id string }{
	{"Normalny", "default"},
	{"Wysoki", "high"},
	{"Pilny (alarm)", "urgent"},
	{"Niski", "low"},
}

// notificationSettings - the "Notifications" tab: e-mail, Slack, ntfy and Telegram.
func (u *ui) notificationSettings() fyne.CanvasObject {
	cfg := u.ai.config()
	ec := cfg.Email

	help := func(s string) *widget.Label {
		l := widget.NewLabel(s)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = widget.LowImportance
		return l
	}
	heading := func(s string) *widget.Label {
		return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}

	// e-mail
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

	// Slack
	slack := widget.NewEntry()
	slack.SetText(cfg.SlackWebhook)
	slack.SetPlaceHolder("https://hooks.slack.com/services/...")

	// ntfy
	ntfyServer := widget.NewEntry()
	ntfyServer.SetText(cfg.Ntfy.Server)
	ntfyServer.SetPlaceHolder("https://ntfy.sh")
	ntfyTopic := widget.NewEntry()
	ntfyTopic.SetText(cfg.Ntfy.Topic)
	ntfyTopic.SetPlaceHolder("xiaomi-cam-...")
	genTopic := newButton(T("Generuj losowy"), theme.ViewRefreshIcon(), func() { ntfyTopic.SetText(randomTopic()) })
	ntfyToken := widget.NewPasswordEntry()
	ntfyToken.SetText(cfg.Ntfy.Token)
	ntfyToken.SetPlaceHolder(T("opcjonalnie, dla własnego serwera"))
	var prioNames []string
	for _, p := range ntfyPriorities {
		prioNames = append(prioNames, T(p.name))
	}
	ntfyPrio := widget.NewSelect(prioNames, nil)
	ntfyPrio.SetSelectedIndex(0)
	for i, p := range ntfyPriorities {
		if p.id == cfg.Ntfy.Priority {
			ntfyPrio.SetSelectedIndex(i)
		}
	}

	// Telegram
	tgToken := widget.NewPasswordEntry()
	tgToken.SetText(cfg.Telegram.Token)
	tgToken.SetPlaceHolder("123456789:AA...")
	tgChat := widget.NewEntry()
	tgChat.SetText(cfg.Telegram.ChatID)
	findChat := newButton(T("Wykryj chat ID"), theme.SearchIcon(), func() {
		token := strings.TrimSpace(tgToken.Text)
		go func() {
			id, name, err := telegramFindChatID(token)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, u.win)
					return
				}
				tgChat.SetText(id)
				u.status.SetText(fmt.Sprintf("Telegram: %s (%s)", name, id))
			})
		}()
	})

	current := func() notifyChannels {
		p, _ := strconv.Atoi(strings.TrimSpace(port.Text))
		return notifyChannels{
			Email: emailConfig{
				Host: strings.TrimSpace(host.Text), Port: p,
				Security: smtpSecurity[max(security.SelectedIndex(), 0)].id,
				Username: strings.TrimSpace(user.Text), Password: pass.Text,
				From: strings.TrimSpace(from.Text), To: strings.TrimSpace(to.Text),
			},
			SlackWebhook: strings.TrimSpace(slack.Text),
			Ntfy: ntfyConfig{
				Server: strings.TrimSpace(ntfyServer.Text), Topic: strings.TrimSpace(ntfyTopic.Text),
				Token: strings.TrimSpace(ntfyToken.Text), Priority: ntfyPriorities[max(ntfyPrio.SelectedIndex(), 0)].id,
			},
			Telegram: telegramConfig{Token: strings.TrimSpace(tgToken.Text), ChatID: strings.TrimSpace(tgChat.Text)},
		}
	}
	save := func() bool {
		if err := u.ai.setNotifications(current()); err != nil {
			dialog.ShowError(err, u.win)
			return false
		}
		return true
	}
	testText := T("To jest wiadomość testowa z Xiaomi Cam.")
	test := func(send func(n notifyChannels) error) func() {
		return func() {
			if !save() {
				return
			}
			n := current()
			u.status.SetText(T("Wysyłam test…"))
			go func() {
				err := send(n)
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
	}
	testBtn := func(f func(n notifyChannels) error) *ptrButton {
		return newButton(T("Wyślij test"), theme.MailSendIcon(), test(f))
	}
	saveBtn := newButton(T("Zapisz"), theme.DocumentSaveIcon(), func() {
		if save() {
			u.status.SetText(T("Zapisano ustawienia powiadomień"))
		}
	})
	saveBtn.Importance = widget.HighImportance

	return container.NewVScroll(container.NewVBox(
		heading(T("Powiadomienie push na telefon (ntfy)")),
		widget.NewForm(
			widget.NewFormItem(T("Temat"), container.NewBorder(nil, nil, nil, genTopic, ntfyTopic)),
			widget.NewFormItem(T("Serwer"), ntfyServer),
			widget.NewFormItem(T("Token"), ntfyToken),
			widget.NewFormItem(T("Priorytet"), ntfyPrio),
		),
		help(T("Zainstaluj aplikację ntfy (Android/iOS), dodaj subskrypcję tego samego tematu (serwer ntfy.sh). Każdy, kto zna temat, może go czytać – użyj długiego, losowego tematu. Powiadomienie przychodzi ze zdjęciem.")),
		container.NewHBox(layout.NewSpacer(), testBtn(func(n notifyChannels) error {
			return sendNtfy(n.Ntfy, "Xiaomi Cam: test", testText, testJPEG(), "test.jpg")
		})),
		widget.NewSeparator(),

		heading("Telegram"),
		widget.NewForm(
			widget.NewFormItem(T("Token bota"), tgToken),
			widget.NewFormItem(T("Chat ID"), container.NewBorder(nil, nil, nil, findChat, tgChat)),
		),
		help(T("W Telegramie napisz do @BotFather /newbot i skopiuj token. Potem wyślij dowolną wiadomość do swojego bota i kliknij „Wykryj chat ID”. Powiadomienie przychodzi jako zdjęcie z opisem.")),
		container.NewHBox(layout.NewSpacer(), testBtn(func(n notifyChannels) error {
			return sendTelegram(n.Telegram, "Xiaomi Cam: test\n"+testText, testJPEG(), "test.jpg")
		})),
		widget.NewSeparator(),

		heading(T("E-mail")),
		widget.NewForm(
			widget.NewFormItem(T("Serwer SMTP"), container.NewBorder(nil, nil, nil,
				container.NewGridWrap(fyne.NewSize(90, 36), port), host)),
			widget.NewFormItem(T("Szyfrowanie"), security),
			widget.NewFormItem(T("Użytkownik"), user),
			widget.NewFormItem(T("Hasło"), pass),
			widget.NewFormItem(T("Od"), from),
			widget.NewFormItem(T("Do"), to),
		),
		container.NewHBox(layout.NewSpacer(), testBtn(func(n notifyChannels) error {
			return sendEmail(n.Email, "Xiaomi Cam: test", testText+"\n"+time.Now().Format("2006-01-02 15:04:05"), nil, "")
		})),
		widget.NewSeparator(),

		heading("Slack"),
		widget.NewForm(widget.NewFormItem(T("Webhook URL"), slack)),
		help(T("Slack → Twoja aplikacja → Incoming Webhooks → Add New Webhook to Workspace → wybierz kanał i skopiuj adres.")),
		container.NewHBox(layout.NewSpacer(), testBtn(func(n notifyChannels) error {
			return sendSlack(n.SlackWebhook, "*Xiaomi Cam: test*\n"+testText)
		})),
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), saveBtn),
	))
}

// randomTopic - long random ntfy topic (topics on ntfy.sh are public to whoever knows the name)
func randomTopic() string {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return "xiaomi-cam-" + string(b)
}

// testJPEG - small picture for "send test" (the camera picture isn't sent in tests)
func testJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / 320), G: 0x67, B: uint8(y * 255 / 180), A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// notificationsHint - what the channel checkboxes in a task need
func notificationsHint() string {
	return fmt.Sprintf(T("Kanały konfigurujesz w Ustawienia → %s."), T("Powiadomienia"))
}
