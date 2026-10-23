package main

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

// showChooseCamera - "Add camera": a Xiaomi camera (Mi Home account) or an IP camera.
func showChooseCamera(w fyne.Window, onAdded func(name string)) {
	var d *dialog.CustomDialog
	xiaomiBtn := newButton(T("Xiaomi – konto Mi Home"), cameraIcon, func() {
		d.Hide()
		showAddCamera(w, onAdded)
	})
	ipBtn := newButton(T("Kamera IP – ONVIF, RTSP, HTTP, Tapo, DVRIP"), theme.ComputerIcon(), func() {
		d.Hide()
		showAddIPCamera(w, nil, "", func(name string, c *ipCamera) error {
			if err := addIPCamera(name, c); err != nil {
				return err
			}
			onAdded(name)
			return nil
		})
	})
	xiaomiBtn.Alignment, ipBtn.Alignment = widget.ButtonAlignLeading, widget.ButtonAlignLeading
	d = dialog.NewCustom(T("Dodaj kamerę"), T("Anuluj"), container.NewVBox(xiaomiBtn, ipBtn), w)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}

// showAddIPCamera - the form of an IP camera: a new one, or edit (the camera edit with the
// form prefill). save is called on the UI thread when the camera answered.
func showAddIPCamera(w fyne.Window, prefill *ipForm, edit string, save func(name string, c *ipCamera) error) {
	name := widget.NewEntry()
	name.SetPlaceHolder(T("np. Garaż"))
	host := widget.NewEntry()
	host.SetPlaceHolder(T("192.168.1.50 lub pełny adres rtsp://…"))
	port := widget.NewEntry()
	user := widget.NewEntry()
	pass := widget.NewPasswordEntry()
	mainPath := widget.NewEntry()
	subPath := widget.NewEntry()
	subPath.SetPlaceHolder(T("opcjonalnie"))
	channel := widget.NewEntry()
	channel.SetText("0")
	hint := widget.NewLabel("")
	hint.Wrapping = fyne.TextWrapWord
	rtlAlign(hint)
	hint.TextStyle.Italic = true

	// Imou cloud (optional): the light, siren, night mode and camera points of Imou cameras
	imouApp := widget.NewEntry()
	imouApp.SetPlaceHolder("lc…")
	imouSecret := widget.NewPasswordEntry()
	var dcNames []string
	for _, dc := range imouDataCenters {
		dcNames = append(dcNames, T(dc.Name))
	}
	imouDC := newSelect(dcNames, nil)
	imouDC.SetSelectedIndex(0)
	imouDevIDs := map[string]string{} // option -> serial number
	imouDev := newSelect(nil, nil)
	imouDev.PlaceHolder = T("Pobierz listę kamer")
	imouFetch := newButton(T("Pobierz"), theme.DownloadIcon(), nil).withTip(T("Pobiera kamery konta Imou Life"))
	imouOn := widget.NewCheck(T("Sterowanie przez chmurę Imou (reflektor, syrena, tryb nocny, punkty)"), nil)
	setImouDevices := func(devs []imouDevice, selectID string) {
		imouDevIDs = map[string]string{}
		var opts []string
		sel := ""
		for _, dv := range devs {
			o := fmt.Sprintf("%s – %s (%s)", dv.Name, dv.Model, dv.ID)
			imouDevIDs[o] = dv.ID
			opts = append(opts, o)
			if dv.ID == selectID {
				sel = o
			}
		}
		imouDev.SetOptions(opts)
		if sel != "" {
			imouDev.SetSelected(sel)
		}
	}

	var brandNames []string
	for _, b := range rtspBrands {
		brandNames = append(brandNames, b.name)
	}
	brandNames = append(brandNames, T("Inny – własna ścieżka"))
	brand := newSelect(brandNames, func(s string) {
		for _, b := range rtspBrands {
			if b.name == s {
				mainPath.SetText(b.main)
				subPath.SetText(b.sub)
			}
		}
	})

	var kindNames []string
	for _, k := range ipKinds {
		kindNames = append(kindNames, k.name)
	}
	form := container.NewStack()
	var d *dialog.CustomDialog
	kind := newSelect(kindNames, nil)
	kindID := func() string { return ipKinds[max(kind.SelectedIndex(), 0)].id }

	find := newButton(T("Szukaj w sieci"), theme.SearchIcon(), nil).withTip(T("Szuka kamer ONVIF w sieci lokalnej"))
	build := func() {
		k := ipKinds[max(kind.SelectedIndex(), 0)]
		if k.port > 0 {
			port.SetPlaceHolder(strconv.Itoa(k.port))
		} else {
			port.SetPlaceHolder("")
		}
		items := []*widget.FormItem{
			widget.NewFormItem(T("Nazwa"), name),
			widget.NewFormItem(T("Typ"), kind),
			widget.NewFormItem(T("Adres"), container.NewBorder(nil, nil, nil, find, host)),
		}
		if k.port > 0 {
			items = append(items, widget.NewFormItem(T("Port"), port))
		}
		if k.id != "tapo" {
			items = append(items, widget.NewFormItem(T("Login"), user))
		}
		items = append(items, widget.NewFormItem(T("Hasło"), pass))
		switch k.id {
		case "rtsp":
			host.SetPlaceHolder(T("192.168.1.50 lub pełny adres rtsp://…"))
		case "http":
			host.SetPlaceHolder(T("192.168.1.50 lub pełny adres http://…"))
		default:
			host.SetPlaceHolder("192.168.1.50")
		}
		if k.id != "tapo" && k.id != "dvrip" {
			items = append(items, widget.NewFormItem("", imouOn))
			if imouOn.Checked {
				items = append(items,
					widget.NewFormItem("AppId", imouApp),
					widget.NewFormItem("AppSecret", imouSecret),
					widget.NewFormItem(T("Centrum danych"), imouDC),
					widget.NewFormItem(T("Kamera Imou"), container.NewBorder(nil, nil, nil, imouFetch, imouDev)))
			}
		}
		switch k.id {
		case "onvif":
			hint.SetText(T("Kamera sama poda adresy strumieni (profile ONVIF). ONVIF trzeba zwykle włączyć w ustawieniach kamery; login i hasło to konto ONVIF lub konto kamery."))
		case "rtsp":
			mainPath.SetPlaceHolder("/stream1")
			items = append(items,
				widget.NewFormItem(T("Producent"), brand),
				widget.NewFormItem(T("Strumień główny"), mainPath),
				widget.NewFormItem(T("Strumień podglądu"), subPath))
			hint.SetText(T("Ścieżki strumieni są w instrukcji kamery. Można też wpisać w polu Adres cały adres rtsp://…"))
		case "http":
			mainPath.SetPlaceHolder("/video.mjpg, /stream.m3u8")
			items = append(items, widget.NewFormItem(T("Ścieżka"), mainPath))
			hint.SetText(T("Strumień MJPEG, HLS (.m3u8) lub H.264 przez HTTP. Można też wpisać w polu Adres cały adres http(s)://…"))
		case "tapo":
			hint.SetText(T("Hasło: konto kamery z aplikacji Tapo (Kamera > Ustawienia > Ustawienia zaawansowane > Konto kamery) lub hasło konta TP-Link."))
		case "dvrip":
			items = append(items, widget.NewFormItem(T("Kanał"), channel))
			hint.SetText(T("Kamery i rejestratory XMeye / Sofia (port 34569). Kanał 0 to pierwsza kamera rejestratora."))
		}
		if imouOn.Checked && k.id != "tapo" && k.id != "dvrip" {
			hint.SetText(T("AppId i AppSecret: open.imoulife.com (konto Imou Life) → My App → Application Information. Obraz idzie lokalnie, chmura Imou tylko steruje (30 000 darmowych zapytań miesięcznie)."))
		}
		form.Objects = []fyne.CanvasObject{widget.NewForm(items...)}
		form.Refresh()
		if d != nil {
			d.Resize(fyne.NewSize(600, d.MinSize().Height))
		}
	}
	kind.OnChanged = func(string) { build() }
	imouOn.OnChanged = func(bool) { build() }
	imouDCID := func() string { return imouDataCenters[max(imouDC.SelectedIndex(), 0)].ID }

	readForm := func() *ipForm {
		f := &ipForm{
			Kind: kindID(), Host: strings.TrimSpace(host.Text),
			User: strings.TrimSpace(user.Text), Pass: pass.Text,
			MainPath: strings.TrimSpace(mainPath.Text), SubPath: strings.TrimSpace(subPath.Text),
		}
		f.Port, _ = strconv.Atoi(strings.TrimSpace(port.Text))
		f.Channel, _ = strconv.Atoi(strings.TrimSpace(channel.Text))
		if f.Kind == "tapo" {
			f.User = ""
		}
		if f.Kind == "http" && f.MainPath == "" {
			f.MainPath = "/"
		}
		if imouOn.Checked {
			f.ImouApp, f.ImouSecret = strings.TrimSpace(imouApp.Text), strings.TrimSpace(imouSecret.Text)
			f.ImouDC, f.ImouDevice = imouDCID(), imouDevIDs[imouDev.Selected]
		}
		return f
	}

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	rtlAlign(status)
	spin := newSpinner(spinnerFrames)
	var test, add, cancel *ptrButton
	setBusy := func(on bool, text string) {
		spin.run(on)
		status.SetText(text)
		for _, b := range []*ptrButton{test, add, find, imouFetch} {
			if on {
				b.Disable()
			} else {
				b.Enable()
			}
		}
	}

	// check connects to the camera; returns the sources and their picture sizes
	check := func(f *ipForm) (*ipCamera, error) {
		mainSrc, subSrc, err := f.sources()
		if err != nil {
			return nil, err
		}
		if f.ImouApp != "" && f.ImouDevice == "" {
			return nil, errors.New(T("Wybierz kamerę Imou (przycisk „Pobierz”)"))
		}
		c := &ipCamera{Kind: f.Kind, Main: mainSrc}
		f.imouTo(c)
		r, err := probeSource(mainSrc)
		if err != nil {
			return nil, err
		}
		c.W, c.H, c.Codec = r.W, r.H, r.Codec
		if subSrc != "" && subSrc != mainSrc {
			if r, err := probeSource(subSrc); err == nil {
				c.Sub, c.SubW, c.SubH, c.SubCodec = subSrc, r.W, r.H, r.Codec
			}
		}
		return c, nil
	}
	describe := func(c *ipCamera) string {
		s := fmt.Sprintf(T("Połączono: strumień główny %d×%d"), c.W, c.H)
		if c.Sub != "" {
			s += fmt.Sprintf(T(", podgląd %d×%d"), c.SubW, c.SubH)
		}
		return s
	}

	test = newButton(T("Testuj"), theme.MediaPlayIcon(), func() {
		f := readForm()
		setBusy(true, T("Łączenie z kamerą…"))
		go func() {
			c, err := check(f)
			fyne.Do(func() {
				setBusy(false, "")
				if err != nil {
					status.SetText(TJ("Błąd: ", err.Error()))
					return
				}
				status.SetText(describe(c))
			})
		}()
	}).withTip(T("Łączy się z kamerą i sprawdza obraz"))

	addText, addIcon := T("Dodaj"), theme.ContentAddIcon()
	if edit != "" {
		addText, addIcon = T("Zapisz"), theme.DocumentSaveIcon()
	}
	add = newButton(addText, addIcon, func() {
		n := strings.TrimSpace(name.Text)
		if n == "" {
			status.SetText(T("Podaj nazwę kamery"))
			return
		}
		if (n != edit && cameraExists(n)) || strings.HasPrefix(n, hiddenPrefix) {
			status.SetText(fmt.Sprintf(T("Kamera „%s” już istnieje"), n))
			return
		}
		f := readForm()
		setBusy(true, T("Łączenie z kamerą…"))
		go func() {
			c, err := check(f)
			fyne.Do(func() {
				setBusy(false, "")
				if err == nil {
					err = save(n, c)
				}
				if err != nil {
					status.SetText(TJ("Błąd: ", err.Error()))
					return
				}
				d.Hide()
			})
		}()
	})
	add.Importance = widget.HighImportance
	cancel = newButton(T("Anuluj"), theme.CancelIcon(), func() { d.Hide() })

	imouFetch.OnTapped = func() {
		app, secret, dc := strings.TrimSpace(imouApp.Text), strings.TrimSpace(imouSecret.Text), imouDCID()
		if app == "" || secret == "" {
			status.SetText(T("Wpisz AppId i AppSecret aplikacji Imou"))
			return
		}
		selected := imouDevIDs[imouDev.Selected]
		setBusy(true, T("Pobieram kamery z chmury Imou…"))
		go func() {
			devs, err := imouDevices(app, secret, dc)
			fyne.Do(func() {
				setBusy(false, "")
				if err != nil {
					status.SetText(TJ("Błąd: ", err.Error()))
					return
				}
				if len(devs) == 0 {
					status.SetText(T("Konto Imou nie ma kamer (inne centrum danych?)"))
					return
				}
				setImouDevices(devs, selected)
				if imouDev.Selected == "" {
					status.SetText(T("Wybierz kamerę z listy"))
				}
			})
		}()
	}

	find.OnTapped = func() {
		setBusy(true, T("Szukam kamer ONVIF w sieci…"))
		go func() {
			devices, err := onvif.DiscoveryStreamingDevices()
			fyne.Do(func() {
				setBusy(false, "")
				if err != nil {
					status.SetText(TJ("Błąd: ", err.Error()))
					return
				}
				if len(devices) == 0 {
					status.SetText(T("Nie znaleziono kamer ONVIF. Sprawdź, czy ONVIF jest włączony w kamerze, albo wpisz adres ręcznie."))
					return
				}
				showFoundCameras(w, devices, func(dev onvif.DiscoveryDevice) {
					u, err := url.Parse(dev.URL)
					if err != nil {
						return
					}
					kind.SetSelectedIndex(0) // ONVIF
					host.SetText(u.Hostname())
					port.SetText(u.Port())
					if name.Text == "" {
						name.SetText(strings.TrimSpace(dev.Name))
					}
					status.SetText(T("Wpisz login i hasło kamery, potem „Testuj” lub „Dodaj”."))
				})
			})
		}()
	}

	kind.SetSelectedIndex(0)
	if prefill != nil {
		for i, k := range ipKinds {
			if k.id == prefill.Kind {
				kind.SetSelectedIndex(i)
			}
		}
		host.SetText(prefill.Host)
		if prefill.Port > 0 {
			port.SetText(strconv.Itoa(prefill.Port))
		}
	}
	brand.SetSelectedIndex(len(brandNames) - 1)
	mainPath.SetText("")
	subPath.SetText("")
	title := T("Dodaj kamerę IP")
	if prefill != nil && edit != "" {
		title = T("Edytuj kamerę")
		name.SetText(edit)
		user.SetText(prefill.User)
		pass.SetText(prefill.Pass)
		mainPath.SetText(prefill.MainPath)
		subPath.SetText(prefill.SubPath)
		channel.SetText(strconv.Itoa(prefill.Channel))
		if prefill.ImouApp != "" {
			imouApp.SetText(prefill.ImouApp)
			imouSecret.SetText(prefill.ImouSecret)
			for i, dc := range imouDataCenters {
				if dc.ID == prefill.ImouDC {
					imouDC.SetSelectedIndex(i)
				}
			}
			setImouDevices([]imouDevice{{ID: prefill.ImouDevice, Name: T("zapisana"), Model: "Imou"}}, prefill.ImouDevice)
			imouOn.SetChecked(true)
		}
		for _, b := range rtspBrands {
			if b.main == prefill.MainPath && b.sub == prefill.SubPath {
				brand.Selected = b.name // without OnChanged: the paths are set
				brand.Refresh()
			}
		}
	}

	content := container.NewBorder(nil,
		container.NewVBox(hint, container.NewBorder(nil, nil, spin, nil, status),
			container.NewHBox(test, layout.NewSpacer(), cancel, add)),
		nil, nil, form)
	d = dialog.NewCustomWithoutButtons(title, content, w)
	d.Show()
	d.Resize(fyne.NewSize(600, d.MinSize().Height))
}

// showFoundCameras lists cameras found with ONVIF discovery; picked fills the form.
func showFoundCameras(w fyne.Window, devices []onvif.DiscoveryDevice, picked func(onvif.DiscoveryDevice)) {
	var d *dialog.CustomDialog
	list := container.NewVBox()
	for _, dev := range devices {
		dev := dev
		host := dev.URL
		if u, err := url.Parse(dev.URL); err == nil {
			host = u.Host
		}
		title := strings.TrimSpace(dev.Name + " " + dev.Hardware)
		if title == "" {
			title = T("Kamera ONVIF")
		}
		b := newButton(title+" – "+host, theme.ComputerIcon(), func() {
			d.Hide()
			picked(dev)
		})
		b.Alignment = widget.ButtonAlignLeading
		list.Add(b)
	}
	d = dialog.NewCustom(T("Znalezione kamery"), T("Anuluj"), container.NewVScroll(list), w)
	d.Resize(fyne.NewSize(520, 300))
	d.Show()
}
