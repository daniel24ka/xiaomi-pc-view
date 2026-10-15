//go:build windows

// xc-glprobe tests the OpenGL of the Windows graphics driver for Xiaomi Cam (exit code 0:
// OpenGL 2.1 or newer, 1: not). A program of its own: the app links opengl32.dll, so the
// app can't load the system one apart from the one it was started with.
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procLoadLibraryExW  = kernel32.NewProc("LoadLibraryExW")
	gdi32               = syscall.NewLazyDLL("gdi32.dll")
	procChoosePixelFmt  = gdi32.NewProc("ChoosePixelFormat")
	procSetPixelFormat  = gdi32.NewProc("SetPixelFormat")
	user32              = syscall.NewLazyDLL("user32.dll")
	procCreateWindowExW = user32.NewProc("CreateWindowExW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procGetDC           = user32.NewProc("GetDC")
	procReleaseDC       = user32.NewProc("ReleaseDC")
)

func main() {
	if nativeGLOK() {
		os.Exit(0)
	}
	os.Exit(1)
}

func utf16(s string) uintptr {
	p, _ := syscall.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

// pixelFormat - PIXELFORMATDESCRIPTOR
type pixelFormat struct {
	Size, Version                                      uint16
	Flags                                              uint32
	PixelType, ColorBits, RedBits, RedShift            uint8
	GreenBits, GreenShift, BlueBits, BlueShift         uint8
	AlphaBits, AlphaShift, AccumBits, AccumRedBits     uint8
	AccumGreenBits, AccumBlueBits, AccumAlphaBits      uint8
	DepthBits, StencilBits, AuxBuffers, LayerType, Res uint8
	LayerMask, VisibleMask, DamageMask                 uint32
}

// nativeGLOK reports whether the system's OpenGL is 2.1 or newer: a hidden window, a WGL
// context of the system opengl32.dll and its GL_VERSION.
func nativeGLOK() bool {
	sys, _ := syscall.UTF16PtrFromString(filepath.Join(os.Getenv("SystemRoot"), "System32", "opengl32.dll"))
	lib, _, _ := procLoadLibraryExW.Call(uintptr(unsafe.Pointer(sys)), 0, 0)
	if lib == 0 {
		return false
	}
	proc := func(name string) uintptr {
		p, _ := syscall.GetProcAddress(syscall.Handle(lib), name)
		return p
	}
	createCtx, makeCurrent, deleteCtx, getString :=
		proc("wglCreateContext"), proc("wglMakeCurrent"), proc("wglDeleteContext"), proc("glGetString")
	if createCtx == 0 || makeCurrent == 0 || deleteCtx == 0 || getString == 0 {
		return false
	}

	hwnd, _, _ := procCreateWindowExW.Call(0, utf16("STATIC"), utf16(""), 0x80000000 /* WS_POPUP */, 0, 0, 1, 1, 0, 0, 0, 0)
	if hwnd == 0 {
		return true // can't test: leave it to the system
	}
	defer procDestroyWindow.Call(hwnd)
	hdc, _, _ := procGetDC.Call(hwnd)
	defer procReleaseDC.Call(hwnd, hdc)

	pfd := pixelFormat{Version: 1, Flags: 0x4 | 0x20 | 0x1, ColorBits: 32, DepthBits: 24} // window, OpenGL, double buffer
	pfd.Size = uint16(unsafe.Sizeof(pfd))
	pf, _, _ := procChoosePixelFmt.Call(hdc, uintptr(unsafe.Pointer(&pfd)))
	if pf == 0 {
		return false
	}
	if ok, _, _ := procSetPixelFormat.Call(hdc, pf, uintptr(unsafe.Pointer(&pfd))); ok == 0 {
		return false
	}
	ctx, _, _ := syscall.SyscallN(createCtx, hdc)
	if ctx == 0 {
		return false
	}
	defer syscall.SyscallN(deleteCtx, ctx)
	if ok, _, _ := syscall.SyscallN(makeCurrent, hdc, ctx); ok == 0 {
		return false
	}
	defer syscall.SyscallN(makeCurrent, 0, 0)
	v, _, _ := syscall.SyscallN(getString, 0x1F02) // GL_VERSION, e.g. "4.6.0 NVIDIA 560.94", "1.1.0"
	if v == 0 {
		return false
	}
	return glVersionOK(cString(v))
}

// cString reads the C string at p (a pointer returned by glGetString)
func cString(p uintptr) string {
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	var b []byte
	for i := 0; i < 64; i++ {
		c := *(*byte)(unsafe.Add(ptr, i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// glVersionOK: "major.minor..." is at least 2.1
func glVersionOK(v string) bool {
	major, minor, _ := strings.Cut(v, ".")
	if i := strings.IndexFunc(minor, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		minor = minor[:i]
	}
	ma, err1 := strconv.Atoi(major)
	mi, _ := strconv.Atoi(minor)
	return err1 == nil && (ma > 2 || ma == 2 && mi >= 1)
}
