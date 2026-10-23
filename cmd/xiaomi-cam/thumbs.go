package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Thumbnails of the camera picture, one per minute, for the preview over the history
// timeline: taken from the live picture (what the camera is recording right then) and from
// the SD card playback, while they are shown. 192x108 JPEG (~5 KB) in
// ~/.cache/xiaomi-cam/thumbs/<did>/<unix minute>.jpg, kept 30 days.

const (
	thumbW, thumbH = 192, 108
	thumbKeep      = 30 * 24 * time.Hour
)

var thumbs = struct {
	sync.Mutex
	index map[string]map[int64]bool // did -> minutes with a thumbnail
}{index: map[string]map[int64]bool{}}

func thumbDir(did string) string {
	dir, err := os.UserCacheDir()
	if err != nil || did == "" {
		return ""
	}
	return filepath.Join(dir, "xiaomi-cam", "thumbs", did)
}

// thumbIndex returns the minutes with a thumbnail (read from the folder once). Locked.
func thumbIndex(did string) map[int64]bool {
	if m, ok := thumbs.index[did]; ok {
		return m
	}
	m := map[int64]bool{}
	thumbs.index[did] = m
	entries, _ := os.ReadDir(thumbDir(did))
	old := time.Now().Add(-thumbKeep).Unix() / 60
	for _, e := range entries {
		n, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".jpg"), 10, 64)
		if err != nil {
			continue
		}
		if n < old {
			_ = os.Remove(filepath.Join(thumbDir(did), e.Name())) // older than kept
			continue
		}
		m[n] = true
	}
	return m
}

// hasThumb: the minute (unix minute) has a thumbnail.
func hasThumb(did string, minute int64) bool {
	thumbs.Lock()
	defer thumbs.Unlock()
	return thumbIndex(did)[minute]
}

// saveThumb stores a thumbnail of the frame for the minute (if there is none yet). The
// frame is scaled down here (cheap: a sample per pixel of the thumbnail), the JPEG is
// written in the background.
func saveThumb(did string, minute int64, frame *image.RGBA) {
	if did == "" || frame == nil || hasThumb(did, minute) {
		return
	}
	small := scaleDown(frame, thumbW, thumbH)
	thumbs.Lock()
	thumbIndex(did)[minute] = true
	thumbs.Unlock()
	go func() {
		var buf bytes.Buffer
		if jpeg.Encode(&buf, small, &jpeg.Options{Quality: 70}) != nil {
			return
		}
		dir := thumbDir(did)
		_ = os.MkdirAll(dir, 0o700)
		_ = os.WriteFile(filepath.Join(dir, strconv.FormatInt(minute, 10)+".jpg"), buf.Bytes(), 0o600)
	}()
}

// addThumbFile stores a ready JPEG thumbnail of the minute (from the PC archive).
func addThumbFile(did string, minute int64, data []byte) {
	dir := thumbDir(did)
	if dir == "" || len(data) == 0 {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	if os.WriteFile(filepath.Join(dir, strconv.FormatInt(minute, 10)+".jpg"), data, 0o600) == nil {
		thumbs.Lock()
		thumbIndex(did)[minute] = true
		thumbs.Unlock()
	}
}

// thumbNear returns the file of the thumbnail nearest to t (at most max minutes away).
func thumbNear(did string, t time.Time, max int64) string {
	m := t.Unix() / 60
	thumbs.Lock()
	idx := thumbIndex(did)
	thumbs.Unlock()
	for d := int64(0); d <= max; d++ {
		for _, n := range []int64{m - d, m + d} {
			thumbs.Lock()
			ok := idx[n]
			thumbs.Unlock()
			if ok {
				return filepath.Join(thumbDir(did), strconv.FormatInt(n, 10)+".jpg")
			}
		}
	}
	return ""
}

// scaleDown samples the image to w x h (nearest pixel; the thumbnail is small anyway).
func scaleDown(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y*sh/h
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*sw/w
			si := src.PixOffset(sx, sy)
			di := dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
