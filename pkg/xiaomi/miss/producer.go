package miss

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	client  *Client
	pending []*Packet // read while probing, sent first
}

func Dial(rawURL string) (core.Producer, error) {
	t0 := time.Now()
	client, err := NewClient(rawURL)
	if err != nil {
		debugf("dial: %v (%v)", err, time.Since(t0))
		return nil, err
	}
	debugf("dial: connected in %v", time.Since(t0))

	u, _ := url.Parse(rawURL)
	query := u.Query()

	if start := query.Get("playback"); start != "" {
		// recordings from the SD card instead of live video
		err = client.StartPlayback(start, query.Get("speed"), query.Get("channel"))
	} else {
		err = client.StartMedia(query.Get("channel"), query.Get("subtype"), query.Get("audio"))
	}
	debugf("start: %v after dial (err %v)", time.Since(t0), err)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	tp := time.Now()
	medias, pending, err := probe(client, query.Get("audio") != "0")
	debugf("probe: %v (err %v)", time.Since(tp), err)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/miss",
			Protocol:   client.Protocol(),
			RemoteAddr: client.RemoteAddr().String(),
			UserAgent:  client.Version(),
			Medias:     medias,
			Transport:  client,
		},
		client:  client,
		pending: pending,
	}, nil
}

// probe reads packets until the codecs are known; pending - the packets from the first
// video key frame on, sent first when the producer starts (else the picture would wait
// for the next key frame, seconds later).
func probe(client *Client, audio bool) (medias []*core.Media, pending []*Packet, err error) {
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))

	var vcodec, acodec *core.Codec

	for {
		pkt, err := client.ReadPacket()
		if err != nil {
			if vcodec != nil {
				err = fmt.Errorf("no audio")
			} else if acodec != nil {
				err = fmt.Errorf("no video")
			}
			return nil, nil, fmt.Errorf("xiaomi: probe: %w", err)
		}

		if debugCommands {
			fmt.Fprintf(os.Stderr, "miss: probe packet codec 0x%x size %d\n", pkt.CodecID, len(pkt.Payload))
		}

		if vcodec != nil {
			pending = append(pending, pkt)
		}

		switch pkt.CodecID {
		case codecH264:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h264.NALUType(buf) == h264.NALUTypeSPS {
					vcodec = h264.AVCCToCodec(buf)
					pending = append(pending, pkt)
				}
			}
		case codecH265:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h265.NALUType(buf) == h265.NALUTypeVPS {
					vcodec = h265.AVCCToCodec(buf)
					pending = append(pending, pkt)
				}
			}
		case codecPCMA:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
			}
		case codecOPUS:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
			}
		}

		if vcodec != nil && (acodec != nil || !audio) {
			break
		}
	}

	_ = client.SetDeadline(time.Time{})

	medias = []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
	}

	if acodec != nil {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		})

		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{acodec.Clone()},
		})
	}

	return medias, pending, nil
}

const pendingDelay = 800 * time.Millisecond

const timestamp40ms = 48000 * 0.040

func (p *Producer) Start() error {
	var audioTS uint32

	if len(p.pending) > 0 {
		// the producer starts when a consumer is added (RTSP DESCRIBE), but an RTSP consumer
		// drops packets until its SETUP: give it a moment, so the key frame read while
		// probing reaches it (else the picture waits for the next key frame, ~3 s)
		time.Sleep(pendingDelay)
	}

	for {
		var pkt *Packet
		if len(p.pending) > 0 { // read while probing
			pkt, p.pending = p.pending[0], p.pending[1:]
		} else {
			_ = p.client.SetDeadline(time.Now().Add(10 * time.Second))
			var err error
			if pkt, err = p.client.ReadPacket(); err != nil {
				return err
			}
		}

		p.Recv += len(pkt.Payload)

		// TODO: rewrite this
		var name string
		var pkt2 *core.Packet

		switch pkt.CodecID {
		case codecH264, codecH265:
			pkt2 = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      TimeToRTP(pkt.Timestamp, 90000),
				},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}
			if pkt.CodecID == codecH264 {
				name = core.CodecH264
			} else {
				name = core.CodecH265
			}
		case codecPCMA:
			name = core.CodecPCMA
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			audioTS += uint32(len(pkt.Payload))
		case codecOPUS:
			name = core.CodecOpus
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			// known cameras sends packets with 40ms long
			audioTS += timestamp40ms
		}

		for _, recv := range p.Receivers {
			if recv.Codec.Name == name {
				recv.WriteRTP(pkt2)
				break
			}
		}
	}
}

func (p *Producer) Stop() error {
	_ = p.client.StopMedia()
	return p.Connection.Stop()
}

// Motor sends a pan/tilt step over this producer's session. The camera allows
// one session, so motor commands must share it rather than dial a second one.
func (p *Producer) Motor(operation int) error {
	return p.client.Motor(operation)
}

// Request sends a MISS command and waits for the response with code resp.
func (p *Producer) Request(cmd uint32, params string, resp uint32, timeout time.Duration) ([]byte, error) {
	return p.client.Request(cmd, params, resp, timeout)
}

// Command sends a MISS command over this producer's session.
// SDFiles returns the files on the SD card (over this session).
func (p *Producer) SDFiles(timeout time.Duration) ([]SDFile, error) {
	return p.client.SDFiles(timeout)
}

func (p *Producer) Command(cmd uint32, params string) error {
	return p.client.Command(cmd, params)
}

// TimeToRTP convert time in milliseconds to RTP time
func TimeToRTP(timeMS, clockRate uint64) uint32 {
	return uint32(timeMS * clockRate / 1000)
}
