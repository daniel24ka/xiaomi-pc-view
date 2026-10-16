package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// Wallpaper mode (tray menu): the window stays where it is, with its size and look, but
// under all windows like the desktop background, on all workspaces, and the mouse goes
// through it. Remembered: the app starts in it again.
//
// It stays below the desktop icons: GNOME's desktop icons (DING) are a transparent desktop
// window too, which lowers itself whenever any window is mapped, so the window lowers
// itself after it (keepBelow), and is then between the icons and the background.

const prefWallpaper = "wallpaper"

var lowerInterval = 10 * time.Millisecond // keepBelow

// setWallpaper turns the wallpaper mode on or off (UI thread).
func (u *ui) setWallpaper(on bool) {
	if on == u.wallpaper {
		return
	}
	win := glfwWindow(u.win)
	if !wallpaperSupported(u.win) {
		dialog.ShowInformation(T("Tryb tapety"), T(wallpaperUnsupported), u.win)
		return
	}
	if on && !u.visible {
		u.show()
	}
	if on {
		u.saveGeometry()
	}
	// the same place and size; the WM would put a re-mapped window elsewhere
	x, y := win.GetPos()
	size := u.win.Canvas().Size()
	place := func() {
		u.win.Resize(size)
		win.SetPos(x, y)
	}
	if !on {
		place = func() {
			setDecorated(u.win, !u.viewOnly)
			u.restoreGeometry(u.geometryKey())
		}
	}
	u.wallpaper = on
	u.keepBelow(false) // on again after the re-map
	u.updateWallpaperItem()
	u.app.Preferences().SetBool(prefWallpaper, on)
	u.exitView.Hide()
	u.remap(on, place, func() {
		place() // the WM may have moved it while mapping (e.g. to the focused monitor)
		if !on {
			u.win.RequestFocus()
		}
	})
}

// remap hides the window, changes its type (the WM reads it when the window is mapped),
// calls before (still hidden: geometry, frame) and shows it again; then calls after.
func (u *ui) remap(desktop bool, before, after func()) {
	win := glfwWindow(u.win)
	u.remapping = true
	win.Hide()
	time.AfterFunc(200*time.Millisecond, func() { // the WM handles the unmap
		fyne.Do(func() {
			wmSetDesktop(u.win, desktop)
			before()
			win.Show()
			time.AfterFunc(300*time.Millisecond, func() {
				fyne.Do(func() {
					after()
					if desktop && u.wallpaper {
						u.keepBelow(true)
					}
					u.video.img.Refresh()
				})
			})
			u.notMinimized()
		})
	})
}

// keepBelow keeps the window at the bottom of the desktop windows while on. UI thread.
// A click on the desktop raises the icons window and it lowers itself at once, below this
// window; the WM tells X11 clients nothing about it, so the window lowers itself often
// enough to be back before the next frame is drawn (a no-op for the WM when already there).
func (u *ui) keepBelow(on bool) {
	if u.wallStop != nil {
		close(u.wallStop)
		u.wallStop = nil
	}
	if on {
		u.wallStop = make(chan struct{})
		wmKeepLowered(u.win, lowerInterval, u.wallStop)
	}
}

// notMinimized: the window is re-mapped (window type, frame on/off); the WM reports it as
// minimize and restore, sometimes late. For 2 s that is not a hide to tray, and the window
// is restored if it ended up minimized. UI thread.
func (u *ui) notMinimized() {
	u.remapping = true
	u.remapSeq++
	seq := u.remapSeq
	time.AfterFunc(2*time.Second, func() {
		fyne.Do(func() {
			if u.remapSeq != seq {
				return // a later re-map is guarded
			}
			u.remapping = false
			if win := glfwWindow(u.win); win != nil && win.GetAttrib(glfw.Iconified) == glfw.True {
				wmRestore(win)
			}
		})
	})
}

func (u *ui) updateWallpaperItem() {
	if u.wallpaperItem != nil {
		u.wallpaperItem.Checked = u.wallpaper
		u.refreshTray()
	}
}

// leaveWallpaperHidden ends the wallpaper mode when the window goes to the tray: the
// window becomes a normal one again for the next show.
func (u *ui) leaveWallpaperHidden() {
	u.wallpaper = false
	u.keepBelow(false)
	u.updateWallpaperItem()
	u.app.Preferences().SetBool(prefWallpaper, false)
	u.win.Hide()
	wmSetDesktop(u.win, false)
	setDecorated(u.win, !u.viewOnly)
}

func (u *ui) refreshTray() {
	if u.trayMenu != nil {
		u.trayMenu.Refresh()
	}
}
