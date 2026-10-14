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

// setWallpaper turns the wallpaper mode on or off (UI thread).
func (u *ui) setWallpaper(on bool) {
	if on == u.wallpaper {
		return
	}
	win := glfwWindow(u.win)
	if win == nil || glfw.GetPlatform() != glfw.PlatformX11 {
		dialog.ShowInformation(T("Tryb tapety"), T("Tryb tapety działa tylko w X11 / XWayland."), u.win)
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
	u.keepBelow(on)
	u.updateWallpaperItem()
	u.app.Preferences().SetBool(prefWallpaper, on)
	u.exitView.Hide()
	u.remap(on, place, func() {
		if on {
			place() // the WM may have moved it while mapping
		} else {
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
					if desktop {
						wmLower(u.win)
					}
					u.video.img.Refresh()
				})
			})
			// the WM reports the re-map as minimize and restore, sometimes late: it is
			// not a hide to tray
			time.AfterFunc(2*time.Second, func() {
				fyne.Do(func() {
					u.remapping = false
					if win.GetAttrib(glfw.Iconified) == glfw.True {
						win.Restore()
					}
				})
			})
		})
	})
}

// keepBelow keeps the window at the bottom of the desktop windows while on. UI thread.
func (u *ui) keepBelow(on bool) {
	if u.wallStop != nil {
		close(u.wallStop)
		u.wallStop = nil
	}
	if !on {
		return
	}
	stop := make(chan struct{})
	u.wallStop = stop
	go func() {
		t := time.NewTicker(250 * time.Millisecond) // a no-op for the WM when already there
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			fyne.Do(func() {
				if u.wallStop == stop && !u.remapping {
					wmLower(u.win)
				}
			})
		}
	}()
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
