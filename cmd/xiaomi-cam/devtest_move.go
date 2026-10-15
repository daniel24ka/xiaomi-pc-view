//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// XC_MOVETEST=<dir>: video only, then an outside script drags the move handle with a real
// (uinput) mouse; when the window has moved: back to the framed view, state + screenshots.
// The window is kept above the others, so the injected mouse hits only it.
func init() {
	dir := os.Getenv("XC_MOVETEST")
	if dir == "" {
		return
	}
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, nil)
					_ = f.Close()
				}
			})
		}
		state := func(tag string) {
			fyne.DoAndWait(func() {
				win := glfwWindow(u.win)
				x, y := win.GetPos()
				w, h := win.GetSize()
				fmt.Printf("%s: win=%d,%d %dx%d visible=%v viewOnly=%v grid=%v frame=%v gotFrame=%v iconified=%v status=%q\n", tag,
					x, y, w, h, u.visible, u.viewOnly, u.grid, u.video.frame != nil, u.gotFrame,
					win.GetAttrib(glfw.Iconified) == glfw.True, u.status.Text)
			})
		}
		grid := os.Getenv("XC_MOVEGRID") != ""
		time.Sleep(8 * time.Second)
		fyne.DoAndWait(func() {
			u.setGrid(grid)
			glfwWindow(u.win).SetAttrib(glfw.Floating, glfw.True)
		})
		time.Sleep(6 * time.Second)
		state("start")
		fyne.DoAndWait(func() { u.setViewOnly(true) })
		time.Sleep(2 * time.Second)
		var hx, hy, x0, y0 int
		fyne.DoAndWait(func() {
			u.showExitButton()
			h := findMoveHandle(u.exitView)
			win := glfwWindow(u.win)
			x0, y0 = win.GetPos()
			p := fyne.CurrentApp().Driver().AbsolutePositionForObject(h)
			sc := u.win.Canvas().Scale()
			hx, hy = x0+int((p.X+h.Size().Width/2)*sc), y0+int((p.Y+h.Size().Height/2)*sc)
		})
		state("video only")
		shot("only.jpg")
		fmt.Printf("READY %d %d\n", hx, hy)
		for i := 0; i < 300; i++ { // wait for the move (30 s)
			time.Sleep(100 * time.Millisecond)
			var x, y int
			fyne.DoAndWait(func() { x, y = glfwWindow(u.win).GetPos() })
			if x != x0 || y != y0 {
				time.Sleep(2 * time.Second) // the drag ends
				break
			}
		}
		state("moved")
		shot("moved.jpg")
		fyne.DoAndWait(func() { u.setViewOnly(false) })
		for _, s := range []int{1, 3, 6} {
			time.Sleep(time.Duration(s) * time.Second)
			state(fmt.Sprintf("back +%ds", s))
		}
		shot("back.jpg")
		fmt.Println("SHOT")
		time.Sleep(3 * time.Second)
		fyne.DoAndWait(func() { glfwWindow(u.win).SetAttrib(glfw.Floating, glfw.False) })
		fyne.Do(u.app.Quit)
	}
}

func findMoveHandle(o fyne.CanvasObject) *moveHandle {
	switch v := o.(type) {
	case *moveHandle:
		return v
	case *fyne.Container:
		for _, c := range v.Objects {
			if h := findMoveHandle(c); h != nil {
				return h
			}
		}
	}
	return nil
}
