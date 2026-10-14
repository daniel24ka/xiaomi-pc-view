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
)

var regions = []struct{ code, name string }{
	{"de", "Europa (Polska)"}, {"cn", "Chiny"}, {"i2", "Indie"},
	{"ru", "Rosja"}, {"sg", "Singapur"}, {"us", "USA"},
}

// showAddCamera: login to Mi account (via embedded go2rtc API) and add selected cameras.
func showAddCamera(w fyne.Window, onAdded func(name string)) {
	var accounts []string
	if b, code, err := apiCall("GET", "/api/xiaomi", nil, nil); err == nil && code == http.StatusOK {
		_ = json.Unmarshal(b, &accounts)
	}
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
	items := []*widget.FormItem{
		widget.NewFormItem(T("Login"), user),
		widget.NewFormItem(T("Hasło"), pass),
	}
	d := dialog.NewForm(T("Logowanie do Mi Home"), T("Zaloguj"), T("Anuluj"), items, func(ok bool) {
		if ok {
			loginStep(w, url.Values{"username": {strings.TrimSpace(user.Text)}, "password": {pass.Text}}, onAdded)
		}
	}, w)
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
}

func loginStep(w fyne.Window, form url.Values, onAdded func(string)) {
	b, code, err := apiCall("POST", "/api/xiaomi", nil, form)
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
		entry := widget.NewEntry()
		var content fyne.CanvasObject
		key := "verify"
		if ch.Captcha != "" {
			key = "captcha"
			raw, _ := base64.StdEncoding.DecodeString(ch.Captcha)
			img := canvas.NewImageFromReader(bytes.NewReader(raw), "captcha.jpg")
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(220, 80))
			content = container.NewVBox(widget.NewLabel(T("Przepisz kod z obrazka:")), img, entry)
		} else {
			target := ch.VerifyEmail + ch.VerifyPhone
			content = container.NewVBox(widget.NewLabel("Xiaomi wysłało kod weryfikacyjny na: "+target), entry)
		}
		dialog.ShowCustomConfirm(T("Weryfikacja"), T("Wyślij"), T("Anuluj"), content, func(ok bool) {
			if ok {
				loginStep(w, url.Values{key: {strings.TrimSpace(entry.Text)}}, onAdded)
			}
		}, w)
	case code != http.StatusOK:
		dialog.ShowError(fmt.Errorf(T("logowanie nieudane: %s"), strings.TrimSpace(string(b))), w)
	default:
		var accounts []string
		if b, _, err = apiCall("GET", "/api/xiaomi", nil, nil); err == nil {
			_ = json.Unmarshal(b, &accounts)
		}
		showDevices(w, accounts, onAdded)
	}
}

type source struct {
	Name string `json:"name"`
	Info string `json:"info"`
	URL  string `json:"url"`
}

func showDevices(w fyne.Window, accounts []string, onAdded func(string)) {
	account := widget.NewSelect(accounts, nil)
	if len(accounts) > 0 {
		account.SetSelectedIndex(0)
	}
	var regionNames []string
	for _, r := range regions {
		regionNames = append(regionNames, T(r.name))
	}
	region := widget.NewSelect(regionNames, nil)
	region.SetSelectedIndex(0)

	var found []source
	checks := container.NewVBox()
	info := widget.NewLabel("")
	load := func() {
		checks.RemoveAll()
		found = nil
		code := regions[region.SelectedIndex()].code
		b, status, err := apiCall("GET", "/api/xiaomi", url.Values{"id": {account.Selected}, "region": {code}}, nil)
		switch {
		case err != nil:
			info.SetText("Błąd: " + err.Error())
		case status == http.StatusNotFound:
			info.SetText(T("Brak kamer w tym regionie – wybierz inny."))
		case status != http.StatusOK:
			info.SetText("Błąd: " + strings.TrimSpace(string(b)))
		default:
			var res struct {
				Sources []source `json:"sources"`
			}
			_ = json.Unmarshal(b, &res)
			found = res.Sources
			for _, s := range found {
				c := widget.NewCheck(s.Name+"  ("+s.Info+")", nil)
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
		existing := map[string]bool{}
		for _, n := range cameraNames() {
			existing[n] = true
		}
		var first string
		for i, s := range found {
			if !checks.Objects[i].(*widget.Check).Checked {
				continue
			}
			name := s.Name
			for n := 2; existing[name]; n++ {
				name = fmt.Sprintf("%s %d", s.Name, n)
			}
			// p2p=server works from any network; subtype=3 - 2.5K (2560x1440)
			if err := addCamera(name, s.URL+"&p2p=server&subtype=3"); err != nil {
				dialog.ShowError(err, w)
				return
			}
			existing[name] = true
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
