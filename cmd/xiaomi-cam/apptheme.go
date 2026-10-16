package main

import (
	"embed"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The app look: a dark theme (graphite, one accent color), the Geist font and Lucide icons
// (tools/icons.sh). Colors, sizes and icons are only looked up here, so the look is changed
// in one place; the values are plain constants (no work while drawing).

//go:embed fonts/Geist-Regular.ttf
var fontRegular []byte

//go:embed fonts/Geist-SemiBold.ttf
var fontBold []byte

//go:embed fonts/GeistMono-Medium.ttf
var fontMono []byte

//go:embed icons/*.svg
var iconFiles embed.FS

var (
	resRegular = fyne.NewStaticResource("Geist-Regular.ttf", fontRegular)
	resBold    = fyne.NewStaticResource("Geist-SemiBold.ttf", fontBold)
	resMono    = fyne.NewStaticResource("GeistMono-Medium.ttf", fontMono)
)

// palette - the colors of the app (variant A of the design)
var palette = struct {
	bg, surface, raised, line, text, muted, accent, onAccent, live, rec color.NRGBA
}{
	bg:       hex(0x0E1116),
	surface:  hex(0x171B22),
	raised:   hex(0x222834),
	line:     hex(0x262C38),
	text:     hex(0xE8EAEE),
	muted:    hex(0x9AA3B2),
	accent:   hex(0xF5A524),
	onAccent: hex(0x0E1116),
	live:     hex(0x22C55E),
	rec:      hex(0xEF4444),
}

// overlayBg - the translucent background of controls lying on the picture
var overlayBg = color.NRGBA{R: 0x0E, G: 0x11, B: 0x16, A: 0xB8}

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}

func alpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

type appTheme struct{}

func (appTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	p := palette
	switch n {
	case theme.ColorNameBackground:
		return p.bg
	case theme.ColorNameButton, theme.ColorNameInputBackground, theme.ColorNameMenuBackground,
		theme.ColorNameOverlayBackground:
		return p.surface
	case theme.ColorNameHeaderBackground:
		return hex(0x12161D)
	case theme.ColorNameHover:
		return alpha(hex(0xFFFFFF), 0x14)
	case theme.ColorNamePressed:
		return alpha(hex(0xFFFFFF), 0x22)
	case theme.ColorNameInputBorder:
		return p.line
	case theme.ColorNameSeparator:
		return hex(0x1F2430)
	case theme.ColorNameForeground:
		return p.text
	case theme.ColorNamePlaceHolder:
		return p.muted
	case theme.ColorNameDisabled:
		return hex(0x5B6373)
	case theme.ColorNameDisabledButton:
		return hex(0x141820)
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return p.accent
	case theme.ColorNameForegroundOnPrimary:
		return p.onAccent
	case theme.ColorNameSelection:
		return alpha(p.accent, 0x40)
	case theme.ColorNameScrollBar:
		return hex(0x3A4150)
	case theme.ColorNameShadow:
		return alpha(hex(0x000000), 0x66)
	case theme.ColorNameSuccess:
		return p.live
	case theme.ColorNameError:
		return p.rec
	case theme.ColorNameWarning:
		return p.accent
	case theme.ColorNameHyperlink:
		return hex(0x7AA2FF)
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (appTheme) Font(s fyne.TextStyle) fyne.Resource {
	switch {
	case s.Monospace:
		return resMono
	case s.Bold:
		return resBold
	}
	return resRegular
}

func (appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameInputRadius:
		return 10
	case theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameScrollBarSmall:
		return 4
	}
	return theme.DefaultTheme().Size(n)
}

// themeIcons - Fyne icons replaced by Lucide icons (file names in icons/)
var themeIcons = map[fyne.ThemeIconName]string{
	theme.IconNameNavigateBack:      "chevron-left",
	theme.IconNameNavigateNext:      "chevron-right",
	theme.IconNameMoveUp:            "chevron-up",
	theme.IconNameMoveDown:          "chevron-down",
	theme.IconNameArrowDropDown:     "chevron-down",
	theme.IconNameMenuExpand:        "chevron-right",
	theme.IconNameGrid:              "layout-grid",
	theme.IconNameHistory:           "history",
	theme.IconNameMediaPhoto:        "camera",
	theme.IconNameSettings:          "settings",
	theme.IconNameMenu:              "menu",
	theme.IconNameViewZoomIn:        "plus",
	theme.IconNameViewZoomOut:       "minus",
	theme.IconNameViewZoomFit:       "scan",
	theme.IconNameVolumeUp:          "volume-2",
	theme.IconNameVolumeMute:        "volume-x",
	theme.IconNameDocumentSave:      "save",
	theme.IconNameDelete:            "trash-2",
	theme.IconNameContentAdd:        "plus",
	theme.IconNameContentRemove:     "minus",
	theme.IconNameComputer:          "cctv",
	theme.IconNameViewRefresh:       "refresh-cw",
	theme.IconNameViewFullScreen:    "maximize",
	theme.IconNameSearch:            "search",
	theme.IconNameMediaPlay:         "play",
	theme.IconNameDocumentCreate:    "pencil",
	theme.IconNameVisibility:        "eye",
	theme.IconNameVisibilityOff:     "eye-off",
	theme.IconNameMediaStop:         "square",
	theme.IconNameMediaSkipPrevious: "skip-back",
	theme.IconNameMediaSkipNext:     "skip-forward",
	theme.IconNameMediaFastRewind:   "rewind",
	theme.IconNameMediaFastForward:  "fast-forward",
	theme.IconNameMailSend:          "send",
	theme.IconNameMailCompose:       "mail",
	theme.IconNameHelp:              "circle-help",
	theme.IconNameQuestion:          "circle-help",
	theme.IconNameFolderOpen:        "folder-open",
	theme.IconNameDownload:          "download",
	theme.IconNameCancel:            "x",
	theme.IconNameWindowClose:       "x",
	theme.IconNameConfirm:           "check",
	theme.IconNameInfo:              "info",
	theme.IconNameError:             "circle-x",
	theme.IconNameWarning:           "triangle-alert",
	theme.IconNameAccount:           "circle-user-round",
}

var iconCache = map[string]fyne.Resource{}

// lucide returns an icon from icons/ (theme colored).
func lucide(name string) fyne.Resource {
	if r, ok := iconCache[name]; ok {
		return r
	}
	b, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		return theme.QuestionIcon()
	}
	r := theme.NewThemedResource(fyne.NewStaticResource(strings.ReplaceAll(name, "-", "_")+".svg", b))
	iconCache[name] = r
	return r
}

func (appTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	if name, ok := themeIcons[n]; ok {
		return lucide(name)
	}
	return theme.DefaultTheme().Icon(n)
}

func init() {
	// the cache is filled before any goroutine draws (icons are looked up on the UI thread too)
	for _, name := range themeIcons {
		lucide(name)
	}
	for _, name := range []string{"cctv", "moon", "lightbulb", "move", "video", "bell", "scan-eye", "filter", "step-back", "step-forward"} {
		lucide(name)
	}
}
