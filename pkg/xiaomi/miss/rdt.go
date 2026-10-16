package miss

import (
	"encoding/binary"
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

const rdtFileList = 6

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
	conn, ok := c.Conn.(*cs2.Conn)
	if !ok || c.rdt == nil {
		return nil, errors.New("miss: SD card file list only over CS2")
	}
	c.rdtMu.Lock()
	defer c.rdtMu.Unlock()
	for len(c.rdt) > 0 { // a late answer of a request that timed out
		<-c.rdt
	}

	req := make([]byte, 24) // the camera channel at offset 16 (0)
	binary.LittleEndian.PutUint32(req, rdtFileList)
	enc, err := crypto.Encode(req, c.key)
	if err != nil {
		return nil, err
	}
	if err = conn.WriteRDT(enc); err != nil {
		return nil, err
	}

	var data []byte
	size := -1
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for size < 0 || len(data) < size {
		select {
		case msg, ok := <-c.rdt:
			if !ok {
				return nil, errors.New("miss: SD card file list: session closed")
			}
			if size < 0 {
				if len(msg) < 12 || binary.LittleEndian.Uint32(msg) != rdtFileList {
					continue // an answer to something else
				}
				size = int(binary.LittleEndian.Uint32(msg[4:]))
				if size > 16<<20 {
					return nil, fmt.Errorf("miss: SD card file list: wrong size %d", size)
				}
				msg = msg[12:]
			}
			data = append(data, msg...)
		case <-deadline.C:
			return nil, errors.New("miss: SD card file list: no answer")
		}
	}

	files := parseSDFiles(data[:size])
	list := make([]recording, len(files))
	for i, f := range files {
		list[i] = recording{f.Start, f.Duration}
	}
	recordings.merge(c.uid, list)
	return files, nil
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
