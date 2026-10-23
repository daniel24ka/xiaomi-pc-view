package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// The enhanced photo: like the night mode of a phone, ~15 frames of the camera are merged
// (a temporal median: the sensor noise and anything passing through – a car, a bird – go,
// the still scene stays) and mildly sharpened. While digitally zoomed, the zoomed part is
// saved too, enlarged 2x with Lanczos from the full resolution. These are the camera's own
// pixels, nothing invented.
//
// With realesrgan-ncnn-vulkan next to the app (Windows: realesrgan\ in the app folder, else
// in PATH), the zoomed part is also enlarged 4x by Real-ESRGAN into a separate file marked
// "AI": it looks sharper, but a neural network draws details that may not exist – never
// evidence.

const (
	photoFrames = 15 // tmedian radius 7: ~0.75 s of video
)

// enhancedPhoto saves the merged photo of the camera, and the 2x enlarged zoomed part when
// crop (fractions of the frame: x, y, w, h) is given. Returns the files.
func enhancedPhoto(name string, crop *[4]float64) ([]string, error) {
	dir := snapshotsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(fileName(name, "png"), ".png")
	full := filepath.Join(dir, base+"-enhanced.png")
	graph := fmt.Sprintf("[0:v]tmedian=radius=%d,select=eq(n\\,%d)", photoFrames/2, photoFrames/2)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-rtsp_transport", "tcp", "-timeout", "30000000", "-i", rtspURL(name)}
	files := []string{full}
	if crop != nil {
		zoom := filepath.Join(dir, base+"-zoom.png")
		graph += fmt.Sprintf(",split=2[a][b];[a]cas=strength=0.35[full];"+
			"[b]crop=w=iw*%.4f:h=ih*%.4f:x=iw*%.4f:y=ih*%.4f,scale=iw*2:ih*2:flags=lanczos,cas=strength=0.5[zoom]",
			crop[2], crop[3], crop[0], crop[1])
		args = append(args, "-filter_complex", graph,
			"-map", "[full]", "-frames:v", "1", full, "-map", "[zoom]", "-frames:v", "1", zoom)
		files = append(files, zoom)
	} else {
		graph += ",cas=strength=0.35[full]"
		args = append(args, "-filter_complex", graph, "-map", "[full]", "-frames:v", "1", full)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := toolCommand(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("enhanced photo: %v %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return files, nil
}

// realesrgan returns the Real-ESRGAN program and its models folder ("" - not installed).
func realesrgan() (exe, models string) {
	name := "realesrgan-ncnn-vulkan"
	if runtime.GOOS == "windows" {
		p := filepath.Join(appDir(), "realesrgan", name+".exe")
		if fileExists(p) {
			return p, filepath.Join(appDir(), "realesrgan", "models")
		}
		return "", ""
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, filepath.Join(filepath.Dir(p), "models")
	}
	return "", ""
}

func hasAIEnhance() bool {
	exe, _ := realesrgan()
	return exe != ""
}

// aiEnhance enlarges src 4x with Real-ESRGAN into "<src>-AI.png" with a warning band on top.
func aiEnhance(src string) (string, error) {
	exe, models := realesrgan()
	if exe == "" {
		return "", fmt.Errorf("Real-ESRGAN is not installed")
	}
	out := strings.TrimSuffix(src, ".png") + "-AI.png"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// tiles of 256: little GPU memory (a broadcast encoder shares the card)
	cmd := exec.CommandContext(ctx, exe, "-i", src, "-o", out, "-n", "realesrgan-x4plus", "-s", "4",
		"-t", "256", "-m", models)
	hideConsole(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("AI enhance: %v %s", err, lastLine(stderr.String()))
	}
	return out, markAI(out)
}

// markAI draws a warning band over the top of an AI image: it must never pass for a photo.
func markAI(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	const text = "AI-ENHANCED (Real-ESRGAN): details may be invented - not a photo, not evidence"
	scale := max(1, img.Bounds().Dx()/700)
	band := image.NewRGBA(image.Rect(0, 0, 7*len(text)+16, 22))
	draw.Draw(band, band.Bounds(), &image.Uniform{color.RGBA{180, 20, 20, 255}}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: band, Src: image.White, Face: basicfont.Face7x13, Dot: fixed.P(8, 16)}
	d.DrawString(text)
	// the band, scaled up with the image size (nearest: crisp letters)
	bw, bh := band.Bounds().Dx()*scale, band.Bounds().Dy()*scale
	for y := 0; y < min(bh, img.Bounds().Dy()); y++ {
		for x := 0; x < min(bw, img.Bounds().Dx()); x++ {
			img.Set(x, y, band.At(x/scale, y/scale))
		}
	}
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()
	return png.Encode(w, img)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
