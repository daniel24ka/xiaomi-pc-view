package miss

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss/cs2"
)

const (
	codecH264 = 4
	codecH265 = 5
	codecPCM  = 1024
	codecPCMU = 1026
	codecPCMA = 1027
	codecOPUS = 1032
)

type Conn interface {
	Protocol() string
	Version() string
	ReadCommand() (cmd uint32, data []byte, err error)
	WriteCommand(cmd uint32, data []byte) error
	ReadPacket() (hdr, payload []byte, err error)
	WritePacket(hdr, payload []byte) error
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	Close() error
}

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// 1. Check if we can create shared key.
	query := u.Query()
	key, err := crypto.CalcSharedKey(query.Get("device_public"), query.Get("client_private"))
	if err != nil {
		return nil, err
	}

	model := query.Get("model")

	// 2. Check if this vendor supported.
	var conn Conn
	switch s := query.Get("vendor"); s {
	case "cs2":
		if query.Get("p2p") == "server" {
			var servers []string
			if s := query.Get("servers"); s != "" {
				servers = strings.Split(s, ",")
			}
			conn, err = cs2.DialServer(query.Get("uid"), servers)
		} else {
			conn, err = cs2.Dial(u.Host, query.Get("transport"))
		}
	case "tutk":
		conn, err = tutk.Dial(u.Host, query.Get("uid"), "Miss", "client")
	default:
		err = fmt.Errorf("miss: unsupported vendor %s", s)
	}

	if err != nil {
		return nil, err
	}

	err = login(conn, query.Get("client_public"), query.Get("sign"))
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	client := &Client{Conn: conn, key: key, model: model, uid: query.Get("uid")}
	client.startCommandLoop()
	return client, nil
}

// XIAOMI_MISS_DEBUG=1 prints command responses from the camera
var debugCommands = os.Getenv("XIAOMI_MISS_DEBUG") != ""

// startCommandLoop drains command responses (e.g. motor 0x113), otherwise the cs2 command
// channel overflows and the stream breaks with "pop buffer is full" (AlexxIT/go2rtc#2489).
func (c *Client) startCommandLoop() {
	go func() {
		for {
			cmd, data, err := c.Conn.ReadCommand()
			if err != nil {
				return
			}
			cmd = bits.ReverseBytes32(cmd) // cs2 ReadCommand reads the big endian code as little endian
			if cmd == cmdEncoded {
				if data, err = crypto.Decode(data, c.key); err == nil && len(data) >= 4 {
					cmd, data = binary.BigEndian.Uint32(data), data[4:]
				}
			}
			if debugCommands {
				fmt.Fprintf(os.Stderr, "miss: command 0x%x %q\n", cmd, data)
			}
			if f := c.onCommand.Load(); f != nil {
				(*f)(cmd, data)
			}
		}
	}()
}

type Client struct {
	Conn
	key   []byte
	model string
	uid   string // the camera's P2P id (the recordings cache key)

	reqMu     sync.Mutex
	onCommand atomic.Pointer[func(cmd uint32, data []byte)]
}

const (
	cmdAuthReq           = 0x100
	cmdAuthRes           = 0x101
	cmdVideoStart        = 0x102
	cmdVideoStop         = 0x103
	cmdAudioStart        = 0x104
	cmdAudioStop         = 0x105
	cmdSpeakerStartReq   = 0x106
	cmdSpeakerStartRes   = 0x107
	cmdSpeakerStop       = 0x108
	cmdStreamCtrlReq     = 0x109
	cmdStreamCtrlRes     = 0x10A
	cmdGetAudioFormatReq = 0x10B
	cmdGetAudioFormatRes = 0x10C
	cmdPlaybackReq       = 0x10D
	cmdPlaybackRes       = 0x10E
	cmdDevInfoReq        = 0x110
	cmdDevInfoRes        = 0x111
	cmdMotorReq          = 0x112
	cmdMotorRes          = 0x113
	cmdEncoded           = 0x1001
)

func login(conn Conn, clientPublic, sign string) error {
	s := fmt.Sprintf(`{"public_key":"%s","sign":"%s","uuid":"","support_encrypt":0}`, clientPublic, sign)
	if err := conn.WriteCommand(cmdAuthReq, []byte(s)); err != nil {
		return err
	}

	_, data, err := conn.ReadCommand()
	if err != nil {
		return err
	}

	if !bytes.Contains(data, []byte(`"result":"success"`)) {
		return fmt.Errorf("miss: auth: %s", data)
	}

	return nil
}

func (c *Client) Version() string {
	return fmt.Sprintf("%s (%s)", c.Conn.Version(), c.model)
}

func (c *Client) WriteCommand(data []byte) error {
	data, err := crypto.Encode(data, c.key)
	if err != nil {
		return err
	}
	return c.Conn.WriteCommand(cmdEncoded, data)
}

const (
	ModelDafang  = "isa.camera.df3"
	ModelLoockV2 = "loock.cateye.v02"
	ModelC200    = "chuangmi.camera.046c04"
	ModelC300    = "chuangmi.camera.72ac1"
)

func (c *Client) StartMedia(channel, quality, audio string) error {
	switch c.model {
	case ModelDafang:
		var q, a byte
		if quality == "sd" {
			q = 1 // 0 - hd, 1 - sd, default - hd
		}
		if audio != "0" {
			a = 1 // 0 - off, 1 - on, default - on
		}

		return errors.Join(
			c.WriteCommand(dafangVideoQuality(q)),
			c.WriteCommand(dafangVideoStart(1, a)),
		)
	}

	// 0 - auto, 1 - sd, 2 - hd, default - hd
	switch quality {
	case "", "hd":
		// Some models have broken codec settings in quality 3.
		// Some models have low quality in quality 2.
		// Different models require different default quality settings.
		switch c.model {
		case ModelC200, ModelC300:
			quality = "3"
		default:
			quality = "2"
		}
	case "sd":
		quality = "1"
	case "auto":
		quality = "0"
	}

	if audio == "" {
		audio = "1"
	}

	data := binary.BigEndian.AppendUint32(nil, cmdVideoStart)
	if channel == "" {
		data = fmt.Appendf(data, `{"videoquality":%s,"enableaudio":%s}`, quality, audio)
	} else {
		data = fmt.Appendf(data, `{"videoquality":-1,"videoquality2":%s,"enableaudio":%s}`, quality, audio)
	}
	return c.WriteCommand(data)
}

// Request sends a MISS command and waits for the camera response with code resp
// (e.g. PLAYBACK_REQ 0x10D -> PLAYBACK_RESP 0x10E).
func (c *Client) Request(cmd uint32, params string, resp uint32, timeout time.Duration) ([]byte, error) {
	return c.RequestMatch(cmd, params, resp, nil, timeout)
}

// RequestMatch is Request that accepts only responses for which match returns true
// (the camera may repeat old responses).
func (c *Client) RequestMatch(cmd uint32, params string, resp uint32, match func([]byte) bool, timeout time.Duration) ([]byte, error) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	ch := make(chan []byte, 1)
	f := func(code uint32, data []byte) {
		if code == resp && (match == nil || match(data)) {
			select {
			case ch <- data:
			default:
			}
		}
	}
	c.onCommand.Store(&f)
	defer c.onCommand.Store(nil)
	if err := c.Command(cmd, params); err != nil {
		return nil, err
	}
	select {
	case data := <-ch:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("miss: no response 0x%x", resp)
	}
}

// Command sends a MISS command with JSON params, like sendP2PCommandToDevice in Mi Home
// plugins (e.g. 530 - zoom {"value": zoom*10} on isa.camera.700sa).
func (c *Client) Command(cmd uint32, params string) error {
	data := binary.BigEndian.AppendUint32(nil, cmd)
	data = append(data, params...)
	return c.WriteCommand(data)
}

// Motor moves a pan/tilt camera one step, the same way the Mi Home app does:
// operation 1 - left, 2 - right, 3 - up, 4 - down.
func (c *Client) Motor(operation int) error {
	data := binary.BigEndian.AppendUint32(nil, cmdMotorReq)
	data = fmt.Appendf(data, `{"operation":%d}`, operation)
	return c.WriteCommand(data)
}

// Debugf, when set, gets debug messages (playback search); nil in normal use.
var Debugf func(format string, args ...any)

func debugf(format string, args ...any) {
	if Debugf != nil {
		Debugf(format, args...)
	}
}

const (
	cmdPlaybackResp = 0x10E
	playbackSearch  = 70 // seconds back to search for the file start (files are ~61 s)
)

// StartPlayback plays SD card recordings from unix time `at` (seconds) at speed
// (1, 2, 4, 8, 16), like the Mi Home plugin (MISS_CMD_PLAYBACK_REQ). The camera needs the
// exact start of a recorded file (files are ~60 s, one after another): playback starts at
// the beginning of the file with `at` (with offset > 0 the camera sends one frame and stops).
//
// The start is searched among the seconds before `at`: first the ones predicted from files
// found before (recordings), then the others; requests go out in batches (one request to a
// remote camera takes ~0.3 s, the answers come in any order).
func (c *Client) StartPlayback(at, speed, channel string) error {
	target, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return err
	}
	if speed == "" {
		speed = "1"
	}
	if channel == "" {
		channel = "0"
	}
	request := func(start int64) string {
		return fmt.Sprintf(
			`{"sessionid":%d,"starttime":%d,"endtime":0,"autoswitchtolive":0,"offset":0,"speed":%s,"avchannelmerge":1,"channel":%s}`,
			start, start, speed, channel)
	}
	predicted, candidates := recordings.candidates(c.uid, target)
	// the predicted starts one by one (usually the first one is it), then the others in
	// small batches (the camera answers one request after another anyway)
	for _, start := range predicted {
		found, file, err := c.playbackBatch([]int64{start}, request)
		if err != nil {
			return err
		}
		if found != 0 {
			recordings.add(c.uid, file)
			return nil
		}
	}
	for len(candidates) > 0 {
		n := min(len(candidates), playbackBatch)
		batch := candidates[:n]
		candidates = candidates[n:]
		found, file, err := c.playbackBatch(batch, request)
		if err != nil {
			return err
		}
		if found == 0 {
			continue
		}
		recordings.add(c.uid, file)
		if found != batch[n-1] {
			// later requests of the batch (other seconds) came after it: ask for it again,
			// so the camera surely plays this file
			if _, _, err = c.playbackBatch([]int64{found}, request); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("miss: no recording at this time")
}

const playbackBatch = 4

// playbackBatch sends playback requests for the starts and waits for all the answers;
// found - the start for which the camera has a file (0: none).
func (c *Client) playbackBatch(starts []int64, request func(int64) string) (found int64, file recording, err error) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	type answer struct {
		ID        int64  `json:"id"`
		Status    string `json:"status"`
		StartTime int64  `json:"starttime"`
		Duration  int64  `json:"duration"`
	}
	ch := make(chan answer, 4*len(starts))
	f := func(code uint32, data []byte) {
		if code != cmdPlaybackResp {
			return
		}
		var a answer
		if json.Unmarshal(bytes.TrimRight(data, "\x00 "), &a) == nil {
			select {
			case ch <- a:
			default:
			}
		}
	}
	c.onCommand.Store(&f)
	defer c.onCommand.Store(nil)
	pending := map[int64]bool{}
	for _, start := range starts {
		pending[start] = true
		if err = c.Command(cmdPlaybackReq, request(start)); err != nil {
			return 0, recording{}, err
		}
	}
	timeout := time.After(time.Duration(2+len(starts)/5) * time.Second)
	for len(pending) > 0 {
		select {
		case a := <-ch:
			if !pending[a.ID] {
				continue // an old answer
			}
			delete(pending, a.ID)
			debugf("playback %d: %s %d+%d", a.ID, a.Status, a.StartTime, a.Duration)
			if a.Status == "filefound" {
				found, file = a.ID, recording{start: a.StartTime, duration: a.Duration}
			}
		case <-timeout:
			if found != 0 {
				return found, file, nil
			}
			return 0, recording{}, fmt.Errorf("miss: no answer to %d playback requests", len(pending))
		}
	}
	return found, file, nil
}

// recordings - the recorded files found so far, per camera: the next searches try the
// predicted file starts first (usually one request instead of up to 70).
var recordings = &recordingCache{files: map[string][]recording{}}

type recording struct{ start, duration int64 }

type recordingCache struct {
	mu    sync.Mutex
	files map[string][]recording // sorted by start
}

func (r *recordingCache) add(uid string, f recording) {
	if f.start == 0 {
		return
	}
	if f.duration <= 0 {
		f.duration = 60
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.files[uid]
	for _, g := range list {
		if g.start == f.start {
			return
		}
	}
	list = append(list, f)
	sort.Slice(list, func(i, j int) bool { return list[i].start < list[j].start })
	if len(list) > 5000 { // ~3 days of 1-minute files
		list = list[len(list)-5000:]
	}
	r.files[uid] = list
}

// candidates returns the starts to try for target: predicted - the known file with target
// and the starts predicted from the nearest known files; rest - the other seconds of the
// search window.
func (r *recordingCache) candidates(uid string, target int64) (predicted, rest []int64) {
	r.mu.Lock()
	list := append([]recording(nil), r.files[uid]...)
	r.mu.Unlock()
	var out []int64
	seen := map[int64]bool{}
	addStart := func(s int64) {
		if s <= target && s > target-playbackSearch && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	var before, after *recording
	for i := range list {
		f := &list[i]
		if f.start <= target && target < f.start+f.duration {
			addStart(f.start) // a known file
		}
		if f.start <= target {
			before = f
		} else if after == nil {
			after = f
		}
	}
	if before != nil { // files follow the known one: start+duration, then ~60 s each
		s := before.start + before.duration
		for s+60 <= target {
			s += 60
		}
		for _, d := range []int64{0, 1, 2, -1, 3, 4, -2} {
			addStart(s + d)
		}
	}
	if after != nil { // files before the known one, ~60 s each
		s := after.start - 60
		for s > target {
			s -= 60
		}
		for _, d := range []int64{0, -1, -2, 1, -3, -4, 2} {
			addStart(s + d)
		}
	}
	predicted = out
	out = nil
	for s := target; s > target-playbackSearch; s-- {
		addStart(s)
	}
	return predicted, out
}

// Recording - a recorded file on the camera's SD card (unix seconds).
type Recording struct {
	Start    int64 `json:"start"`
	Duration int64 `json:"duration"`
}

// Recordings returns the recorded files found so far, per camera P2P id (to be saved).
func Recordings() map[string][]Recording {
	recordings.mu.Lock()
	defer recordings.mu.Unlock()
	out := map[string][]Recording{}
	for uid, list := range recordings.files {
		for _, f := range list {
			out[uid] = append(out[uid], Recording{f.start, f.duration})
		}
	}
	return out
}

// AddRecordings adds files found before (e.g. saved by the app last time).
func AddRecordings(all map[string][]Recording) {
	for uid, list := range all {
		for _, f := range list {
			recordings.add(uid, recording{f.Start, f.Duration})
		}
	}
}

func (c *Client) StopMedia() error {
	data := binary.BigEndian.AppendUint32(nil, cmdVideoStop)
	return c.WriteCommand(data)
}

func (c *Client) StartAudio() error {
	data := binary.BigEndian.AppendUint32(nil, cmdAudioStart)
	return c.WriteCommand(data)
}

func (c *Client) StartSpeaker() error {
	data := binary.BigEndian.AppendUint32(nil, cmdSpeakerStartReq)
	return c.WriteCommand(data)
}

// SpeakerCodec if the camera model has a non-standard two-way codec.
func (c *Client) SpeakerCodec() uint32 {
	switch c.model {
	case ModelDafang, "isa.camera.hlc6":
		return codecPCM
	case "chuangmi.camera.72ac1":
		return codecOPUS
	}
	return 0
}

const hdrSize = 32

func (c *Client) ReadPacket() (*Packet, error) {
	hdr, payload, err := c.Conn.ReadPacket()
	if err != nil {
		return nil, fmt.Errorf("miss: read media: %w", err)
	}

	if len(hdr) < hdrSize {
		return nil, fmt.Errorf("miss: packet header too small")
	}

	payload, err = crypto.Decode(payload, c.key)
	if err != nil {
		return nil, err
	}

	pkt := &Packet{
		CodecID:  binary.LittleEndian.Uint32(hdr[4:]),
		Sequence: binary.LittleEndian.Uint32(hdr[8:]),
		Flags:    binary.LittleEndian.Uint32(hdr[12:]),
		Payload:  payload,
	}

	switch c.model {
	case ModelDafang, ModelLoockV2:
		// Dafang has ts in sec
		// LoockV2 has ts in msec for video, but zero ts for audio
		pkt.Timestamp = uint64(time.Now().UnixMilli())
	default:
		pkt.Timestamp = binary.LittleEndian.Uint64(hdr[16:])
	}

	return pkt, nil
}

func (c *Client) WriteAudio(codecID uint32, payload []byte) error {
	payload, err := crypto.Encode(payload, c.key) // new payload will have new size!
	if err != nil {
		return err
	}

	n := uint32(len(payload))

	header := make([]byte, hdrSize)
	binary.LittleEndian.PutUint32(header, n)
	binary.LittleEndian.PutUint32(header[4:], codecID)
	binary.LittleEndian.PutUint64(header[16:], uint64(time.Now().UnixMilli())) // not really necessary
	return c.Conn.WritePacket(header, payload)
}

type Packet struct {
	//Length    uint32
	CodecID   uint32
	Sequence  uint32
	Flags     uint32
	Timestamp uint64 // msec
	//TimestampS uint32
	//Reserved uint32
	Payload []byte
}

func dafangRaw(cmd uint32, args ...byte) []byte {
	payload := tutk.ICAM(cmd, args...)

	data := make([]byte, 4+len(payload)*2)
	copy(data, "\x7f\xff\xff\xff")
	hex.Encode(data[4:], payload)
	return data
}

// DafangVideoQuality 0 - hd, 1 - sd
func dafangVideoQuality(quality uint8) []byte {
	return dafangRaw(0xff07d5, quality)
}

func dafangVideoStart(video, audio uint8) []byte {
	return dafangRaw(0xff07d8, video, audio)
}

//func dafangLeft() []byte {
//	return dafangRaw(0xff2404, 2, 0, 5)
//}
//
//func dafangRight() []byte {
//	return dafangRaw(0xff2404, 1, 0, 5)
//}
//
//func dafangUp() []byte {
//	return dafangRaw(0xff2404, 0, 2, 5)
//}
//
//func dafangDown() []byte {
//	return dafangRaw(0xff2404, 0, 1, 5)
//}
//
//func dafangStop() []byte {
//	return dafangRaw(0xff2404, 0, 0, 5)
//}
