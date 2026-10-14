package main

/*
#cgo LDFLAGS: -lX11 -lXext
#include <X11/Xlib.h>
#include <string.h>
#include <X11/Xatom.h>
#include <X11/extensions/shape.h>

// Ask the window manager to move/resize the window (_NET_WM_MOVERESIZE, as GTK does for
// windows without frame). The WM follows the mouse itself, so it is smooth.
static void wm_moveresize(void *dpy, unsigned long win, int dir) {
	Display *d = (Display *)dpy;
	Window root = DefaultRootWindow(d), r, c;
	int rx, ry, wx, wy;
	unsigned int mask;
	XQueryPointer(d, win, &r, &c, &rx, &ry, &wx, &wy, &mask);

	// the app holds an implicit pointer grab from the button press, the WM needs it
	XUngrabPointer(d, CurrentTime);

	XEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.xclient.type = ClientMessage;
	ev.xclient.window = win;
	ev.xclient.message_type = XInternAtom(d, "_NET_WM_MOVERESIZE", False);
	ev.xclient.format = 32;
	ev.xclient.data.l[0] = rx;
	ev.xclient.data.l[1] = ry;
	ev.xclient.data.l[2] = dir;
	ev.xclient.data.l[3] = Button1;
	ev.xclient.data.l[4] = 1; // normal application
	XSendEvent(d, root, False, SubstructureRedirectMask | SubstructureNotifyMask, &ev);

	// the WM takes the button release, so tell the app the button is up,
	// otherwise the UI would treat later mouse moves as a drag
	XEvent rel;
	memset(&rel, 0, sizeof(rel));
	rel.xbutton.type = ButtonRelease;
	rel.xbutton.display = d;
	rel.xbutton.window = win;
	rel.xbutton.root = root;
	rel.xbutton.x = wx;
	rel.xbutton.y = wy;
	rel.xbutton.x_root = rx;
	rel.xbutton.y_root = ry;
	rel.xbutton.state = Button1Mask;
	rel.xbutton.button = Button1;
	rel.xbutton.same_screen = True;
	XSendEvent(d, win, True, ButtonReleaseMask, &rel);
	XFlush(d);
}

static long get_cardinal(Display *d, Window w, const char *name) {
	Atom prop = XInternAtom(d, name, True), type;
	int format;
	unsigned long n, after;
	unsigned char *data = NULL;
	long v = -1;
	if (prop == None) return -1;
	if (XGetWindowProperty(d, w, prop, 0, 1, False, XA_CARDINAL, &type, &format, &n, &after, &data) == Success && data) {
		if (n == 1 && format == 32) v = (long)(*(unsigned long *)data & 0xFFFFFFFF);
		XFree(data);
	}
	return v;
}

// 1 - the window is on the current workspace (or on all of them, or unknown)
static int wm_on_current_desktop(void *dpy, unsigned long win) {
	Display *d = (Display *)dpy;
	long wd = get_cardinal(d, win, "_NET_WM_DESKTOP");
	long cd = get_cardinal(d, DefaultRootWindow(d), "_NET_CURRENT_DESKTOP");
	if (wd < 0 || cd < 0 || wd == 0xFFFFFFFF) return 1;
	return wd == cd;
}

// Desktop (wallpaper) window: below all windows, on all workspaces, not in the task bar,
// and an empty input region, so the mouse goes through it to the desktop. The window
// type and state are read by the WM when the window is mapped: call while it is unmapped.
static void wm_set_desktop(void *dpy, unsigned long win, int on) {
	Display *d = (Display *)dpy;
	Atom type = XInternAtom(d, on ? "_NET_WM_WINDOW_TYPE_DESKTOP" : "_NET_WM_WINDOW_TYPE_NORMAL", False);
	XChangeProperty(d, win, XInternAtom(d, "_NET_WM_WINDOW_TYPE", False), XA_ATOM, 32,
		PropModeReplace, (unsigned char *)&type, 1);
	Atom state = XInternAtom(d, "_NET_WM_STATE", False);
	if (on) {
		Atom s[4] = {
			XInternAtom(d, "_NET_WM_STATE_BELOW", False),
			XInternAtom(d, "_NET_WM_STATE_STICKY", False),
			XInternAtom(d, "_NET_WM_STATE_SKIP_TASKBAR", False),
			XInternAtom(d, "_NET_WM_STATE_SKIP_PAGER", False),
		};
		XChangeProperty(d, win, state, XA_ATOM, 32, PropModeReplace, (unsigned char *)s, 4);
		XShapeCombineRectangles(d, win, ShapeInput, 0, 0, NULL, 0, ShapeSet, Unsorted);
	} else {
		XDeleteProperty(d, win, state);
		XShapeCombineMask(d, win, ShapeInput, 0, 0, None, ShapeSet); // the whole window again
	}
	XSync(d, False);
}
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// _NET_WM_MOVERESIZE directions
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

// wmMoveResize starts moving (wmMove) or resizing the window by the window manager.
// Call from a mouse button press handler. Returns false when it is not possible (not X11).
func wmMoveResize(w fyne.Window, dir int) bool {
	win := glfwWindow(w)
	if win == nil || glfw.GetPlatform() != glfw.PlatformX11 {
		return false
	}
	C.wm_moveresize(unsafe.Pointer(glfw.GetX11Display()), C.ulong(win.GetX11Window()), C.int(dir))
	return true
}

// setAspectRatio makes the WM keep the picture aspect ratio while resizing (0 - free).
func setAspectRatio(w fyne.Window, num, den int) {
	if win := glfwWindow(w); win != nil {
		if num == 0 || den == 0 {
			win.SetAspectRatio(glfw.DontCare, glfw.DontCare)
		} else {
			win.SetAspectRatio(num, den)
		}
	}
}

// wmOnCurrentDesktop reports whether the window is on the visible workspace. When the user
// switches workspaces, the WM marks windows of other workspaces hidden and GLFW reports it
// as minimizing.
func wmOnCurrentDesktop(w fyne.Window) bool {
	win := glfwWindow(w)
	if win == nil || glfw.GetPlatform() != glfw.PlatformX11 {
		return true
	}
	return C.wm_on_current_desktop(unsafe.Pointer(glfw.GetX11Display()), C.ulong(win.GetX11Window())) != 0
}

// wmSetDesktop makes the window a desktop (wallpaper) window or a normal one again; false
// when not possible (not X11). The window must be unmapped (hidden) at the time.
func wmSetDesktop(w fyne.Window, on bool) bool {
	win := glfwWindow(w)
	if win == nil || glfw.GetPlatform() != glfw.PlatformX11 {
		return false
	}
	v := C.int(0)
	if on {
		v = 1
	}
	C.wm_set_desktop(unsafe.Pointer(glfw.GetX11Display()), C.ulong(win.GetX11Window()), v)
	return true
}
