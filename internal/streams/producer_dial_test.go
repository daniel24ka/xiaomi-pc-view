package streams

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// slowProducer stands for a camera that takes a while to answer a dial.
type slowProducer struct {
	core.Connection
	stopped atomic.Bool
}

func (s *slowProducer) GetMedias() []*core.Media { return nil }
func (s *slowProducer) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, errors.New("none")
}
func (s *slowProducer) Start() error { return nil }
func (s *slowProducer) Stop() error  { s.stopped.Store(true); return nil }

// A source change while a dial is running returns at once (the UI called it) and the late
// connection is thrown away, not used.
func TestSetSourceDoesNotWaitForDial(t *testing.T) {
	var made []*slowProducer
	HandleFunc("slowtest", func(url string) (core.Producer, error) {
		time.Sleep(2 * time.Second)
		p := &slowProducer{}
		made = append(made, p)
		return p, nil
	})
	p := NewProducer("slowtest://a")

	dialed := make(chan error, 1)
	go func() { dialed <- p.Dial() }()
	time.Sleep(200 * time.Millisecond) // the dial is running

	t0 := time.Now()
	p.SetSource("slowtest://b")
	require.Less(t, time.Since(t0), 500*time.Millisecond, "SetSource waited for the dial")

	err := <-dialed
	require.Error(t, err) // the source changed while dialing
	require.Len(t, made, 1)
	require.True(t, made[0].stopped.Load(), "the late connection was not closed")
	require.Nil(t, p.conn)
}

// Two dials at once make one connection (a Xiaomi camera takes one session).
func TestDialsAreSerialized(t *testing.T) {
	var n atomic.Int32
	HandleFunc("slowtest2", func(url string) (core.Producer, error) {
		n.Add(1)
		time.Sleep(300 * time.Millisecond)
		return &slowProducer{}, nil
	})
	p := NewProducer("slowtest2://a")
	done := make(chan error, 2)
	go func() { done <- p.Dial() }()
	go func() { done <- p.Dial() }()
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	require.Equal(t, int32(1), n.Load())
}
