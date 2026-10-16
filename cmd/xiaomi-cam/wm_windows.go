//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// Windows counterparts of wm_linux.go (user32): moving/resizing the frameless window by
// the system, and the wallpaper mode (the window as a child of the WorkerW window that
// Explorer puts between the wallpaper and the desktop icons).

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procReleaseCapture      = user32.NewProc("ReleaseCapture")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procFindWindowExW       = user32.NewProc("FindWindowExW")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procSetParent           = user32.NewProc("SetParent")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procSetLayeredAttrs     = user32.NewProc("SetLayeredWindowAttributes")
)

// setChild switches the window style between a child window and a top (popup) one.
func setChild(hwnd uintptr, on bool) {
	st, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
	if on {
		st = st&^wsPopup | wsChild
	} else {
		st = st&^wsChild | wsPopup
	}
	procSetWindowLongPtrW.Call(hwnd, gwlStyle, st)
}

// moveresize directions (the same numbers as _NET_WM_MOVERESIZE of wm_linux.go)
const (
	wmTopLeft = iota
	wmTop
	wmTopRight
	wmRight
	wmBottomRight
	wmBottom
	wmBottomLeft
	wmLeft
	wmMove
)

const (
	wmNCLButtonDown = 0x00A1
	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	gwlExStyle      = ^uintptr(19) // -20
	gwlStyle        = ^uintptr(15) // -16
	wsChild         = 0x40000000
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	lwaAlpha        = 0x2
)

// hit-test codes of the window parts, by direction
var hitTest = map[int]uintptr{
	wmTopLeft: 13, wmTop: 12, wmTopRight: 14, wmRight: 11,
	wmBottomRight: 17, wmBottom: 15, wmBottomLeft: 16, wmLeft: 10, wmMove: 2, // HTCAPTION
}

func hwndOf(w fyne.Window) uintptr {
	win := glfwWindow(w)
	if win == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(win.GetWin32Window()))
}

// wmMoveResize starts moving (wmMove) or resizing the window by the system, as a press on
// the title bar or the border would: the system follows the mouse itself (smooth, snaps).
func wmMoveResize(w fyne.Window, dir int) bool {
	hwnd := hwndOf(w)
	ht, ok := hitTest[dir]
	if hwnd == 0 || !ok {
		return false
	}
	procReleaseCapture.Call()
	// posted: the system's move loop runs in the event loop, not inside this handler
	procPostMessageW.Call(hwnd, wmNCLButtonDown, ht, 0)
	return true
}

// setAspectRatio makes the system keep the picture aspect ratio while resizing (0 - free).
func setAspectRatio(w fyne.Window, num, den int) {
	if win := glfwWindow(w); win != nil {
		if num == 0 || den == 0 {
			win.SetAspectRatio(glfw.DontCare, glfw.DontCare)
		} else {
			win.SetAspectRatio(num, den)
		}
	}
}

// wmOnCurrentDesktop: a minimized window on Windows is always a real minimize (virtual
// desktops don't report it).
func wmOnCurrentDesktop(fyne.Window) bool { return true }

func wmRestore(win *glfw.Window) { win.Restore() }

func wallpaperSupported(w fyne.Window) bool { return hwndOf(w) != 0 }

const wallpaperUnsupported = "Tryb tapety nie jest dostępny."

type rect struct{ Left, Top, Right, Bottom int32 }

func windowRect(hwnd uintptr) rect {
	var r rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func utf16(s string) uintptr {
	p, _ := syscall.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

// desktopWindows returns Progman, the window with the icons (SHELLDLL_DefView) and the
// wallpaper window (WorkerW); Progman creates WorkerW on the message 0x052C (as wallpaper
// engines do). Windows 11 24H2: both are children of Progman (child24H2); older: WorkerW is
// a top window after the one with the icons.
func desktopWindows() (progman, icons, worker uintptr, child24H2 bool) {
	progman, _, _ = procFindWindowW.Call(utf16("Progman"), 0)
	if progman == 0 {
		return
	}
	var res uintptr
	procSendMessageTimeoutW.Call(progman, 0x052C, 0xD, 0x1, 0 /* SMTO_NORMAL */, 1000, uintptr(unsafe.Pointer(&res)))
	if w, _, _ := procFindWindowExW.Call(progman, 0, utf16("WorkerW"), 0); w != 0 {
		icons, _, _ = procFindWindowExW.Call(progman, 0, utf16("SHELLDLL_DefView"), 0)
		return progman, icons, w, true
	}
	cb := syscall.NewCallback(func(top, _ uintptr) uintptr {
		if v, _, _ := procFindWindowExW.Call(top, 0, utf16("SHELLDLL_DefView"), 0); v != 0 {
			icons = v
			worker, _, _ = procFindWindowExW.Call(0, top, utf16("WorkerW"), 0)
			return 0 // stop
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return progman, icons, worker, false
}

func setLayered(hwnd uintptr, on bool) {
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	if on {
		ex |= wsExLayered
	} else {
		ex &^= wsExLayered
	}
	procSetWindowLongPtrW.Call(hwnd, gwlExStyle, ex)
	if on {
		procSetLayeredAttrs.Call(hwnd, 0, 255, lwaAlpha)
	}
}

// wmSetDesktop puts the window between the wallpaper and the desktop icons (not in the
// task bar, the mouse goes to the desktop) or makes it a top window again, at the same
// place on the screen. Windows 11 24H2 like Lively Wallpaper does; in a Windows 11 24H2 VM
// with the software OpenGL (Mesa, GDI) the window was not drawn there (not tested with a
// GPU).
func wmSetDesktop(w fyne.Window, on bool) bool {
	hwnd := hwndOf(w)
	if hwnd == 0 {
		return false
	}
	r := windowRect(hwnd)
	if !on {
		procSetParent.Call(hwnd, 0)
		setChild(hwnd, false)
		setLayered(hwnd, false)
		procSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
		return true
	}
	progman, icons, worker, child24H2 := desktopWindows()
	if worker == 0 {
		return false
	}
	parent := worker
	if child24H2 {
		// 24H2: a layered child of Progman, under the icons and above the wallpaper
		parent = progman
		setLayered(hwnd, true)
	}
	setChild(hwnd, true)
	procSetParent.Call(hwnd, parent)
	// child coordinates are relative to the parent (it starts at the virtual screen's corner)
	p := windowRect(parent)
	x, y := r.Left-p.Left, r.Top-p.Top
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	if child24H2 && icons != 0 {
		procSetWindowPos.Call(hwnd, icons, 0, 0, 0, 0, swpNoSize|swpNoMove|swpNoActivate)  // below the icons
		procSetWindowPos.Call(worker, hwnd, 0, 0, 0, 0, swpNoSize|swpNoMove|swpNoActivate) // the wallpaper below it
	}
	return true
}

// wmKeepLowered: not needed on Windows (a WorkerW child is always under the icons).
func wmKeepLowered(fyne.Window, time.Duration, chan struct{}) {}
