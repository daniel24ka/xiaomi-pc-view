package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

// Local recording to MP4: H.265 video is copied as it comes from the camera (no
// re-encoding, almost no CPU), audio is encoded to AAC. Fragmented MP4, so the file is
// playable even if the app is killed. Recording uses the same camera session as the live
// view and works with the window hidden. Used by the UI, the HTTP API and MCP.

type recording struct {
	camera  string
	path    string
	started time.Time
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	done    chan struct{}
	limit   *time.Timer
}

var (
	recMu sync.Mutex
	recs  = map[string]*recording{}

	// onRecordingChanged is called (from any goroutine) when a recording starts or stops
	onRecordingChanged func(camera string)

	appPrefs fyne.Preferences // set in main, used for save folders
)

const (
	prefRecordingsDir = "dir.recordings"
	prefSnapshotsDir  = "dir.snapshots"
)

// xdgDir returns a user folder (VIDEOS, PICTURES) in any language, or $HOME.
func xdgDir(kind string) string {
	if out, err := exec.Command("xdg-user-dir", kind).Output(); err == nil {
		if d := strings.TrimSpace(string(out)); d != "" {
			if _, err = os.Stat(d); err == nil {
				return d
			}
		}
	}
	home, _ := os.UserHomeDir()
	return home
}

func recordingsDir() string {
	if appPrefs != nil {
		if d := appPrefs.String(prefRecordingsDir); d != "" {
			return d
		}
	}
	return xdgDir("VIDEOS")
}

func snapshotsDir() string {
	if appPrefs != nil {
		if d := appPrefs.String(prefSnapshotsDir); d != "" {
			return d
		}
	}
	return xdgDir("PICTURES")
}

// fileName - "<camera>_2026-10-14_12-30-00.<ext>" with characters safe for file systems
func fileName(camera, ext string) string {
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, camera)
	return fmt.Sprintf("%s_%s.%s", safe, time.Now().Format("2006-01-02_15-04-05"), ext)
}

// startRecording starts recording the camera; limit > 0 stops it automatically.
func startRecording(camera string, limit time.Duration) (*recording, error) {
	recMu.Lock()
	defer recMu.Unlock()
	if r := recs[camera]; r != nil {
		return r, errors.New("already recording: " + r.path)
	}
	dir := recordingsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fileName(camera, "mp4"))
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-rtsp_transport", "tcp", "-i", rtspURL(camera),
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", "copy", "-tag:v", "hvc1", // H.265 as is; hvc1 tag for players (QuickTime, browsers)
		"-c:a", "aac", "-b:a", "64k",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-y", path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	r := &recording{camera: camera, path: path, started: time.Now(), cmd: cmd, stdin: stdin, done: make(chan struct{})}
	recs[camera] = r
	go func() {
		_ = cmd.Wait()
		recMu.Lock()
		if recs[camera] == r {
			delete(recs, camera)
		}
		recMu.Unlock()
		close(r.done)
		if r.limit != nil {
			r.limit.Stop()
		}
		if onRecordingChanged != nil {
			onRecordingChanged(camera)
		}
	}()
	if limit > 0 {
		r.limit = time.AfterFunc(limit, func() { _, _ = stopRecording(camera) })
	}
	if onRecordingChanged != nil {
		onRecordingChanged(camera)
	}
	return r, nil
}

// stopRecording finishes the file (ffmpeg "q") and returns the recording.
func stopRecording(camera string) (*recording, error) {
	recMu.Lock()
	r := recs[camera]
	recMu.Unlock()
	if r == nil {
		return nil, errors.New("not recording")
	}
	_, _ = io.WriteString(r.stdin, "q")
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		_ = r.cmd.Process.Kill()
		<-r.done
	}
	return r, nil
}

func currentRecording(camera string) *recording {
	recMu.Lock()
	defer recMu.Unlock()
	return recs[camera]
}

// stopAllRecordings - on app exit
func stopAllRecordings() {
	recMu.Lock()
	var names []string
	for n := range recs {
		names = append(names, n)
	}
	recMu.Unlock()
	for _, n := range names {
		_, _ = stopRecording(n)
	}
}

// saveSnapshot saves a live JPEG snapshot to the snapshots folder.
func saveSnapshot(camera string) (string, error) {
	b, err := snapshotJPEG(camera, 0)
	if err != nil {
		return "", err
	}
	dir := snapshotsDir()
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileName(camera, "jpg"))
	return path, os.WriteFile(path, b, 0o644)
}
