// Xiaomi Cam – desktop viewer for Xiaomi Mi Home cameras (e.g. Outdoor Camera CW700S).
//
// One binary: embedded go2rtc (with CS2 connection via Xiaomi servers, so it works from
// any network like the Mi Home app), Fyne UI with tray icon, PTZ, digital zoom, spotlight.
// Video decoding uses system ffmpeg.
package main

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/xccore"
	"image"
	"image/jpeg"
	"os"
	"runtime/debug"
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
	"github.com/go-gl/glfw/v3.4/glfw"
)

// startDebug prints a start stage (XC_STARTDEBUG=1): finding why the app doesn't start on a PC
func startDebug(stage string) {
	if os.Getenv("XC_STARTDEBUG") == "1" {
		fmt.Fprintln(os.Stderr, time.Now().Format("15:04:05.000"), "xiaomi-cam:", stage)
	}
}

// devHook runs a self-test (devtest.go, build with -tags devtest)
var devHook func(u *ui)

// version is set at build time: -ldflags "-X main.version=v1.0.0"
var version = "dev"

var qualities = xccore.XiaomiQualities

// quality - a picture quality of a camera: name in the list and picture size
type quality struct {
	name string
	w, h int
}

// cameraQualities returns the qualities of the camera and the index of the selected one:
// Xiaomi 2.5K/720p/SD, IP camera main/sub stream.
func cameraQualities(name string) ([]quality, int) {
	if c := ipCam(name); c != nil {
		size := func(w, h int) (int, int) {
			if w <= 0 || h <= 0 {
				return 1920, 1080 // not known (yet): the picture is fitted in
			}
			return w, h
		}
		w, h := size(c.W, c.H)
		// names: the picture height; "main" / "sub" only when both are the same
		qs := []quality{{fmt.Sprintf("%dp", h), w, h}}
		if c.Sub != "" {
			sw, sh := size(c.SubW, c.SubH)
			qs = append(qs, quality{fmt.Sprintf("%dp", sh), sw, sh})
			if sh == h {
				qs[0].name, qs[1].name = T("Główny"), T("Podgląd")
			}
		}
		return qs, min(max(c.Quality, 0), len(qs)-1)
	}
	var qs []quality
	for _, q := range qualities {
		qs = append(qs, quality{q.Name, q.W, q.H})
	}
	return qs, xccore.XiaomiQuality(name)
}

// currentQuality returns the selected quality of the camera.
func currentQuality(name string) quality {
	qs, i := cameraQualities(name)
	return qs[i]
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
	status *statusLabel

	activity   *spinner // spinner next to the status line: an action in progress
	busyCount  int      // actions in progress (busy.go)
	snapButton *ptrButton
	picWait    *pictureWait // sound on/off: waiting for the new decoder's picture
	night      *ptrSelect   // night vision mode
	// wallpaper mode (wallpaper.go)
	wallpaper     bool
	wallStop      chan struct{} // keepBelow running
	remapping     bool          // the window is being re-mapped (window type, frame): not a minimize
	remapSeq      int
	wallpaperItem *fyne.MenuItem
	trayMenu      *fyne.Menu
	capNight      fyne.CanvasObject
	// "run now" buttons of AI tasks being checked (Settings)
	aiRunButtons []*ptrButton

	camera     *ptrSelect
	quality    *segmented
	connection *ptrSelect
	connBox    fyne.CanvasObject // the connection list (Xiaomi only)
	// light, siren, night mode: in a panel opened by this button (bottom bar)
	camControls *ptrButton
	menuButton  *ptrButton
	// controls on the picture (overlay.go)
	pictureControls, bottomControls, statusChip *fyne.Container
	soundBox                                    *fyne.Container
	liveDot                                     *canvas.Circle
	histPreview                                 *histPreview
	lastThumb                                   int64 // the unix minute of the last thumbnail
	controlsTimer                               *time.Timer
	// onVideoMouse shows the controls lying on the picture (they hide after a while)
	onVideoMouse func()
	light        *ptrCheck
	brightness   *ptrSlider
	siren        *ptrCheck
	sound        *ptrCheck
	bars         []fyne.CanvasObject
	exitView     *fyne.Container // video-only overlay: exit/move buttons (top right), pan/tilt (bottom left)
	viewOnly     bool
	hideTimer    *time.Timer
	normalSize   fyne.Size // window size before video-only mode

	hist                                 history // SD card playback
	recButton                            *ptrButton
	ai                                   *aiEngine            // AI tasks (Settings → AI)
	points                               [presetSlots]*preset // camera points 1-3
	grid                                 bool                 // grid of all cameras
	tiles                                []*tile
	videoArea                            *fyne.Container
	gridButton                           *ptrButton
	caps                                 camCaps // features of the selected camera
	capPTZ, capPoints, capZoom, capLight fyne.CanvasObject
	capOverlayPTZ, capOverlayZoom        fyne.CanvasObject
	pointButtons                         [presetSlots]*ptrButton
	pointNames                           [presetSlots]*canvas.Text // the names under the numbers
	pointWraps                           [presetSlots]*fyne.Container
	gotFrame                             bool // first frame of the current playback arrived
	decodeW                              int  // width the video is decoded to (tier, see decodeSize)
	decodeStable                         int  // seconds the window has needed another decode size

	camZoom    float64 // camera zoom 1.0-9.0
	zoomTimer  *time.Timer
	zoomDone   func() // ends the busy indicator of the pending zoom command
	zoomLabel  *widget.Label
	zoomSlider *zoomSlider // vertical, over the picture in video-only mode

	dragX, dragY float32      // accumulated mouse drag for camera moves
	mover        *mover       // pan/tilt steps
	current      atomic.Value // name of the selected camera (string), for background goroutines

	syncing bool // skip On* handlers while syncing controls with camera state
	visible bool
}

func main() {
	// video frames are large and short-lived: collect garbage more often to keep memory low
	debug.SetGCPercent(50)
	startHidden := false
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp": // MCP server for AI assistants (stdio)
			if err := runMCP(); err != nil {
				fmt.Fprintln(os.Stderr, "xiaomi-cam mcp:", err)
				os.Exit(1)
			}
			return
		case "--tray": // start hidden in the tray (used by the MCP server)
			startHidden = true
		}
	}
	if showRunningInstance() {
		startDebug("another instance runs")
		return
	}
	prepareOpenGL() // Windows without a GPU driver: software OpenGL (gl_windows.go)
	if err := startBackend(); err != nil {
		fmt.Fprintln(os.Stderr, "xiaomi-cam:", err)
		os.Exit(1)
	}
	startDebug("backend started")

	loadRecordings() // SD card files found before (history.go)
	evStore.load()   // camera events read before (events.go)
	a := fyneapp.NewWithID("pl.lawinski.xiaomi-cam")
	a.SetIcon(appIcon)
	a.Settings().SetTheme(appTheme{})
	lang = a.Preferences().StringWithFallback(prefLanguage, "pl")
	appPrefs = a.Preferences()
	u := &ui{app: a, win: a.NewWindow("Xiaomi Cam")}
	u.current.Store("")
	u.caps = allCaps
	u.camZoom = 1
	u.mover = newMover(func() string { return u.current.Load().(string) }, func(err error) {
		fyne.Do(func() { u.status.SetText(T("Obrót: ") + err.Error()) })
	})
	u.build()
	xccore.OnZoomState = func(name string, z float64) {
		fyne.Do(func() {
			if name == u.camera.Selected && z != u.camZoom {
				u.camZoom = z
				u.showZoom(z)
			}
		})
	}
	u.setupTray()
	onShowRequest(func() { fyne.Do(u.show) })
	go func() {
		for range time.Tick(time.Second) {
			fyne.Do(func() {
				u.historyStatus()
				u.updateRecordButton()
				u.saveGeometry()
				u.checkDecodeSize()
			})
		}
	}()
	a.Lifecycle().SetOnStarted(func() {
		onMinimize(u.win, u.hide)     // minimize = hide to tray
		onCursorLeave(u.win, func() { // the mouse left the window: hide the video-only buttons soon
			if u.viewOnly {
				u.hideOverlayAfter(time.Second)
			}
		})
		u.restoreGeometry(geomFramed)
		if a.Preferences().Bool(prefWallpaper) {
			time.AfterFunc(time.Second, func() { fyne.Do(func() { u.setWallpaper(true) }) })
		}
	})
	a.Lifecycle().SetOnStopped(func() {
		u.player.stop() // no frames to the UI after it stopped
		stopAllRecordings()
	})

	u.win.Resize(fyne.NewSize(1100, 700))
	u.win.SetCloseIntercept(u.hide)
	onRecordingChanged = func(string) { fyne.Do(u.updateRecordButton) }
	pointsChanged = func() { fyne.Do(u.loadPoints) }
	u.ai = newAIEngine(aiActions{notify: func(title, text string) {
		a.SendNotification(fyne.NewNotification(title, text))
	}})
	initHTTPAPI(func(name string, z float64) {
		fyne.Do(func() {
			if name == u.camera.Selected {
				u.camZoom = z
				u.showZoom(z)
			}
		})
	})
	if !startHidden {
		u.show()
	}
	if a.Preferences().String(prefLayout) == "grid" {
		u.setGrid(true)
	}
	if len(cameraNames()) == 0 && !startHidden {
		showChooseCamera(u.win, u.selectCamera)
	}
	u.startEventWatcher()
	if devHook != nil {
		go devHook(u) // only in builds with -tags devtest
	}
	if path := os.Getenv("XIAOMI_CAM_SCREENSHOT"); path != "" {
		go u.screenshotAndQuit(path) // for development: window screenshot after 15 s
	}
	startDebug("UI starts")
	a.Run()
	startDebug("UI ended")
	saveRecordings()
	evStore.save()
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
	u.video.onMouseMove = u.pictureControlsActive
	u.onVideoMouse = u.pictureControlsActive
	u.video.onCameraDrag = u.cameraDrag
	u.video.onCameraDragEnd = u.cameraDragEnd
	u.video.onScroll = func(up bool) {
		if up {
			u.zoomBy(zoomStep)
		} else {
			u.zoomBy(-zoomStep)
		}
	}
	u.zoomLabel = widget.NewLabel(fmt.Sprintf("%.1f×", u.camZoom))
	u.video.canResize = func() bool { return u.viewOnly && !u.win.FullScreen() }
	u.video.onResize = u.resizeKeepAspect
	u.video.onEdgePress = func(dir int) bool { return wmMoveResize(u.win, dir) }
	u.status = newStatusLabel()
	u.activity = newSpinner(spinnerFrames)
	u.player.onFrame = func(img *image.RGBA, gen int64) {
		fyne.Do(func() {
			if gen != u.player.gen.Load() {
				return // late frame of a stopped playback
			}
			if !u.gotFrame {
				u.gotFrame = true
				u.setLive(!u.hist.on)
				if !u.hist.on {
					u.status.SetText(fmt.Sprintf(T("Na żywo (%s)"), u.quality.Selected))
				}
			}
			u.historyFrame()
			u.video.setFrame(img)
			u.captureThumb(img)
			u.pictureShown(&u.player, gen)
		})
	}
	u.player.onStatus = func(s string) {
		fyne.Do(func() {
			if u.grid {
				return // a late status of the stopped single view (the grid has its own)
			}
			u.video.setStatus(s)
			u.status.SetText(s)
		})
	}

	u.camera = newSelect(cameraNames(), func(name string) {
		if !u.syncing && u.grid {
			u.selectTile(name)
			return
		}
		if !u.syncing {
			if u.hist.on {
				u.goLive() // history is per camera
			}
			u.restart()
			go u.loadCameraState()
			u.updateCaps()
			u.loadPoints()
		}
	})
	var cNames []string
	for _, c := range connections {
		cNames = append(cNames, T(c.name))
	}
	u.quality = newSegmented(func(string) {
		if u.syncing {
			return
		}
		if i := u.quality.SelectedIndex(); ipCam(u.camera.Selected) != nil {
			u.changeIPQuality(i)
		} else {
			u.changeSource(map[string]string{"subtype": qualities[i].Subtype})
		}
	})
	u.connection = newSelect(cNames, func(string) {
		if !u.syncing {
			u.changeSource(map[string]string{"p2p": connections[u.connection.SelectedIndex()].p2p})
		}
	})

	u.buildHistory()
	u.gridButton = newButton("", theme.GridIcon(), func() { u.setGrid(!u.grid) })
	u.recButton = newButton(T("Nagraj"), recordIcon, u.toggleRecording)
	u.snapButton = newButton("", theme.MediaPhotoIcon(), u.snapshot).withTip(T("Zdjęcie w pełnej rozdzielczości do folderu zdjęć (S)"))
	// tooltips of the named controls
	u.camera.tip = T("Wybór kamery")
	u.quality.tip = T("Jakość obrazu z kamery: 2.5K, 720p lub SD (kamera IP: strumień główny lub podgląd)")
	u.connection.tip = T("Połączenie: zdalne przez serwery Xiaomi (z każdej sieci) lub lokalne w sieci LAN")
	u.gridButton.tip = T("Siatka wszystkich kamer / jedna kamera (w siatce: kliknij kamerę, aby nią sterować; podwójne kliknięcie – pełny widok)")
	u.recButton.tip = T("Nagrywanie do MP4 – start/stop (tryb i kamery w Ustawieniach)")
	u.hist.button.tip = T("Historia nagrań z karty SD / powrót na żywo")
	// lists of a fixed width (the window can be narrow); the labels are in the tooltips
	fixed := func(w float32, o fyne.CanvasObject) fyne.CanvasObject {
		return container.NewGridWrap(fyne.NewSize(w, o.MinSize().Height), o)
	}
	u.connBox = fixed(100, u.connection)
	u.menuButton = u.appMenuButton()
	top := container.NewHBox(
		fixed(140, u.camera),
		u.quality,
		u.connBox,
		layout.NewSpacer(),
		// view: grid, history, snapshot, video only, full screen
		u.gridButton,
		u.hist.button,
		u.recButton,
		u.snapButton,
		// app: settings; cameras, language, help in the menu
		newButton("", theme.SettingsIcon(), u.showSettings).withTip(T("Ustawienia: nagrywanie, foldery, zadania AI, powiadomienia")),
		u.menuButton,
	)

	ptz := func(icon fyne.Resource, op int) *holdButton {
		b := newHoldButton(icon, func() {
			u.pictureControlsActive()
			u.mover.Press(op)
		}, u.mover.Release).withTip(ptzTip(op))
		b.Importance = widget.LowImportance // on the pill's background
		return b
	}
	u.light = newCheck(T("Reflektor"), func(on bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propLight, on, u.light)
		}
	})
	u.brightness = newSlider(0, 100)
	u.brightness.Step = 10
	u.brightness.OnChangeEnded = func(v float64) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propBrightness, int(v), u.brightness)
		}
	}
	u.siren = newCheck(T("Syrena"), func(on bool) {
		u.win.Canvas().Unfocus()
		if u.syncing {
			return
		}
		if !on {
			u.setProp(propSiren, false, u.siren)
			return
		}
		dialog.ShowConfirm(T("Syrena"), T("Włączyć syrenę alarmową kamery?"), func(ok bool) {
			if ok {
				u.setProp(propSiren, true, u.siren)
			} else {
				u.syncing = true
				u.siren.SetChecked(false)
				u.syncing = false
			}
		}, u.win)
	})
	var nightNames []string
	for _, m := range nightModes {
		nightNames = append(nightNames, T(m.Name))
	}
	u.night = newSelect(nightNames, func(string) {
		u.win.Canvas().Unfocus()
		if i := u.night.SelectedIndex(); !u.syncing && i >= 0 {
			u.setProp(propNightShot, nightModes[i].Value, u.night)
		}
	})
	u.night.PlaceHolder = T("Tryb nocny")
	u.night.tip = T("Tryb nocny: podczerwień (czarno-biały obraz) lub kolor z reflektorem; auto przełącza o zmroku")
	u.capNight = container.NewBorder(nil, nil, widget.NewIcon(moonIcon), nil, container.NewGridWrap(fyne.NewSize(230, u.night.MinSize().Height), u.night))
	u.sound = newCheck("", func(bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			p := &u.player
			if t := u.tileOf(u.camera.Selected); u.grid && t != nil {
				p = &t.player
			}
			u.busyUntilNewPicture(p, u.sound)
			u.refresh()
		}
	})

	u.light.tip = T("Reflektor kamery (białe światło)")
	u.brightness.tip = T("Jasność reflektora")
	u.siren.tip = T("Syrena alarmowa kamery (z potwierdzeniem)")
	u.sound.tip = T("Dźwięk z kamery")
	// groups of controls on the picture (overlay.go), hidden when the selected camera doesn't
	// have the feature (caps.go)
	pad := [4]fyne.CanvasObject{
		ptz(theme.NavigateBackIcon(), motorLeft),
		ptz(theme.MoveUpIcon(), motorUp),
		ptz(theme.MoveDownIcon(), motorDown),
		ptz(theme.NavigateNextIcon(), motorRight),
	}
	zoom := container.NewHBox(
		overlayButton(theme.ZoomOutIcon(), T("Oddal – zoom kamery (−, kółko myszy)"), func() { u.zoomBy(-zoomStep) }),
		u.zoomLabel,
		overlayButton(theme.ZoomInIcon(), T("Przybliż – zoom kamery do 9× (+, kółko myszy)"), func() { u.zoomBy(zoomStep) }),
		overlayButton(theme.ZoomFitIcon(), T("Zoom 1× (0)"), func() { u.zoomBy(-maxZoom) }))
	u.capLight = container.NewBorder(nil, nil, u.light, nil, u.brightness)
	u.camControls = u.cameraControlsButton()
	u.camControls.Importance = widget.LowImportance
	u.soundBox = container.NewHBox(widget.NewIcon(theme.VolumeUpIcon()), u.sound)
	u.pictureControls = u.buildPictureControls(pad, u.buildPoints(), zoom)
	bottomBox := container.NewVBox(u.hist.bar)
	u.bars = []fyne.CanvasObject{top, bottomBox}
	exitBtn := newButton(T("Pokaż panele"), theme.VisibilityIcon(), func() { u.setViewOnly(false) }).withTip(T("Wróć do widoku z panelami i ramką okna (Esc, prawy przycisk myszy)"))
	exitBtn.Importance = widget.LowImportance
	moveH := newMoveHandle(u.win, widget.LowImportance)
	moveH.tip = T("Przesuń okno – przeciągnij")
	// button lies on the picture, on a translucent background
	exitBox := container.NewStack(canvas.NewRectangle(overlayBg),
		container.NewHBox(moveH, exitBtn))
	// pan/tilt buttons for video-only mode (dragging the picture doesn't move the camera there);
	// pressing keeps the overlay visible
	overlayPTZ := func(icon fyne.Resource, op int) *holdButton {
		b := newHoldButton(icon, func() {
			u.showExitButton()
			u.mover.Press(op)
		}, u.mover.Release)
		b.Importance = widget.LowImportance // no button background, only the light overlay one
		return b.withTip(ptzTip(op))
	}
	ptzBox := container.NewStack(canvas.NewRectangle(overlayBg), container.NewHBox(
		overlayPTZ(theme.NavigateBackIcon(), motorLeft),
		overlayPTZ(theme.MoveUpIcon(), motorUp),
		overlayPTZ(theme.MoveDownIcon(), motorDown),
		overlayPTZ(theme.NavigateNextIcon(), motorRight),
	))
	u.zoomSlider = newZoomSlider(1, maxZoom, zoomStep)
	u.zoomSlider.tip = T("Zoom kamery – przeciągnij lub kliknij")
	u.zoomSlider.SetValue(u.camZoom)
	u.zoomSlider.onChanged = func(v float64) {
		u.showExitButton()
		u.zoomLabel.SetText(fmt.Sprintf("%.1f×", v))
	}
	u.zoomSlider.onChangeEnded = func(v float64) {
		u.showExitButton()
		u.setZoom(v)
	}
	zoomBox := container.NewStack(canvas.NewRectangle(overlayBg),
		container.New(layout.NewCustomPaddedLayout(4, 4, 4, 8), u.zoomSlider))
	u.capOverlayPTZ, u.capOverlayZoom = ptzBox, zoomBox
	u.exitView = container.NewBorder(
		container.New(layout.NewCustomPaddedLayout(10, 0, 0, 10),
			container.NewHBox(layout.NewSpacer(), exitBox)), // 10 px from top and right edge
		container.New(layout.NewCustomPaddedLayout(0, 10, 10, 0),
			container.NewHBox(ptzBox, layout.NewSpacer())), // 10 px from bottom and left edge
		nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, 0, 10), // 10 px from right edge, centered
			container.NewVBox(layout.NewSpacer(), zoomBox, layout.NewSpacer())))
	u.exitView.Hide()
	u.videoArea = container.NewStack(u.video, u.pictureControls, u.exitView)
	u.updateGridButton()
	u.win.SetContent(container.NewBorder(top, bottomBox, nil, nil, u.videoArea))

	u.win.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
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
	// arrows move the camera while held (key down - move, key up - stop)
	arrows := map[fyne.KeyName]int{fyne.KeyLeft: motorLeft, fyne.KeyRight: motorRight, fyne.KeyUp: motorUp, fyne.KeyDown: motorDown}
	if dc, ok := u.win.Canvas().(desktop.Canvas); ok {
		dc.SetOnKeyDown(func(e *fyne.KeyEvent) {
			if op, ok := arrows[e.Name]; ok && u.caps.PTZ {
				u.mover.Press(op)
			}
		})
		dc.SetOnKeyUp(func(e *fyne.KeyEvent) {
			if _, ok := arrows[e.Name]; ok {
				u.mover.Release()
			}
		})
	}
	u.win.Canvas().SetOnTypedRune(func(r rune) {
		switch r {
		case '+', '=':
			u.zoomBy(zoomStep)
		case '-':
			u.zoomBy(-zoomStep)
		case '0':
			u.zoomBy(-maxZoom)
		case '1', '2', '3':
			u.pointPressed(int(r - '0'))
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
		u.updateCaps()
		u.loadPoints()
	}
}

func (u *ui) setupTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	show := fyne.NewMenuItem(T("Pokaż"), u.show)
	hide := fyne.NewMenuItem(T("Ukryj do zasobnika"), u.hide)
	u.wallpaperItem = fyne.NewMenuItem(T("Tryb tapety (obraz w tle pulpitu)"), func() { u.setWallpaper(!u.wallpaper) })
	quit := fyne.NewMenuItem(T("Zakończ"), u.app.Quit)
	quit.IsQuit = true
	u.trayMenu = fyne.NewMenu("Xiaomi Cam", show, hide, u.wallpaperItem, fyne.NewMenuItemSeparator(), quit)
	desk.SetSystemTrayMenu(u.trayMenu)
	desk.SetSystemTrayIcon(appIcon)
}

// hide closes the window to tray and stops video, so the camera is not streaming in the background.
func (u *ui) hide() {
	if u.remapping {
		return // not a minimize: the wallpaper mode re-maps the window
	}
	if u.wallpaper {
		u.leaveWallpaperHidden()
	}
	u.saveGeometry()
	u.visible = false
	u.win.Hide()
	u.player.stop()
	u.stopTiles()
}

func (u *ui) show() {
	if u.wallpaper {
		u.setWallpaper(false) // "Show" brings the normal window back
		return
	}
	u.visible = true
	u.win.Show()
	u.restoreGeometry(u.geometryKey()) // the WM may place a re-shown window elsewhere
	if win := glfwWindow(u.win); win != nil && win.GetAttrib(glfw.Iconified) == glfw.True {
		win.Restore() // it was minimized to tray
	}
	u.win.RequestFocus()
	if u.grid {
		for _, t := range u.tiles {
			u.startTile(t, false)
		}
		return
	}
	u.restart()
}

func (u *ui) selectCamera(name string) {
	u.camerasChanged()
	u.camera.SetSelected(name) // triggers restart
}

// restart (re)starts video of the selected camera with current settings.
func (u *ui) restart() {
	name := u.camera.Selected
	if !u.visible {
		return
	}
	if u.grid {
		if t := u.tileOf(name); t != nil {
			u.cameraSelected(name)
			u.startTile(t, false)
		}
		return
	}
	if name == "" {
		u.player.stop()
		u.video.clear(T("Brak kamery – kliknij „Dodaj”"), false)
		return
	}
	q := u.syncControls(name)
	u.gotFrame = false
	u.setLive(false)
	u.status.SetText(T("Łączenie z kamerą…"))
	u.video.clear(T("Łączenie z kamerą…"), true)
	w, h := u.decodeSize(q.w, q.h)
	u.decodeW = w
	go u.player.start(rtspURL(name), w, h, u.sound.Checked)
}

// syncControls sets the quality and connection lists from the camera source;
// returns the selected quality.
func (u *ui) syncControls(name string) quality {
	u.current.Store(name)
	src := cameraSource(name)
	u.syncing = true
	defer func() { u.syncing = false }()
	qs, qi := cameraQualities(name)
	var names []string
	for _, q := range qs {
		names = append(names, q.name)
	}
	u.quality.Options = names
	u.quality.SetSelectedIndex(qi)
	// connection: only Xiaomi cameras (servers / LAN)
	if src == "" {
		u.connBox.Hide()
		return qs[qi]
	}
	u.connBox.Show()
	ci := 1
	for i, c := range connections {
		if c.p2p == sourceParam(src, "p2p") {
			ci = i
		}
	}
	u.connection.SetSelectedIndex(ci)
	return qs[qi]
}

// refresh restarts the decoder with the current settings (sound, decode size) without
// reconnecting to the camera: the new decoder joins the same stream and replaces the old
// one when it has a picture.
func (u *ui) refresh() {
	name := u.camera.Selected
	if u.grid {
		if t := u.tileOf(name); t != nil && u.visible {
			u.startTile(t, true)
		}
		return
	}
	if !u.visible || name == "" || u.video.frame == nil {
		u.restart()
		return
	}
	q := currentQuality(name)
	w, h := u.decodeSize(q.w, q.h)
	u.decodeW = w
	go u.player.switchTo(rtspURL(name), w, h, u.sound.Checked)
}

func (u *ui) changeSource(params map[string]string) {
	name := u.camera.Selected
	if name == "" {
		return
	}
	u.player.stop() // close camera session, it will be redialed with new params
	if t := u.tileOf(name); t != nil {
		t.player.stop()
	}
	time.Sleep(300 * time.Millisecond)
	if err := setSourceParams(name, params); err != nil {
		dialog.ShowError(err, u.win)
	}
	u.restart()
}

// changeIPQuality switches an IP camera to its main (0) or sub (1) stream.
func (u *ui) changeIPQuality(q int) {
	name := u.camera.Selected
	u.player.stop()
	if t := u.tileOf(name); t != nil {
		t.player.stop()
	}
	if err := setIPQuality(name, q); err != nil {
		dialog.ShowError(err, u.win)
	}
	u.restart()
}

func (u *ui) loadCameraState() {
	if cameraSource(u.camera.Selected) == "" {
		return // not a Xiaomi camera: no state to read
	}
	name := u.camera.Selected
	props, err := getProps(name, propLight, propBrightness, propSiren, propNightShot)
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
			case p.Siid == propNightShot.Siid && p.Piid == propNightShot.Piid && p.Code == 0:
				v, _ := p.Value.(float64)
				if i := nightModeByValue(int(v)); i >= 0 {
					u.night.SetSelectedIndex(i)
				}
			}
		}
	})
}

// setProp sets a camera property; the control w is disabled until the camera answers.
func (u *ui) setProp(p miotProp, value any, w fyne.Disableable) {
	name := u.camera.Selected
	done := u.busy(nil, w)
	go func() {
		err := setProp(name, p, value)
		if err != nil {
			fyne.Do(func() { dialog.ShowError(err, u.win) })
		}
		u.loadCameraState()
		fyne.Do(done)
	}()
}

// snapshot saves a full resolution JPEG from the camera stream (the window shows a
// downscaled picture) to the snapshots folder from the settings.
func (u *ui) snapshot() {
	name := u.camera.Selected
	if name == "" || u.snapButton.isBusy() {
		return
	}
	u.status.SetText(T("Zapisuję zdjęcie…"))
	done := u.busy(u.snapButton)
	go func() {
		path, err := saveSnapshot(name)
		fyne.Do(func() {
			done()
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.status.SetText(T("Zapisano ") + path)
		})
	}()
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
	u.saveGeometry() // of the mode we are leaving
	u.notMinimized() // the frame on/off re-maps the window
	u.viewOnly = on
	u.showBars(!on)
	if on {
		u.pictureControls.Hide()
	} else {
		u.pictureControls.Show()
	}
	u.win.SetPadded(!on)
	if on {
		u.normalSize = u.win.Canvas().Size()
		setDecorated(u.win, false)
		if !u.restoreGeometry(geomFrameless) {
			aspect := float32(9) / 16
			if f := u.video.frame; f != nil {
				aspect = float32(f.Bounds().Dy()) / float32(f.Bounds().Dx())
			}
			w := u.normalSize.Width
			u.win.Resize(fyne.NewSize(w, w*aspect))
		}
		if f := u.video.frame; f != nil {
			setAspectRatio(u.win, f.Bounds().Dx(), f.Bounds().Dy())
		} else {
			setAspectRatio(u.win, 16, 9)
		}
		u.showExitButton()
		return
	}
	u.exitView.Hide()
	if u.win.FullScreen() {
		u.win.SetFullScreen(false)
	}
	setDecorated(u.win, true)
	setAspectRatio(u.win, 0, 0)
	if !u.restoreGeometry(geomFramed) && !u.normalSize.IsZero() {
		u.win.Resize(u.normalSize)
	}
	u.normalSize = fyne.Size{}
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
	u.hideOverlayAfter(3 * time.Second) // no mouse movement inside the window
}

// hideOverlayAfter hides the video-only buttons after d (restarted on every call).
func (u *ui) hideOverlayAfter(d time.Duration) {
	if u.hideTimer != nil {
		u.hideTimer.Stop()
	}
	u.hideTimer = time.AfterFunc(d, func() {
		fyne.Do(func() { u.exitView.Hide() })
	})
}

// cameraDrag turns the camera when the picture is dragged: like grabbing the view,
// dragging right turns the camera left. The camera moves while the mouse keeps dragging
// in that direction and stops on release (cameraDragEnd).
func (u *ui) cameraDrag(dx, dy float32) {
	if !u.caps.PTZ {
		return
	}
	if u.viewOnly {
		return // video-only mode has pan/tilt buttons instead
	}
	const threshold = 15 // px before the camera starts moving
	u.dragX += dx
	u.dragY += dy
	op := 0
	switch {
	case u.dragX >= threshold && u.dragX >= abs32(u.dragY):
		op = motorLeft
	case u.dragX <= -threshold && -u.dragX >= abs32(u.dragY):
		op = motorRight
	case u.dragY >= threshold:
		op = motorUp
	case u.dragY <= -threshold:
		op = motorDown
	}
	if op != 0 {
		u.mover.Press(op)
	}
}

func (u *ui) cameraDragEnd() {
	u.dragX, u.dragY = 0, 0
	u.mover.Release()
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// setLanguage switches the app language (menu).
func (u *ui) setLanguage(code string) {
	{
		if code == lang {
			return
		}
		lang = code
		u.app.Preferences().SetString(prefLanguage, code)
		// rebuild UI with new texts; live video keeps playing (the new view gets its frames)
		wasGrid := u.grid
		if wasGrid {
			u.stopTiles()
			u.tiles, u.grid = nil, false
		}
		wasHistory := u.hist.on
		if wasHistory {
			u.player.stop()
			_ = setPlayback(u.current.Load().(string), 0, 0)
		}
		sound := u.sound.Checked
		frame := u.video.frame
		u.build()
		u.syncing = true
		u.sound.SetChecked(sound)
		u.syncing = false
		u.setupTray()
		if wasGrid {
			u.setGrid(true)
		} else if wasHistory || frame == nil {
			u.restart()
		} else {
			u.syncControls(u.camera.Selected)
			u.video.setFrame(frame)
			u.gotFrame = true
			u.status.SetText(fmt.Sprintf(T("Na żywo (%s)"), u.quality.Selected))
		}
		go u.loadCameraState()
		u.updateCaps()
		u.loadPoints()
	}
}

func (u *ui) showShortcuts() {
	text := T(`Strzałki – obrót kamery
Przeciąganie obrazu myszą – obrót kamery
+ / − (także na klawiaturze numerycznej), kółko myszy – zoom kamery (do 9×)
0 – zoom 1×
1, 2, 3 – punkty kamery (zapisane pozycje)
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

// resizeKeepAspect resizes the frameless window by a dragged edge, keeping the picture
// aspect ratio, so there are no black bars.
func (u *ui) resizeKeepAspect(dx, dy float32) {
	aspect := float32(9) / 16
	if f := u.video.frame; f != nil {
		aspect = float32(f.Bounds().Dy()) / float32(f.Bounds().Dx())
	}
	size := u.win.Canvas().Size()
	w := size.Width + dx
	if dx == 0 || (dy != 0 && dy/aspect > dx) {
		w = size.Width + dy/aspect
	}
	w = max(w, 320)
	u.win.Resize(fyne.NewSize(w, w*aspect))
}

const (
	zoomStep = 0.5
	maxZoom  = 9.0 // hlCameraVideoMaxZoomScale in the Mi Home plugin
)

// zoomBy changes the camera zoom. Only the last value is sent after a short pause,
// so scrolling the wheel doesn't flood the camera with commands.
func (u *ui) zoomBy(delta float64) {
	if !u.caps.Zoom {
		return
	}
	u.setZoom(u.camZoom + delta)
}

// setZoom sets the camera zoom; only the last value is sent after a short pause.
func (u *ui) setZoom(z float64) {
	u.camZoom = min(max(z, 1), maxZoom)
	u.showZoom(u.camZoom)
	if u.zoomTimer != nil && u.zoomTimer.Stop() {
		u.zoomDone() // the previous value wasn't sent
	}
	name, zoom := u.camera.Selected, u.camZoom
	setZoomState(name, zoom)
	done := u.busy(nil)
	u.zoomDone = done
	u.zoomTimer = time.AfterFunc(300*time.Millisecond, func() {
		err := cameraZoom(name, zoom)
		fyne.Do(func() {
			done()
			if err != nil {
				u.status.SetText("Zoom: " + err.Error())
			}
		})
	})
}

// showZoom updates the zoom label and the video-only zoom slider.
func (u *ui) showZoom(z float64) {
	u.zoomLabel.SetText(fmt.Sprintf("%.1f×", z))
	if u.zoomSlider != nil {
		u.zoomSlider.SetValue(z)
	}
}

// decodeTiers - widths the video is decoded to. Decoding to the size shown on the screen
// instead of the full 2560x1440 saves most of CPU and memory (camera zoom is done by the
// camera, so no detail is lost).
var decodeTiers = []int{1280, 1920, 2560}

// decodeSize returns the smallest tier not smaller than the picture on the screen
// (in pixels), limited by the stream size w x h.
func (u *ui) decodeSize(w, h int) (int, int) {
	px := int(u.video.Size().Width * u.win.Canvas().Scale())
	if px <= 0 {
		px = 1280
	}
	dw := w
	for _, t := range decodeTiers {
		if t >= px {
			dw = min(t, w)
			break
		}
	}
	return dw, dw * h / w
}

// checkDecodeSize restarts live video when the window was resized to another tier
// (e.g. full screen); called every second.
func (u *ui) checkDecodeSize() {
	if u.grid {
		return // tiles have a fixed decode size
	}
	if !u.visible || u.hist.on || u.decodeW == 0 || u.camera.Selected == "" {
		return
	}
	q := currentQuality(u.camera.Selected)
	if w, _ := u.decodeSize(q.w, q.h); w != u.decodeW {
		u.decodeStable++
		if u.decodeStable >= 2 { // the same new size for 2 s - resizing has finished
			u.decodeStable = 0
			u.refresh()
		}
		return
	}
	u.decodeStable = 0
}

func ptzTip(op int) string {
	switch op {
	case motorLeft:
		return T("Obróć w lewo – przytrzymaj (klawisz ←)")
	case motorRight:
		return T("Obróć w prawo – przytrzymaj (klawisz →)")
	case motorUp:
		return T("Obróć w górę – przytrzymaj (klawisz ↑)")
	default:
		return T("Obróć w dół – przytrzymaj (klawisz ↓)")
	}
}
