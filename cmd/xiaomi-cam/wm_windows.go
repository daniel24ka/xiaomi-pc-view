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
)

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
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
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

// workerW returns the window between the wallpaper and the desktop icons. Progman creates
// it on the message 0x052C (as wallpaper engines do); Windows 11 24H2 has it as a child of
// Progman, older ones as a top window after the one with the icons (SHELLDLL_DefView).
func workerW() uintptr {
	progman, _, _ := procFindWindowW.Call(utf16("Progman"), 0)
	if progman == 0 {
		return 0
	}
	var res uintptr
	procSendMessageTimeoutW.Call(progman, 0x052C, 0xD, 0x1, 0 /* SMTO_NORMAL */, 1000, uintptr(unsafe.Pointer(&res)))
	if w, _, _ := procFindWindowExW.Call(progman, 0, utf16("WorkerW"), 0); w != 0 {
		return w
	}
	var found uintptr
	cb := syscall.NewCallback(func(top, _ uintptr) uintptr {
		if v, _, _ := procFindWindowExW.Call(top, 0, utf16("SHELLDLL_DefView"), 0); v != 0 {
			found, _, _ = procFindWindowExW.Call(0, top, utf16("WorkerW"), 0)
			return 0 // stop
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// wmSetDesktop makes the window a child of WorkerW (under the icons, not in the task bar,
// the mouse goes to the desktop) or a top window again, at the same place on the screen.
func wmSetDesktop(w fyne.Window, on bool) bool {
	hwnd := hwndOf(w)
	if hwnd == 0 {
		return false
	}
	r := windowRect(hwnd)
	if !on {
		procSetParent.Call(hwnd, 0)
		procSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
		return true
	}
	ww := workerW()
	if ww == 0 {
		return false
	}
	procSetParent.Call(hwnd, ww)
	// child coordinates are relative to WorkerW (it starts at the virtual screen's corner)
	p := windowRect(ww)
	x, y := r.Left-p.Left, r.Top-p.Top
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	return true
}

// wmKeepLowered: not needed on Windows (a WorkerW child is always under the icons).
func wmKeepLowered(fyne.Window, time.Duration, chan struct{}) {}
