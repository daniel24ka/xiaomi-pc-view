//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// XC_RECMODETEST=1: record 6 s in "each" and "grid" modes with a temporary relay camera.
func init() {
	if os.Getenv("XC_RECMODETEST") == "" {
		return
	}
	devHook = func(u *ui) {
		cam := u.camera.Selected
		time.Sleep(8 * time.Second)
		_, _ = streams.New("Test relay", rtspURL(cam))
		setRecordCamera("Test relay", false)
		fmt.Println("record cameras:", recordCameras())
		recs, err := startRecordings(recordEach, cam, nil, 3*time.Second)
		fmt.Println("each with relay unchecked: started", len(recs), err)
		for _, r := range recs {
			<-r.done
			fmt.Println("FILE", r.path)
		}
		fyne.Do(func() {
			u.app.Preferences().SetString(prefRecordMode, recordEach)
			u.showSettings()
		})
		time.Sleep(time.Second)
		fyne.Do(func() {
			if f, err := os.Create(os.Getenv("XC_RECMODETEST") + "/settings.jpg"); err == nil {
				_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
				_ = f.Close()
			}
		})
		time.Sleep(500 * time.Millisecond)
		setRecordCamera("Test relay", true)
		streams.Delete("Test relay")
		fyne.Do(u.app.Quit)
	}
}
