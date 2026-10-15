//go:build devtest

package main

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
)

// XC_ADDCAM="name|url;name|url": adds IP cameras the same way as the "Add IP camera" form
// (probe, sizes, codec), prints the result and quits.
func init() {
	spec := os.Getenv("XC_ADDCAM")
	if spec == "" {
		return
	}
	devHook = func(u *ui) {
		for _, item := range strings.Split(spec, ";") {
			name, src, _ := strings.Cut(item, "|")
			kind := "http"
			if strings.HasPrefix(src, "rtsp") {
				kind = "rtsp"
			}
			r, err := probeSource(src)
			if err != nil {
				fmt.Printf("%s: probe error: %v\n", name, err)
				continue
			}
			c := &ipCamera{Kind: kind, Main: src, W: r.W, H: r.H, Codec: r.Codec}
			err = addIPCamera(name, c)
			fmt.Printf("%s: %dx%d %s audio=%q added err=%v\n", name, r.W, r.H, r.Codec, r.Audio, err)
		}
		fyne.Do(u.app.Quit)
	}
}
