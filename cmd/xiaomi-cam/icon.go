package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"fyne.io/fyne/v2"
)

// appIcon - simple camera lens icon, drawn at startup.
var appIcon = fyne.NewStaticResource("xiaomi-cam.png", drawIcon(128))

func drawIcon(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	orange := color.NRGBA{R: 0xff, G: 0x67, B: 0x00, A: 0xff} // Mi orange
	dark := color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	lens := color.NRGBA{R: 0x3a, G: 0x7b, B: 0xd5, A: 0xff}
	c := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c) / c
			switch {
			case d < 0.30:
				img.Set(x, y, lens)
			case d < 0.55:
				img.Set(x, y, dark)
			case d < 0.95:
				img.Set(x, y, orange)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
