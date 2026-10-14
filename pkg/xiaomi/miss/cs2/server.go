package cs2

import (
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// DefaultServers - CS2 servers used by Xiaomi cameras in the EU region (from PCAP of Mi Home app).
var DefaultServers = []string{"34.34.47.37", "34.90.51.72", "34.147.123.47"}

const (
	msgP2PReq    = 0x20
	msgP2PReqAck = 0x21
	msgPunchTo   = 0x40
	serverPort   = 32100
)

// DialServer connects to the camera with help of CS2 servers (UDP hole punching),
// like the Mi Home app does when the camera is not in the same LAN.
func DialServer(uid string, servers []string) (*Conn, error) {
	conn, err := serverHandshake(uid, servers)
	if err != nil {
		return nil, err
	}
	return newConn(conn, false), nil
}

// parseUID - "XMSYDEB-814722-SNJUY" => 8 bytes prefix + 4 bytes serial + 8 bytes check
func parseUID(uid string) ([]byte, error) {
	parts := strings.Split(uid, "-")
	if len(parts) != 3 {
		return nil, errors.New("cs2: wrong uid: " + uid)
	}
	serial, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return nil, errors.New("cs2: wrong uid: " + uid)
	}
	b := make([]byte, 20)
	copy(b[:8], parts[0])
	binary.BigEndian.PutUint32(b[8:], uint32(serial))
	copy(b[12:], parts[2])
	return b, nil
}

// CS2 sockaddr: family (BE), port (LE), IPv4 (reversed), 8 bytes zero
func marshalAddr(addr *net.UDPAddr) []byte {
	b := make([]byte, 16)
	b[1] = 2
	binary.LittleEndian.PutUint16(b[2:], uint16(addr.Port))
	ip := addr.IP.To4()
	b[4], b[5], b[6], b[7] = ip[3], ip[2], ip[1], ip[0]
	return b
}

func unmarshalAddr(b []byte) *net.UDPAddr {
	return &net.UDPAddr{
		IP:   net.IPv4(b[7], b[6], b[5], b[4]),
		Port: int(binary.LittleEndian.Uint16(b[2:])),
	}
}

func packet(msg byte, payload []byte) []byte {
	b := make([]byte, 4, 4+len(payload))
	b[0] = magic
	b[1] = msg
	binary.BigEndian.PutUint16(b[2:], uint16(len(payload)))
	return append(b, payload...)
}

func serverHandshake(uid string, servers []string) (net.Conn, error) {
	did, err := parseUID(uid)
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		servers = DefaultServers
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	var serverAddrs []*net.UDPAddr
	for _, s := range servers {
		if addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(s, strconv.Itoa(serverPort))); err == nil {
			serverAddrs = append(serverAddrs, addr)
		}
	}
	if len(serverAddrs) == 0 {
		_ = conn.Close()
		return nil, errors.New("cs2: no servers")
	}

	// local address, so the camera can also try to reach us directly in the same LAN
	local := &net.UDPAddr{IP: net.IPv4zero, Port: conn.LocalAddr().(*net.UDPAddr).Port}
	if probe, err := net.DialUDP("udp4", nil, serverAddrs[0]); err == nil {
		local.IP = probe.LocalAddr().(*net.UDPAddr).IP
		_ = probe.Close()
	}

	req := packet(msgP2PReq, append(did, marshalAddr(local)...))
	punch := packet(msgPunchPkt, did)

	isServer := func(addr *net.UDPAddr) bool {
		for _, s := range serverAddrs {
			if s.IP.Equal(addr.IP) && s.Port == addr.Port {
				return true
			}
		}
		return false
	}

	var candidates []*net.UDPAddr

	deadline := time.Now().Add(10 * time.Second)
	nextSend := time.Now()
	buf := make([]byte, 1500)

	for time.Now().Before(deadline) {
		if now := time.Now(); now.After(nextSend) {
			// repeat request to servers until camera answers, punch all known camera addresses
			for _, s := range serverAddrs {
				_, _ = conn.WriteToUDP(req, s)
			}
			for _, c := range candidates {
				_, _ = conn.WriteToUDP(punch, c)
			}
			nextSend = now.Add(500 * time.Millisecond)
		}

		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			_ = conn.Close()
			return nil, err
		}
		if n < 4 || buf[0] != magic {
			continue
		}

		switch msg := buf[1]; {
		case isServer(addr):
			if msg == msgPunchTo && n >= 20 {
				c := unmarshalAddr(buf[4:20])
				candidates = append(candidates, c)
				_, _ = conn.WriteToUDP(punch, c)
			}
		case msg == msgPunchPkt:
			// camera punched us, answer with the same packet
			_, _ = conn.WriteToUDP(buf[:n], addr)
		case msg == msgP2PRdyUDP:
			_ = conn.SetReadDeadline(time.Time{})
			return &udpConn{UDPConn: conn, addr: addr}, nil
		}
	}

	_ = conn.Close()
	return nil, errors.New("cs2: camera didn't answer via servers")
}
