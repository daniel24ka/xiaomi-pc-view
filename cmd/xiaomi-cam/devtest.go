//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// Self-test with a real camera: XC_TEST=<dir> ./xiaomi-cam (built with -tags devtest).
// Holds pan right/left, saves frames, checks window frame in video-only mode and quits.
func init() {
	devHook = func(u *ui) {
		dir := os.Getenv("XC_TEST")
		if dir == "" {
			return
		}
		save := func(name string) {
			fyne.Do(func() {
				if f := u.video.frame; f != nil {
					fmt.Println(name, f.Bounds().Dx(), "x", f.Bounds().Dy(), time.Now().Format("15:04:05"))
					if out, err := os.Create(filepath.Join(dir, name)); err == nil {
						_ = jpeg.Encode(out, f, nil)
						_ = out.Close()
					}
				}
			})
		}
		decorated := func() int {
			v := reflect.ValueOf(u.win).Elem().FieldByName("viewport")
			return (*glfw.Window)(unsafe.Pointer(v.Pointer())).GetAttrib(glfw.Decorated)
		}
		time.Sleep(15 * time.Second)
		save("h0.jpg")
		u.mover.press(motorLeft)
		time.Sleep(3 * time.Second)
		u.mover.release()
		time.Sleep(3 * time.Second)
		save("h1.jpg")
		u.mover.press(motorRight)
		time.Sleep(3 * time.Second)
		u.mover.release()
		time.Sleep(3 * time.Second)
		save("h2.jpg")
		fyne.Do(func() {
			u.setViewOnly(true)
			fmt.Println("decorated in video-only:", decorated())
		})
		time.Sleep(2 * time.Second)
		fyne.Do(func() {
			img := u.win.Canvas().Capture()
			fmt.Println("video-only window:", img.Bounds().Dx(), "x", img.Bounds().Dy())
			if f, err := os.Create(filepath.Join(dir, "videoonly.jpg")); err == nil {
				_ = jpeg.Encode(f, img, nil)
				_ = f.Close()
			}
		})
		time.Sleep(time.Second)
		fyne.Do(func() {
			u.resizeKeepAspect(200, 0)
			u.resizeKeepAspect(0, 90)
		})
		time.Sleep(time.Second)
		fyne.Do(func() {
			fmt.Println("after resize:", u.win.Canvas().Size(), "edge at corner:", u.video.edgeAt(fyne.NewPos(u.video.Size().Width-2, u.video.Size().Height-2)))
		})
		time.Sleep(time.Second)
		fyne.Do(func() {
			u.setViewOnly(false)
			fmt.Println("decorated after exit:", decorated())
		})
		time.Sleep(time.Second)
		fyne.Do(u.app.Quit)
	}
}
