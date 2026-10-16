package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
)

// Local recording to MP4: H.265 video is copied as it comes from the camera (no
// re-encoding, almost no CPU), audio is encoded to AAC. Fragmented MP4, so the file is
// playable even if the app is killed. Recording uses the same camera session as the live
// view and works with the window hidden. Used by the UI, the HTTP API and MCP.

type recording struct {
	camera   string
	path     string
	started  time.Time
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	done     chan struct{}
	limit    *time.Timer
	stderr   *tailBuffer
	err      error // set by stopRecording when the file was not saved
	stopping atomic.Bool
}

// tailBuffer keeps the last 2 KB of ffmpeg's errors (for the message when nothing was saved)
type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.b = append(t.b, p...)
	if len(t.b) > 2048 {
		t.b = t.b[len(t.b)-2048:]
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.b))
}

// writing - ffmpeg has opened all inputs and writes the file (the grid opens the cameras one
// after another, that takes seconds)
func (r *recording) writing() bool {
	st, err := os.Stat(r.path)
	return err == nil && st.Size() > 0
}

var (
	recMu sync.Mutex
	recs  = map[string]*recording{}

	// onRecordingChanged is called (from any goroutine) when a recording starts or stops
	onRecordingChanged func(camera string)
	onRecordingFailed  func(msg string) // ffmpeg ended by itself without a file

	appPrefs fyne.Preferences // set in main, used for save folders
)

const (
	prefRecordingsDir = "dir.recordings"
	prefSnapshotsDir  = "dir.snapshots"
)

// xdgDir returns a user folder (VIDEOS, PICTURES) in any language, or $HOME. Windows:
// %USERPROFILE%\Videos, \Pictures (the names on disk are English in every language).
func xdgDir(kind string) string {
	if runtime.GOOS == "windows" {
		home, _ := os.UserHomeDir()
		name := map[string]string{"VIDEOS": "Videos", "PICTURES": "Pictures"}[kind]
		if d := filepath.Join(home, name); name != "" && fileExists(d) {
			return d
		}
		return home
	}
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
// recordVideoArgs - the video is saved as it comes (H.265 of Xiaomi cameras, H.264 / H.265
// of IP cameras); other codecs (MJPEG) are encoded to H.264.
func recordVideoArgs(camera string) []string {
	codec := "hevc"
	if c := ipCam(camera); c != nil {
		codec = c.Codec
		if c.Quality == 1 && c.Sub != "" {
			codec = c.SubCodec
		}
	}
	switch codec {
	case "hevc":
		return []string{"-c:v", "copy", "-tag:v", "hvc1"} // hvc1 tag for players (QuickTime, browsers)
	case "h264", "": // "": not known, IP cameras send H.264 mostly
		return []string{"-c:v", "copy"}
	}
	return []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "26", "-pix_fmt", "yuv420p"}
}

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
	cmd := toolCommand(nil, "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-analyzeduration", "500000", "-probesize", "500000",
		"-rtsp_transport", "tcp", "-i", rtspURL(camera),
		"-map", "0:v:0", "-map", "0:a:0?")
	cmd.Args = append(cmd.Args, recordVideoArgs(camera)...)
	cmd.Args = append(cmd.Args, "-c:a", "aac", "-b:a", "64k",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-y", path)
	return launchRecording(camera, path, cmd, limit)
}

// launchRecording starts ffmpeg and registers the recording under key (recMu held).
func launchRecording(key, path string, cmd *exec.Cmd, limit time.Duration) (*recording, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	r := &recording{camera: key, path: path, started: time.Now(), cmd: cmd, stdin: stdin,
		done: make(chan struct{}), stderr: &tailBuffer{}}
	cmd.Stderr = r.stderr
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	recs[key] = r
	go func() {
		_ = cmd.Wait()
		recMu.Lock()
		if recs[key] == r {
			delete(recs, key)
		}
		recMu.Unlock()
		close(r.done)
		if r.limit != nil {
			r.limit.Stop()
		}
		if !r.stopping.Load() && onRecordingFailed != nil {
			if msg := r.failure(); msg != "" {
				_ = os.Remove(r.path)
				onRecordingFailed(msg)
			}
		}
		if onRecordingChanged != nil {
			onRecordingChanged(key)
		}
	}()
	if limit > 0 {
		r.limit = time.AfterFunc(limit, func() { _, _ = stopRecording(key) })
	}
	if onRecordingChanged != nil {
		onRecordingChanged(key)
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
	r.stopping.Store(true)
	if !r.writing() {
		// still opening the cameras: ffmpeg reads "q" only when it writes
		_ = r.cmd.Process.Kill()
		<-r.done
		_ = os.Remove(r.path)
		r.err = errors.New(T("nic nie nagrano – kamery jeszcze się łączyły"))
		return r, r.err
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

// failure - why a recording saved nothing ("" when it saved)
func (r *recording) failure() string {
	if r.writing() {
		return ""
	}
	msg := T("nagranie nie zostało zapisane")
	if e := r.stderr.String(); e != "" {
		lines := strings.Split(e, "\n")
		msg += ": " + lines[len(lines)-1]
	}
	return msg
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

// --- recording modes: the selected camera, each camera to its own file, or all cameras
// in one grid video

const (
	prefRecordMode = "record.mode"
	recordSelected = ""     // the selected camera
	recordEach     = "each" // every camera, separate files
	recordGrid     = "grid" // all cameras in one file (grid, re-encoded)

	gridKey = "\x00grid" // recordings map key of the grid recording
)

const prefRecordSkip = "record.skip" // cameras NOT recorded in each/grid mode (new cameras are recorded)

// recordCameras - cameras recorded in the each/grid modes (all minus the unchecked ones)
func recordCameras() []string {
	skip := map[string]bool{}
	if appPrefs != nil {
		for _, n := range appPrefs.StringList(prefRecordSkip) {
			skip[n] = true
		}
	}
	var out []string
	for _, n := range cameraNames() {
		if !skip[n] {
			out = append(out, n)
		}
	}
	return out
}

func setRecordCamera(name string, on bool) {
	if appPrefs == nil {
		return
	}
	var skip []string
	for _, n := range appPrefs.StringList(prefRecordSkip) {
		if n != name {
			skip = append(skip, n)
		}
	}
	if !on {
		skip = append(skip, name)
	}
	appPrefs.SetStringList(prefRecordSkip, skip)
}

func recordMode() string {
	if appPrefs != nil {
		return appPrefs.String(prefRecordMode)
	}
	return recordSelected
}

// startRecordings starts recording in the given mode; selected is the selected camera,
// cameras - cameras for the each/grid modes (nil - from the settings).
func startRecordings(mode, selected string, cameras []string, limit time.Duration) ([]*recording, error) {
	if cameras == nil {
		cameras = recordCameras()
	}
	if (mode == recordEach || mode == recordGrid) && len(cameras) == 0 {
		return nil, errors.New("no cameras selected for recording (Settings → General)")
	}
	switch mode {
	case recordEach:
		var out []*recording
		var errs []error
		for _, name := range cameras {
			r, err := startRecording(name, limit)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
				continue
			}
			out = append(out, r)
		}
		return out, errors.Join(errs...)
	case recordGrid:
		r, err := startGridRecording(cameras, selected, limit)
		if err != nil {
			return nil, err
		}
		return []*recording{r}, nil
	default:
		r, err := startRecording(selected, limit)
		if err != nil {
			return nil, err
		}
		return []*recording{r}, nil
	}
}

// startGridRecording records all cameras in one H.265 video, laid out like the grid view
// (cells 960x540); the sound is from the selected camera.
func startGridRecording(cameras []string, selected string, limit time.Duration) (*recording, error) {
	if len(cameras) == 0 {
		return nil, errors.New("no cameras")
	}
	recMu.Lock()
	defer recMu.Unlock()
	if r := recs[gridKey]; r != nil {
		return r, errors.New("already recording: " + r.path)
	}
	dir := recordingsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fileName("grid", "mp4"))

	const cw, ch = 960, 540
	cols := int(math.Ceil(math.Sqrt(float64(len(cameras)))))
	args := []string{"-hide_banner", "-loglevel", "error"}
	audio := 0
	for i, name := range cameras {
		// short probe: ffmpeg opens the inputs one after another (9 cameras: 22 s before
		// the file was created, 7 s now); wall clock timestamps keep the cells in sync
		args = append(args, "-analyzeduration", "500000", "-probesize", "500000",
			"-use_wallclock_as_timestamps", "1", "-rtsp_transport", "tcp", "-i", rtspURL(name))
		if name == selected {
			audio = i
		}
	}
	var filter strings.Builder
	var layout []string
	for i := range cameras {
		fmt.Fprintf(&filter, "[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:-1:-1,setsar=1[v%d];", i, cw, ch, cw, ch, i)
		layout = append(layout, fmt.Sprintf("%d_%d", (i%cols)*cw, (i/cols)*ch))
	}
	if len(cameras) == 1 {
		filter.WriteString("[v0]null")
	} else {
		for i := range cameras {
			fmt.Fprintf(&filter, "[v%d]", i)
		}
		fmt.Fprintf(&filter, "xstack=inputs=%d:layout=%s:fill=black", len(cameras), strings.Join(layout, "|"))
	}
	filter.WriteString(",fps=25,setpts=PTS-STARTPTS[v]") // constant rate, starting at 0
	args = append(args,
		"-filter_complex", filter.String(),
		"-map", "[v]", "-map", fmt.Sprintf("%d:a:0?", audio),
		"-c:v", "libx265", "-preset", "superfast", "-crf", "28", "-pix_fmt", "yuv420p", "-tag:v", "hvc1",
		"-c:a", "aac", "-b:a", "64k",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-y", path)

	cmd := toolCommand(nil, "ffmpeg", args...)
	return launchRecording(gridKey, path, cmd, limit)
}

// activeRecordings - all running recordings, oldest first
func activeRecordings() []*recording {
	recMu.Lock()
	defer recMu.Unlock()
	var out []*recording
	for _, r := range recs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].started.Before(out[j].started) })
	return out
}

// stopAll stops all running recordings and returns them.
func stopAll() []*recording {
	var out []*recording
	for _, r := range activeRecordings() {
		if rr, _ := stopRecording(r.camera); rr != nil {
			out = append(out, rr)
		}
	}
	return out
}
