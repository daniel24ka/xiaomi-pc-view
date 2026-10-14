//go:build devtest

package main

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <string.h>
static void move_to_desktop(void *dpy, unsigned long win, long desk) {
	Display *d = (Display *)dpy;
	XEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.xclient.type = ClientMessage;
	ev.xclient.window = win;
	ev.xclient.message_type = XInternAtom(d, "_NET_WM_DESKTOP", False);
	ev.xclient.format = 32;
	ev.xclient.data.l[0] = desk;
	ev.xclient.data.l[1] = 1;
	XSendEvent(d, DefaultRootWindow(d), False, SubstructureRedirectMask | SubstructureNotifyMask, &ev);
	XFlush(d);
}
*/
import "C"

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// XC_DESKTOPTEST=1: move the window to another workspace and back, it must not hide to tray.
func init() {
	if os.Getenv("XC_DESKTOPTEST") == "" {
		return
	}
	devHook = func(u *ui) {
		move := func(desk int) {
			fyne.Do(func() {
				C.move_to_desktop(unsafe.Pointer(glfw.GetX11Display()), C.ulong(glfwWindow(u.win).GetX11Window()), C.long(desk))
			})
		}
		state := func(label string) {
			fyne.Do(func() {
				fmt.Println(label, "| visible:", u.visible, "on current desktop:", wmOnCurrentDesktop(u.win),
					"iconified:", glfwWindow(u.win).GetAttrib(glfw.Iconified))
			})
		}
		time.Sleep(8 * time.Second)
		state("start")
		move(1)
		time.Sleep(3 * time.Second)
		state("on workspace 2")
		move(0)
		time.Sleep(3 * time.Second)
		state("back")
		fyne.Do(func() { glfwWindow(u.win).Iconify() })
		time.Sleep(2 * time.Second)
		state("real minimize")
		fyne.Do(u.app.Quit)
	}
}
