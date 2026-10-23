package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/AlexxIT/go2rtc/internal/xccore"
)

var regions = xccore.Regions

// inBackground runs work off the UI thread behind a "please wait" dialog, then then on the
// UI thread. The Mi cloud can take seconds to answer (more from another region), and on the
// UI thread the whole window froze meanwhile.
func inBackground(w fyne.Window, work func(), then func()) {
	d := dialog.NewCustomWithoutButtons(T("Proszę czekać…"), widget.NewProgressBarInfinite(), w)
	d.Resize(fyne.NewSize(320, 0))
	d.Show()
	go func() {
		work()
		fyne.Do(func() {
			d.Hide()
			then()
		})
	}()
}

// miAccounts lists the Mi accounts logged in (the go2rtc API; call it off the UI thread).
func miAccounts() []string {
	var accounts []string
	if b, code, err := apiCall("GET", "/api/xiaomi", nil, nil); err == nil && code == http.StatusOK {
		_ = json.Unmarshal(b, &accounts)
	}
	return accounts
}

// showAddCamera: login to Mi account (via embedded go2rtc API) and add selected cameras.
func showAddCamera(w fyne.Window, onAdded func(name string)) {
	var accounts []string
	inBackground(w, func() { accounts = miAccounts() }, func() { showAddCameraFor(w, accounts, onAdded) })
}

func showAddCameraFor(w fyne.Window, accounts []string, onAdded func(name string)) {
	if len(accounts) > 0 {
		dialog.ShowCustomConfirm(T("Dodaj kamerę"), T("Wybierz kamery"), T("Zaloguj inne konto"),
			widget.NewLabel(T("Masz już zalogowane konto Mi Home.")), func(useExisting bool) {
				if useExisting {
					showDevices(w, accounts, onAdded)
				} else {
					showLogin(w, onAdded)
				}
			}, w)
		return
	}
	showLogin(w, onAdded)
}

func showLogin(w fyne.Window, onAdded func(string)) {
	user := widget.NewEntry()
	user.SetPlaceHolder(T("e-mail, telefon lub Xiaomi ID"))
	pass := widget.NewPasswordEntry()
	var d dialog.Dialog
	qr := newButton(T("Zaloguj kodem QR (bez kodu z maila)"), nil, func() {
		d.Hide()
		showLoginQR(w, onAdded)
	})
	qr.Importance = widget.HighImportance
	items := []*widget.FormItem{
		widget.NewFormItem("", qr),
		widget.NewFormItem(T("Login"), user),
		widget.NewFormItem(T("Hasło"), pass),
	}
	d = dialog.NewForm(T("Logowanie do Mi Home"), T("Zaloguj"), T("Anuluj"), items, func(ok bool) {
		if ok {
			loginStep(w, url.Values{"username": {strings.TrimSpace(user.Text)}, "password": {pass.Text}}, onAdded)
		}
	}, w)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}

// showLoginQR: a QR code scanned with the phone (no password, no e-mail code).
func showLoginQR(w fyne.Window, onAdded func(string)) {
	var (
		b    []byte
		code int
		err  error
	)
	inBackground(w, func() { b, code, err = apiCall("POST", "/api/xiaomi", nil, url.Values{"qr": {"start"}}) },
		func() { showLoginQRCode(w, b, code, err, onAdded) })
}

func showLoginQRCode(w fyne.Window, b []byte, code int, err error, onAdded func(string)) {
	var ch struct {
		QR string `json:"qr"`
	}
	_ = json.Unmarshal(b, &ch)
	raw, _ := base64.StdEncoding.DecodeString(ch.QR)
	switch {
	case err != nil:
		dialog.ShowError(err, w)
		return
	case code != http.StatusUnauthorized || len(raw) == 0:
		dialog.ShowError(fmt.Errorf(T("logowanie nieudane: %s"), strings.TrimSpace(string(b))), w)
		return
	}
	img := canvas.NewImageFromReader(bytes.NewReader(raw), "qr.png")
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(260, 260))
	text := widget.NewLabel(T("Zeskanuj ten kod telefonem z kontem Xiaomi (Mi Home → + → Skanuj, albo Ustawienia → Konto Xiaomi na telefonie Xiaomi) i potwierdź logowanie na telefonie."))
	text.Wrapping = fyne.TextWrapWord
	rtlAlign(text)
	status := widget.NewLabel(T("Czekam na zeskanowanie…"))
	done := make(chan struct{})
	d := dialog.NewCustom(T("Logowanie do Mi Home"), T("Anuluj"), container.NewVBox(text, img, status), w)
	d.SetOnClosed(func() { close(done) })
	d.Resize(fyne.NewSize(460, 0))
	d.Show()

	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			b, code, err := apiCall("POST", "/api/xiaomi", nil, url.Values{"qr": {"wait"}})
			if err == nil && code == http.StatusUnauthorized {
				continue // not scanned yet
			}
			var accounts []string
			if err == nil && code == http.StatusOK {
				accounts = miAccounts() // here, not on the UI thread
			}
			fyne.Do(func() {
				select {
				case <-done:
					return // closed meanwhile
				default:
				}
				switch {
				case err != nil:
					status.SetText(TJ("Błąd: ", err.Error()))
				case code != http.StatusOK:
					msg := strings.TrimSpace(string(b))
					if strings.Contains(msg, "expired") {
						msg = T("Kod QR wygasł – zamknij i pokaż nowy.")
					}
					status.SetText(msg)
				default:
					d.Hide()
					showDevices(w, accounts, onAdded)
				}
			})
			return
		}
	}()
}

func loginStep(w fyne.Window, form url.Values, onAdded func(string)) {
	var (
		b        []byte
		code     int
		err      error
		accounts []string
	)
	inBackground(w, func() {
		b, code, err = apiCall("POST", "/api/xiaomi", nil, form)
		if err == nil && code == http.StatusOK {
			accounts = miAccounts()
		}
	}, func() { loginResult(w, form, b, code, err, accounts, onAdded) })
}

func loginResult(w fyne.Window, form url.Values, b []byte, code int, err error, accounts []string, onAdded func(string)) {
	switch {
	case err != nil:
		dialog.ShowError(err, w)
	case code == http.StatusUnauthorized:
		var ch struct {
			Captcha     string `json:"captcha"`
			VerifyPhone string `json:"verify_phone"`
			VerifyEmail string `json:"verify_email"`
		}
		_ = json.Unmarshal(b, &ch)
		key := "verify"
		if ch.Captcha != "" {
			key = "captcha"
		}
		showCodeDialog(w, key, ch.Captcha, ch.VerifyEmail+ch.VerifyPhone, "", onAdded)
	case code == http.StatusBadRequest && form.Get("verify") != "" && strings.Contains(string(b), "not accepted"):
		// a wrong code: the login step stays, type it again (Xiaomi sends no new code)
		showCodeDialog(w, "verify", "", "", T("Xiaomi nie przyjęło kodu. Sprawdź go i wpisz ponownie (nowy kod nie został wysłany).")+
			"\n"+strings.TrimSpace(string(b)), onAdded)
	case code != http.StatusOK:
		dialog.ShowError(fmt.Errorf(T("logowanie nieudane: %s"), strings.TrimSpace(string(b))), w)
	default:
		showDevices(w, accounts, onAdded)
	}
}

// showCodeDialog asks for the captcha (captcha: base64 JPEG) or the e-mail/SMS code.
func showCodeDialog(w fyne.Window, key, captcha, target, errText string, onAdded func(string)) {
	entry := widget.NewEntry()
	box := container.NewVBox()
	if errText != "" {
		l := widget.NewLabel(errText)
		l.Wrapping = fyne.TextWrapWord
		rtlAlign(l)
		l.Importance = widget.DangerImportance
		box.Add(l)
	}
	if key == "captcha" {
		raw, _ := base64.StdEncoding.DecodeString(captcha)
		img := canvas.NewImageFromReader(bytes.NewReader(raw), "captcha.jpg")
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(220, 80))
		box.Add(widget.NewLabel(T("Przepisz kod z obrazka:")))
		box.Add(img)
	} else if target != "" {
		box.Add(widget.NewLabel(TJ("Xiaomi wysłało kod weryfikacyjny na: ", target)))
	} else {
		box.Add(widget.NewLabel(T("Kod weryfikacyjny:")))
	}
	box.Add(entry)
	d := dialog.NewCustomConfirm(T("Weryfikacja"), T("Wyślij"), T("Anuluj"), box, func(ok bool) {
		if ok && strings.TrimSpace(entry.Text) != "" {
			loginStep(w, url.Values{key: {strings.TrimSpace(entry.Text)}}, onAdded)
		}
	}, w)
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
}

type source struct {
	Name string `json:"name"`
	Info string `json:"info"`
	URL  string `json:"url"`
}

func showDevices(w fyne.Window, accounts []string, onAdded func(string)) {
	account := newSelect(accounts, nil)
	if len(accounts) > 0 {
		account.SetSelectedIndex(0)
	}
	var regionNames []string
	for _, r := range regions {
		regionNames = append(regionNames, T(r.Name))
	}
	region := newSelect(regionNames, nil)
	region.SetSelectedIndex(0)

	var found []source
	checks := container.NewVBox()
	info := widget.NewLabel("")
	var showFound func(b []byte, status int, err error)
	seq := 0
	load := func() {
		checks.RemoveAll()
		found = nil
		seq++
		mine := seq
		code := regions[region.SelectedIndex()].Code
		info.SetText(T("Proszę czekać…"))
		acc := account.Selected
		go func() {
			b, status, err := apiCall("GET", "/api/xiaomi", url.Values{"id": {acc}, "region": {code}}, nil)
			fyne.Do(func() {
				if mine == seq {
					showFound(b, status, err)
				}
			})
		}()
	}
	showFound = func(b []byte, status int, err error) {
		switch {
		case err != nil:
			info.SetText(TJ("Błąd: ", err.Error()))
		case status == http.StatusNotFound:
			info.SetText(T("Brak kamer w tym regionie – wybierz inny."))
		case status != http.StatusOK:
			info.SetText(TJ("Błąd: ", strings.TrimSpace(string(b))))
		default:
			var res struct {
				Sources []source `json:"sources"`
			}
			_ = json.Unmarshal(b, &res)
			found = res.Sources
			for _, s := range found {
				c := newCheck(s.Name+"  ("+s.Info+")", nil)
				c.SetChecked(true)
				checks.Add(c)
			}
			info.SetText(fmt.Sprintf(T("Znaleziono kamer: %d"), len(found)))
		}
	}
	region.OnChanged = func(string) { load() }
	account.OnChanged = func(string) { load() }

	content := container.NewVBox(
		widget.NewForm(widget.NewFormItem(T("Konto"), account), widget.NewFormItem(T("Region"), region)),
		info, checks,
	)
	d := dialog.NewCustomConfirm(T("Wybierz kamery"), T("Dodaj"), T("Anuluj"), content, func(ok bool) {
		if !ok {
			return
		}
		var first string
		for i, s := range found {
			if !checks.Objects[i].(*ptrCheck).Checked {
				continue
			}
			name, err := xccore.AddXiaomiCamera(s.Name, s.URL)
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if first == "" {
				first = name
			}
		}
		if first != "" {
			onAdded(first)
		}
	}, w)
	d.Resize(fyne.NewSize(520, 360))
	d.Show()
	load()
}
