package miss

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss/cs2"
)

// RDT - the reliable data channel of the camera (CS2 channel 1), used by the Mi Home plugin
// for the SD card file list (sendRDTCommandToDevice). Every message is size + data
// encrypted with the session key; an answer is [cmd LE][size LE][status LE][data], split
// into messages of ~2 KB.

const (
	rdtFileList   = 6
	rdtEventTypes = 11
)

// SDFile - a recorded file on the SD card.
type SDFile struct {
	Start    int64 // unix seconds
	Duration int64 // seconds
	Motion   bool
	Saved    bool
}

// startRDTLoop drains the RDT channel (an unread channel would break the session) and hands
// the decrypted messages to the waiting request.
func (c *Client) startRDTLoop() {
	conn, ok := c.Conn.(*cs2.Conn)
	if !ok {
		return
	}
	c.rdt = make(chan []byte, 64)
	go func() {
		defer close(c.rdt)
		for {
			msg, err := conn.ReadRDT()
			if err != nil {
				return
			}
			if msg, err = crypto.Decode(msg, c.key); err != nil {
				continue
			}
			select {
			case c.rdt <- msg:
			default: // nobody waits: drop
			}
		}
	}()
}

// SDFiles returns the files on the camera's SD card (oldest first) and adds them to the
// recordings cache. It uses the open session (live video or playback).
func (c *Client) SDFiles(timeout time.Duration) ([]SDFile, error) {
	req := make([]byte, 24) // the camera channel at offset 16 (0)
	binary.LittleEndian.PutUint32(req, rdtFileList)
	var data []byte
	err := c.rdtRequest(rdtFileList, req, timeout, func(answer []byte) bool {
		data = answer
		return true
	})
	if err != nil {
		return nil, err
	}

	files := parseSDFiles(data)
	list := make([]recording, len(files))
	for i, f := range files {
		list[i] = recording{f.Start, f.Duration}
	}
	recordings.merge(c.uid, list)
	return files, nil
}

// SDEvents returns the event types of the SD card recordings with motion (command 11, like
// the Mi Home plugin of the Hualai cameras); day "20261016" or "" for all days.
func (c *Client) SDEvents(day string, timeout time.Duration) ([]SDEvent, error) {
	param, _ := json.Marshal(map[string]string{"mac": "F1F2F3F4F5F6", "time": day, "channel": "0"})
	n := (len(param) + 3) &^ 3
	req := make([]byte, 8+n)
	binary.LittleEndian.PutUint32(req, rdtEventTypes)
	binary.LittleEndian.PutUint32(req[4:], uint32(n))
	copy(req[8:], param)

	var out []SDEvent
	err := c.rdtRequest(rdtEventTypes, req, timeout, func(answer []byte) bool {
		debugf("[miss] RDT events page: %d bytes, head % x", len(answer), answer[:min(len(answer), 76)])
		events, last := parseSDEvents(answer)
		out = append(out, events...)
		return last
	})
	if err != nil {
		return nil, err
	}
	return mergeSDEvents(out), nil
}

// rdtRequest sends an RDT command and hands every complete answer to page until it
// returns true (the answers of some commands come in pages).
func (c *Client) rdtRequest(cmd uint32, req []byte, timeout time.Duration, page func([]byte) bool) error {
	conn, ok := c.Conn.(*cs2.Conn)
	if !ok || c.rdt == nil {
		return errors.New("miss: RDT only over CS2")
	}
	c.rdtMu.Lock()
	defer c.rdtMu.Unlock()
	for len(c.rdt) > 0 { // a late answer of a request that timed out
		<-c.rdt
	}

	enc, err := crypto.Encode(req, c.key)
	if err != nil {
		return err
	}
	if err = conn.WriteRDT(enc); err != nil {
		return err
	}

	var data []byte
	size := -1
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-c.rdt:
			if !ok {
				return fmt.Errorf("miss: RDT %d: session closed", cmd)
			}
			if size < 0 {
				if len(msg) < 12 || binary.LittleEndian.Uint32(msg) != cmd {
					continue // an answer to something else
				}
				size = int(binary.LittleEndian.Uint32(msg[4:]))
				if size > 16<<20 {
					return fmt.Errorf("miss: RDT %d: wrong size %d", cmd, size)
				}
				data, msg = data[:0], msg[12:]
			}
			data = append(data, msg...)
			if len(data) < size {
				continue
			}
			if page(data[:size]) {
				return nil
			}
			size = -1 // the next page
		case <-deadline.C:
			return fmt.Errorf("miss: RDT %d: no answer", cmd)
		}
	}
}

// parseSDFiles - 8 bytes per file: start (uint32 LE), duration (0: 60 s), motion, saved,
// being deleted.
func parseSDFiles(b []byte) []SDFile {
	var out []SDFile
	for ; len(b) >= 8; b = b[8:] {
		start := int64(binary.LittleEndian.Uint32(b))
		if start == 0 || b[7] != 0 {
			continue
		}
		d := int64(b[4])
		if d == 0 {
			d = 60
		}
		out = append(out, SDFile{Start: start, Duration: d, Motion: b[5] == 1, Saved: b[6] != 0})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// SDEvent - the event type of an SD card recording with motion.
type SDEvent struct {
	Start int64 // unix seconds
	Type  int   // SDEvent* (the camera's own codes)
}

// Event types of the SD card recordings (codes of the Hualai cameras; the plugin shows the
// highest code of a file). CW400 (isa.camera.hlc8) uses 12 and 14 for the car events.
const (
	SDEventObjectMotion      = 0
	SDEventPeopleMotion      = 1
	SDEventCat               = 2
	SDEventDog               = 3
	SDEventFace              = 4
	SDEventKnownFace         = 5
	SDEventBabyCry           = 7
	SDEventAI                = 8
	SDEventPersonApproaching = 17
	SDEventCarDetected       = 19
	SDEventFenceIn           = 20
	SDEventFenceOut          = 21
)

// parseSDEvents - a page: [?][total pages][page index][32 bytes] and 32 bytes per event
// (start uint64 LE, type uint32 LE, the rest unknown).
func parseSDEvents(b []byte) (events []SDEvent, last bool) {
	if len(b) < 12 {
		return nil, true
	}
	total := binary.LittleEndian.Uint32(b[4:])
	index := binary.LittleEndian.Uint32(b[8:])
	for p := 44; p+32 <= len(b); p += 32 {
		start := int64(binary.LittleEndian.Uint64(b[p:]))
		if start == 0 {
			continue
		}
		events = append(events, SDEvent{Start: start, Type: int(binary.LittleEndian.Uint32(b[p+8:]))})
	}
	return events, index >= total
}

// mergeSDEvents sorts the events and keeps one per start (the highest code, like Mi Home).
func mergeSDEvents(in []SDEvent) []SDEvent {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Start != in[j].Start {
			return in[i].Start < in[j].Start
		}
		return in[i].Type > in[j].Type
	})
	var out []SDEvent
	for _, e := range in {
		if len(out) == 0 || out[len(out)-1].Start != e.Start {
			out = append(out, e)
		}
	}
	return out
}
