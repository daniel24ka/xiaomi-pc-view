// Xiaomi Cam – desktop viewer for Xiaomi Mi Home cameras (e.g. Outdoor Camera CW700S).
//
// One binary: embedded go2rtc (with CS2 connection via Xiaomi servers, so it works from
// any network like the Mi Home app), Fyne UI with tray icon, PTZ, digital zoom, spotlight.
// Video decoding uses system ffmpeg.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// devHook runs a self-test (devtest.go, build with -tags devtest)
var devHook func(u *ui)

// version is set at build time: -ldflags "-X main.version=v1.0.0"
var version = "dev"

var qualities = []struct {
	name, subtype string
	w, h          int
}{
	{"2.5K", "3", 2560, 1440}, // full sensor resolution, so digital zoom shows all details
	{"720p", "", 1280, 720},
	{"SD", "sd", 1280, 720},
}

var connections = []struct{ name, p2p string }{
	{"Zdalne", "server"},
	{"Lokalne (LAN)", ""},
}

type ui struct {
	app fyne.App
	win fyne.Window

	video  *videoView
	player player
	status *widget.Label

	camera     *widget.Select
	quality    *widget.Select
	connection *widget.Select
	light      *widget.Check
	brightness *widget.Slider
	siren      *widget.Check
	sound      *widget.Check
	bars       []fyne.CanvasObject
	exitView   *fyne.Container // small "exit view-only mode" button over the video
	viewOnly   bool
	hideTimer  *time.Timer
	normalSize fyne.Size // window size before video-only mode

	dragX, dragY float32      // accumulated mouse drag for camera moves
	mover        *mover       // pan/tilt steps
	current      atomic.Value // name of the selected camera (string), for background goroutines

	syncing bool // skip On* handlers while syncing controls with camera state
	visible bool
}

func main() {
	if showRunningInstance() {
		return
	}
	if err := startBackend(); err != nil {
		fmt.Fprintln(os.Stderr, "xiaomi-cam:", err)
		os.Exit(1)
	}

	a := fyneapp.NewWithID("pl.lawinski.xiaomi-cam")
	a.SetIcon(appIcon)
	lang = a.Preferences().StringWithFallback(prefLanguage, "pl")
	u := &ui{app: a, win: a.NewWindow("Xiaomi Cam")}
	u.current.Store("")
	u.mover = newMover(func() string { return u.current.Load().(string) }, func(err error) {
		fyne.Do(func() { u.status.SetText(T("Obrót: ") + err.Error()) })
	})
	u.build()
	u.setupTray()
	onShowRequest(func() { fyne.Do(u.show) })
	a.Lifecycle().SetOnStopped(u.player.stop) // no frames to the UI after it stopped

	u.win.Resize(fyne.NewSize(1100, 700))
	u.win.SetCloseIntercept(u.hide)
	u.show()
	if len(cameraNames()) == 0 {
		showAddCamera(u.win, u.selectCamera)
	}
	if devHook != nil {
		go devHook(u) // only in builds with -tags devtest
	}
	if path := os.Getenv("XIAOMI_CAM_SCREENSHOT"); path != "" {
		go u.screenshotAndQuit(path) // for development: window screenshot after 15 s
	}
	a.Run()
	u.player.stop()
}

func (u *ui) build() {
	u.video = newVideoView()
	u.video.onDoubleTap = u.toggleFullScreen
	u.video.onSecondary = func() {
		if u.viewOnly {
			u.setViewOnly(false)
		}
	}
	u.video.onMouseMove = u.showExitButton
	u.video.onCameraDrag = u.cameraDrag
	u.status = widget.NewLabel("")
	u.player.onFrame = func(img *image.RGBA) {
		fyne.Do(func() {
			if u.video.frame == nil {
				u.status.SetText(fmt.Sprintf(T("Na żywo (%s)"), u.quality.Selected))
			}
			u.video.setFrame(img)
		})
	}
	u.player.onStatus = func(s string) {
		fyne.Do(func() {
			u.video.setStatus(s)
			u.status.SetText(s)
		})
	}

	u.camera = widget.NewSelect(cameraNames(), func(string) {
		if !u.syncing {
			u.restart()
			go u.loadCameraState()
		}
	})
	var qNames, cNames []string
	for _, q := range qualities {
		qNames = append(qNames, q.name)
	}
	for _, c := range connections {
		cNames = append(cNames, T(c.name))
	}
	u.quality = widget.NewSelect(qNames, func(string) {
		if !u.syncing {
			u.changeSource(map[string]string{"subtype": qualities[u.quality.SelectedIndex()].subtype})
		}
	})
	u.connection = widget.NewSelect(cNames, func(string) {
		if !u.syncing {
			u.changeSource(map[string]string{"p2p": connections[u.connection.SelectedIndex()].p2p})
		}
	})

	top := container.NewHBox(
		widget.NewLabel(T("Kamera:")), u.camera,
		widget.NewLabel(T("Jakość:")), u.quality,
		widget.NewLabel(T("Połączenie:")), u.connection,
		layout.NewSpacer(),
		widget.NewButtonWithIcon("", theme.MediaPhotoIcon(), u.snapshot),
		widget.NewButtonWithIcon(T("Tylko obraz"), theme.VisibilityOffIcon(), func() { u.setViewOnly(true) }),
		widget.NewButtonWithIcon("", theme.ViewFullScreenIcon(), u.toggleFullScreen),
		widget.NewButtonWithIcon(T("Dodaj"), theme.ContentAddIcon(), func() { showAddCamera(u.win, u.selectCamera) }),
		widget.NewButtonWithIcon("", theme.DeleteIcon(), u.removeCamera),
		u.languageSelect(),
		widget.NewButtonWithIcon("", theme.HelpIcon(), u.showShortcuts),
	)

	ptz := func(icon fyne.Resource, op int) *holdButton {
		return newHoldButton(icon, func() { u.mover.press(op) }, u.mover.release)
	}
	u.light = widget.NewCheck(T("Reflektor"), func(on bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propLight, on)
		}
	})
	u.brightness = widget.NewSlider(0, 100)
	u.brightness.Step = 10
	u.brightness.OnChangeEnded = func(v float64) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propBrightness, int(v))
		}
	}
	u.siren = widget.NewCheck(T("Syrena"), func(on bool) {
		u.win.Canvas().Unfocus()
		if u.syncing {
			return
		}
		if !on {
			u.setProp(propSiren, false)
			return
		}
		dialog.ShowConfirm(T("Syrena"), T("Włączyć syrenę alarmową kamery?"), func(ok bool) {
			if ok {
				u.setProp(propSiren, true)
			} else {
				u.syncing = true
				u.siren.SetChecked(false)
				u.syncing = false
			}
		}, u.win)
	})
	u.sound = widget.NewCheck(T("Dźwięk"), func(bool) {
		u.win.Canvas().Unfocus()
		u.restart()
	})

	bottom := container.NewHBox(
		ptz(theme.NavigateBackIcon(), motorLeft),
		ptz(theme.MoveUpIcon(), motorUp),
		ptz(theme.MoveDownIcon(), motorDown),
		ptz(theme.NavigateNextIcon(), motorRight),
		widget.NewSeparator(),
		widget.NewButtonWithIcon("", theme.ZoomOutIcon(), func() { u.video.setZoom(u.video.zoom / 1.25) }),
		widget.NewButtonWithIcon("", theme.ZoomInIcon(), func() { u.video.setZoom(u.video.zoom * 1.25) }),
		widget.NewButtonWithIcon("", theme.ZoomFitIcon(), func() { u.video.setZoom(1) }),
		widget.NewSeparator(),
		u.light, container.NewGridWrap(fyne.NewSize(120, 36), u.brightness),
		u.siren, u.sound,
		widget.NewSeparator(),
		u.status,
	)
	u.bars = []fyne.CanvasObject{top, bottom}
	exitBtn := widget.NewButtonWithIcon(T("Pokaż panele"), theme.VisibilityIcon(), func() { u.setViewOnly(false) })
	// button lies on the picture, on a translucent background
	exitBox := container.NewStack(canvas.NewRectangle(color.NRGBA{A: 140}), exitBtn)
	u.exitView = container.NewVBox(container.NewHBox(layout.NewSpacer(), exitBox))
	u.exitView.Hide()
	u.win.SetContent(container.NewBorder(top, bottom, nil, nil, container.NewStack(u.video, u.exitView)))

	u.win.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyLeft:
			u.motor(motorLeft)
		case fyne.KeyRight:
			u.motor(motorRight)
		case fyne.KeyUp:
			u.motor(motorUp)
		case fyne.KeyDown:
			u.motor(motorDown)
		case fyne.KeyF11:
			u.toggleFullScreen()
		case fyne.KeyEscape:
			if u.win.FullScreen() {
				u.toggleFullScreen()
			} else if u.viewOnly {
				u.setViewOnly(false)
			}
		}
	})
	u.win.Canvas().SetOnTypedRune(func(r rune) {
		switch r {
		case '+', '=':
			u.video.setZoom(u.video.zoom * 1.25)
		case '-':
			u.video.setZoom(u.video.zoom / 1.25)
		case '0':
			u.video.setZoom(1)
		case 'h', 'H':
			u.setViewOnly(!u.viewOnly)
		case 'f', 'F':
			u.toggleFullScreen()
		case 's', 'S':
			u.snapshot()
		}
	})

	if names := cameraNames(); len(names) > 0 {
		u.syncing = true
		u.camera.SetSelected(names[0])
		u.syncing = false
		go u.loadCameraState()
	}
}

func (u *ui) setupTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	show := fyne.NewMenuItem(T("Pokaż"), u.show)
	hide := fyne.NewMenuItem(T("Ukryj do zasobnika"), u.hide)
	quit := fyne.NewMenuItem(T("Zakończ"), u.app.Quit)
	quit.IsQuit = true
	desk.SetSystemTrayMenu(fyne.NewMenu("Xiaomi Cam", show, hide, fyne.NewMenuItemSeparator(), quit))
	desk.SetSystemTrayIcon(appIcon)
}

// hide closes the window to tray and stops video, so the camera is not streaming in the background.
func (u *ui) hide() {
	u.visible = false
	u.win.Hide()
	u.player.stop()
}

func (u *ui) show() {
	u.visible = true
	u.win.Show()
	u.win.RequestFocus()
	u.restart()
}

func (u *ui) selectCamera(name string) {
	u.camera.Options = cameraNames()
	u.camera.SetSelected(name) // triggers restart
}

// restart (re)starts video of the selected camera with current settings.
func (u *ui) restart() {
	name := u.camera.Selected
	if !u.visible {
		return
	}
	if name == "" {
		u.player.stop()
		u.video.clear(T("Brak kamery – kliknij „Dodaj”"))
		return
	}
	u.current.Store(name)
	src := cameraSource(name)
	u.syncing = true
	qi := 1
	for i, q := range qualities {
		if q.subtype == sourceParam(src, "subtype") {
			qi = i
		}
	}
	u.quality.SetSelectedIndex(qi)
	ci := 1
	for i, c := range connections {
		if c.p2p == sourceParam(src, "p2p") {
			ci = i
		}
	}
	u.connection.SetSelectedIndex(ci)
	u.syncing = false

	q := qualities[qi]
	u.video.clear(T("Łączenie z kamerą…"))
	go u.player.start(rtspURL(name), q.w, q.h, u.sound.Checked)
}

func (u *ui) changeSource(params map[string]string) {
	name := u.camera.Selected
	if name == "" {
		return
	}
	u.player.stop() // close camera session, it will be redialed with new params
	time.Sleep(300 * time.Millisecond)
	if err := setSourceParams(name, params); err != nil {
		dialog.ShowError(err, u.win)
	}
	u.restart()
}

func (u *ui) loadCameraState() {
	name := u.camera.Selected
	props, err := getProps(name, propLight, propBrightness, propSiren)
	if err != nil {
		fyne.Do(func() { u.status.SetText("Nie udało się odczytać stanu kamery: " + err.Error()) })
		return
	}
	fyne.Do(func() {
		u.syncing = true
		defer func() { u.syncing = false }()
		for _, p := range props {
			switch {
			case p.Siid == propLight.Siid && p.Piid == propLight.Piid:
				on, _ := p.Value.(bool)
				u.light.SetChecked(on)
			case p.Siid == propBrightness.Siid && p.Piid == propBrightness.Piid:
				v, _ := p.Value.(float64)
				u.brightness.SetValue(v)
			case p.Siid == propSiren.Siid && p.Piid == propSiren.Piid:
				on, _ := p.Value.(bool)
				u.siren.SetChecked(on)
			}
		}
	})
}

func (u *ui) setProp(p miotProp, value any) {
	name := u.camera.Selected
	go func() {
		if err := setProp(name, p, value); err != nil {
			fyne.Do(func() { dialog.ShowError(err, u.win) })
		}
		u.loadCameraState()
	}()
}

func (u *ui) motor(op int) {
	u.mover.step(op)
}

func (u *ui) snapshot() {
	frame := u.video.frame
	if frame == nil {
		return
	}
	dir := filepath.Join(os.Getenv("HOME"), "Obrazy")
	if _, err := os.Stat(dir); err != nil {
		dir = os.Getenv("HOME")
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%s.jpg", u.camera.Selected, time.Now().Format("2006-01-02_15-04-05")))
	f, err := os.Create(path)
	if err == nil {
		err = jpeg.Encode(f, frame, &jpeg.Options{Quality: 92})
		_ = f.Close()
	}
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	u.status.SetText("Zapisano " + path)
}

func (u *ui) toggleFullScreen() {
	if u.win.FullScreen() {
		u.setViewOnly(false)
		return
	}
	u.viewOnly = true
	u.showBars(false)
	u.win.SetPadded(false)
	u.showExitButton()
	u.win.SetFullScreen(true)
}

// setViewOnly shows only the camera picture: no panels and no window frame, the window
// gets the picture's aspect ratio (no black bars). Exit: Esc, H, right click or
// "Pokaż panele" button shown on the picture when the mouse moves.
func (u *ui) setViewOnly(on bool) {
	if on == u.viewOnly {
		return
	}
	u.viewOnly = on
	u.showBars(!on)
	u.win.SetPadded(!on)
	if on {
		u.normalSize = u.win.Canvas().Size()
		setDecorated(u.win, false)
		aspect := float32(9) / 16
		if f := u.video.frame; f != nil {
			aspect = float32(f.Bounds().Dy()) / float32(f.Bounds().Dx())
		}
		w := u.normalSize.Width
		u.win.Resize(fyne.NewSize(w, w*aspect))
		u.showExitButton()
		return
	}
	u.exitView.Hide()
	if u.win.FullScreen() {
		u.win.SetFullScreen(false)
	}
	setDecorated(u.win, true)
	if !u.normalSize.IsZero() {
		u.win.Resize(u.normalSize)
		u.normalSize = fyne.Size{}
	}
}

func (u *ui) showBars(on bool) {
	for _, b := range u.bars {
		if on {
			b.Show()
		} else {
			b.Hide()
		}
	}
}

func (u *ui) showExitButton() {
	if !u.viewOnly {
		return
	}
	u.exitView.Show()
	if u.hideTimer != nil {
		u.hideTimer.Stop()
	}
	u.hideTimer = time.AfterFunc(3*time.Second, func() {
		fyne.Do(func() { u.exitView.Hide() })
	})
}

// cameraDrag turns the camera when the picture is dragged: like grabbing the view,
// dragging right turns the camera left. One motor step per dragStep pixels.
func (u *ui) cameraDrag(dx, dy float32) {
	const dragStep = 15
	u.dragX += dx
	u.dragY += dy
	for u.dragX >= dragStep {
		u.dragX -= dragStep
		u.mover.step(motorLeft)
	}
	for u.dragX <= -dragStep {
		u.dragX += dragStep
		u.mover.step(motorRight)
	}
	for u.dragY >= dragStep {
		u.dragY -= dragStep
		u.mover.step(motorUp)
	}
	for u.dragY <= -dragStep {
		u.dragY += dragStep
		u.mover.step(motorDown)
	}
}

func (u *ui) removeCamera() {
	name := u.camera.Selected
	if name == "" {
		return
	}
	dialog.ShowConfirm(T("Usuń kamerę"), fmt.Sprintf(T("Usunąć „%s” z aplikacji?"), name), func(ok bool) {
		if !ok {
			return
		}
		u.player.stop()
		if err := removeCamera(name); err != nil {
			dialog.ShowError(err, u.win)
		}
		u.camera.Options = cameraNames()
		u.camera.ClearSelected()
		if len(u.camera.Options) > 0 {
			u.camera.SetSelectedIndex(0)
		} else {
			u.restart()
		}
	}, u.win)
}

func (u *ui) languageSelect() *widget.Select {
	var names []string
	for _, l := range languages {
		names = append(names, l.name)
	}
	sel := widget.NewSelect(names, nil)
	for i, l := range languages {
		if l.code == lang {
			sel.SetSelectedIndex(i)
		}
	}
	sel.OnChanged = func(string) {
		code := languages[sel.SelectedIndex()].code
		if code == lang {
			return
		}
		lang = code
		u.app.Preferences().SetString(prefLanguage, code)
		// rebuild UI with new texts
		u.player.stop()
		sound := u.sound.Checked
		u.build()
		u.sound.SetChecked(sound)
		u.setupTray()
		u.restart()
		go u.loadCameraState()
	}
	return sel
}

func (u *ui) showShortcuts() {
	text := T(`Strzałki – obrót kamery
Przeciąganie obrazu myszą – obrót kamery (przy zoomie: przesuwanie powiększenia)
+ / − (także na klawiaturze numerycznej), kółko myszy – zoom
0 – zoom 1:1
H – tylko obraz (ukrywa panele), prawy przycisk myszy lub Esc – powrót
F, F11, podwójne kliknięcie – pełny ekran
S – zdjęcie do ~/Obrazy
Zamknięcie okna chowa aplikację do zasobnika`)
	dialog.ShowInformation(T("Skróty klawiaturowe"), text+"\n\nXiaomi Cam "+version, u.win)
}

func (u *ui) screenshotAndQuit(path string) {
	time.Sleep(15 * time.Second)
	fyne.Do(func() {
		if f, err := os.Create(path); err == nil {
			_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
			_ = f.Close()
		}
		u.app.Quit()
	})
}
