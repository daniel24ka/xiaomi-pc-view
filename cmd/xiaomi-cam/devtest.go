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

// Self-test of window features with a real camera: XC_TEST=<dir> ./xiaomi-cam
// (built with -tags devtest). With XC_TEST_PTZ=1 it also moves the camera left and back.
func init() {
	devHook = func(u *ui) {
		dir := os.Getenv("XC_TEST")
		if dir == "" {
			return
		}
		step := func(d time.Duration, f func()) {
			time.Sleep(d)
			done := make(chan struct{})
			fyne.Do(func() { f(); close(done) })
			<-done
		}
		save := func(name string) {
			img := u.win.Canvas().Capture()
			fmt.Println(name, img.Bounds().Dx(), "x", img.Bounds().Dy())
			if f, err := os.Create(filepath.Join(dir, name)); err == nil {
				_ = jpeg.Encode(f, img, nil)
				_ = f.Close()
			}
		}

		step(15*time.Second, func() {
			fmt.Println("frame:", u.video.frame != nil, u.status.Text)
			fmt.Println("glfw platform X11:", glfw.GetPlatform() == glfw.PlatformX11, "x11 window:", glfwWindow(u.win).GetX11Window() != 0)
		})

		if os.Getenv("XC_TEST_PTZ") != "" {
			u.mover.press(motorLeft)
			time.Sleep(2 * time.Second)
			u.mover.release()
			u.mover.press(motorRight)
			time.Sleep(2 * time.Second)
			u.mover.release()
		}

		step(time.Second, func() { u.setViewOnly(true) })
		step(2*time.Second, func() {
			save("videoonly.jpg")
			fmt.Println("decorated:", glfwWindow(u.win).GetAttrib(glfw.Decorated))
			u.resizeKeepAspect(200, 0)
		})
		step(time.Second, func() {
			fmt.Println("after resize:", u.win.Canvas().Size())
			win := glfwWindow(u.win)
			x, y := win.GetPos()
			win.SetPos(x+50, y+30)
			fmt.Println("pos before:", x, y)
		})
		step(time.Second, func() {
			x, y := glfwWindow(u.win).GetPos()
			fmt.Println("pos after move:", x, y)
			u.setViewOnly(false)
			fmt.Println("decorated after exit:", glfwWindow(u.win).GetAttrib(glfw.Decorated))
			glfwWindow(u.win).Iconify()
		})
		step(2*time.Second, func() { fmt.Println("visible after minimize:", u.visible) })
		step(time.Second, func() { u.show() })
		step(2*time.Second, func() {
			fmt.Println("visible after show:", u.visible, "iconified:", glfwWindow(u.win).GetAttrib(glfw.Iconified))
			u.app.Quit()
		})
	}
}
