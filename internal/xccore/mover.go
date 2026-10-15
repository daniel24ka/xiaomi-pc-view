package xccore

import (
	"sync"
	"time"
)

// Mover drives the pan/tilt motor like the Mi Home plugin does: a direction command
// (operation 1-4) starts a continuous movement and is repeated every 200 ms while the button
// or key is held; on release the stop command (operation -1001) is sent - right away after
// a long press, 200 ms later after a short click (so a click moves the camera a bit).
type Mover struct {
	mu      sync.Mutex
	held    int       // direction being held, 0 - none
	gen     int       // press counter: a stop belongs to the press it was scheduled for
	since   time.Time // when the current press started
	onError func(error)
	camera  func() string
}

const (
	motorStop     = -1001 // CMD_OFF in the Mi Home plugin
	motorRepeat   = 200 * time.Millisecond
	motorLongHold = 800 * time.Millisecond
)

func NewMover(camera func() string, onError func(error)) *Mover {
	return &Mover{camera: camera, onError: onError}
}

func (m *Mover) send(op int) {
	if err := Motor(m.camera(), op); err != nil && m.onError != nil {
		m.onError(err)
	}
}

// Press starts moving in the direction (button down / key down / drag).
func (m *Mover) Press(op int) {
	m.mu.Lock()
	if m.held == op {
		m.mu.Unlock()
		return
	}
	m.gen++
	gen := m.gen
	m.held, m.since = op, time.Now()
	m.mu.Unlock()

	go func() {
		for {
			m.send(op)
			time.Sleep(motorRepeat)
			m.mu.Lock()
			still := m.gen == gen && m.held == op
			m.mu.Unlock()
			if !still {
				return
			}
		}
	}()
}

// Release stops the movement (button up / key up / end of drag).
func (m *Mover) Release() {
	m.mu.Lock()
	if m.held == 0 {
		m.mu.Unlock()
		return
	}
	gen := m.gen
	delay := motorRepeat // short click: let it move a bit
	if time.Since(m.since) > motorLongHold {
		delay = 0
	}
	m.held = 0
	m.mu.Unlock()

	time.AfterFunc(delay, func() {
		m.mu.Lock()
		current := m.gen == gen && m.held == 0 // not pressed again meanwhile
		m.mu.Unlock()
		if current {
			m.send(motorStop)
		}
	})
}

// Pulse moves in the direction for d, then stops (API, MCP).
func (m *Mover) Pulse(op int, d time.Duration) {
	m.Press(op)
	time.Sleep(d)
	m.mu.Lock()
	m.since = time.Now().Add(-time.Hour) // stop right away
	m.mu.Unlock()
	m.Release()
}
