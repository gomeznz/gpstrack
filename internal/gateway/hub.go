// Package gateway accepts tracker TCP connections, logs every byte in both
// directions, decodes H02 positions and can send commands back down the
// open session.
package gateway

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/gomeznz/gpstrack/internal/h02"
	"github.com/gomeznz/gpstrack/internal/store"
)

// IdleTimeout closes a session that has sent nothing for this long.
// CJ720 heartbeats and reports should arrive well inside it.
const IdleTimeout = 15 * time.Minute

// LogEntry is one line of the raw traffic log.
type LogEntry struct {
	Time     time.Time `json:"time"`
	Dir      string    `json:"dir"` // in, out, info
	Remote   string    `json:"remote,omitempty"`
	DeviceID string    `json:"deviceId,omitempty"`
	Text     string    `json:"text,omitempty"`
	Hex      string    `json:"hex,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// Device is the latest known state of one tracker.
type Device struct {
	ID          string        `json:"id"`
	Connected   bool          `json:"connected"`
	Remote      string        `json:"remote"`
	FirstSeen   time.Time     `json:"firstSeen"`
	LastSeen    time.Time     `json:"lastSeen"`
	LastType    string        `json:"lastType"`
	Messages    int           `json:"messages"`
	Position    *h02.Position `json:"position,omitempty"`
	LastCommand string        `json:"lastCommand,omitempty"`
	LastReply   string        `json:"lastReply,omitempty"` // last non-V1 message, often the command reply
}

type session struct {
	conn   net.Conn
	remote string
	wmu    sync.Mutex
}

// Hub owns all sessions and device state.
type Hub struct {
	raw       *store.DailyJSONL
	positions *store.DailyJSONL
	now       func() time.Time

	mu       sync.RWMutex
	sessions map[string]*session // by device ID
	devices  map[string]*Device
	recent   []LogEntry // ring of the newest entries
}

const recentMax = 500

func NewHub(raw, positions *store.DailyJSONL) *Hub {
	return &Hub{
		raw:       raw,
		positions: positions,
		now:       time.Now,
		sessions:  map[string]*session{},
		devices:   map[string]*Device{},
	}
}

// Serve accepts connections until the listener is closed.
func (h *Hub) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go h.handle(c)
	}
}

func (h *Hub) handle(c net.Conn) {
	remote := c.RemoteAddr().String()
	s := &session{conn: c, remote: remote}
	h.record(LogEntry{Dir: "info", Remote: remote, Note: "connected"})

	var deviceID string
	var fr h02.Framer
	buf := make([]byte, 4096)
	for {
		c.SetReadDeadline(time.Now().Add(IdleTimeout))
		n, err := c.Read(buf)
		if n > 0 {
			for _, f := range fr.Push(buf[:n]) {
				if id := h.frame(s, f); id != "" {
					deviceID = id
				}
			}
		}
		if err != nil {
			note := "disconnected"
			if !errors.Is(err, io.EOF) {
				note = "disconnected: " + err.Error()
			}
			h.record(LogEntry{Dir: "info", Remote: remote, DeviceID: deviceID, Note: note})
			break
		}
	}
	c.Close()
	if deviceID != "" {
		h.mu.Lock()
		if h.sessions[deviceID] == s {
			delete(h.sessions, deviceID)
			if d := h.devices[deviceID]; d != nil {
				d.Connected = false
			}
		}
		h.mu.Unlock()
	}
}

// frame handles one frame and returns the device ID it carried, if any.
func (h *Hub) frame(s *session, f h02.Frame) string {
	now := h.now().UTC()
	if f.Text == "" {
		h.record(LogEntry{Time: now, Dir: "in", Remote: s.remote, Hex: hex.EncodeToString(f.Binary), Note: describeBinary(f.Binary)})
		return ""
	}
	msg, err := h02.ParseText(f.Text)
	if err != nil {
		h.record(LogEntry{Time: now, Dir: "in", Remote: s.remote, Text: f.Text, Note: err.Error()})
		return ""
	}
	entry := LogEntry{Time: now, Dir: "in", Remote: s.remote, DeviceID: msg.DeviceID, Text: f.Text}

	var pos *h02.Position
	if msg.Type == "V1" {
		pos, err = msg.Position()
		if err != nil {
			entry.Note = err.Error()
		}
	}
	h.record(entry)

	h.mu.Lock()
	d := h.devices[msg.DeviceID]
	if d == nil {
		d = &Device{ID: msg.DeviceID, FirstSeen: now}
		h.devices[msg.DeviceID] = d
		log.Printf("new device %s from %s", msg.DeviceID, s.remote)
	}
	if old := h.sessions[msg.DeviceID]; old != nil && old != s {
		old.conn.Close() // the unit reconnected; drop the stale socket
	}
	h.sessions[msg.DeviceID] = s
	d.Connected, d.Remote, d.LastSeen, d.LastType = true, s.remote, now, msg.Type
	d.Messages++
	if pos != nil {
		d.Position = pos
	} else if msg.Type != "V1" {
		d.LastReply = f.Text
	}
	h.mu.Unlock()

	if pos != nil && h.positions != nil {
		if err := h.positions.Write(now, struct {
			Received time.Time `json:"received"`
			*h02.Position
		}{now, pos}); err != nil {
			log.Printf("positions log: %v", err)
		}
	}
	return msg.DeviceID
}

// describeBinary labels common non-tracker traffic so it isn't mistaken for
// a tracker in the log.
func describeBinary(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0x16 && b[1] == 0x03:
		return "HTTPS handshake from a web browser or port scanner, not a tracker"
	case bytes.HasPrefix(b, []byte("GET ")), bytes.HasPrefix(b, []byte("POST ")),
		bytes.HasPrefix(b, []byte("HEAD ")):
		return "plain HTTP request from a web browser or port scanner, not a tracker"
	case bytes.HasPrefix(b, []byte("SSH-")):
		return "SSH client, not a tracker"
	case len(b) > 0 && b[0] == '$':
		return "binary H02 frame (not decoded yet)"
	case bytes.HasPrefix(b, []byte("TRV")):
		return "TRV protocol frame (not decoded yet)"
	}
	return "unrecognised data, not H02 text"
}

var ErrNotConnected = errors.New("device is not connected")

// Send writes one raw line to a connected device and logs it.
func (h *Hub) Send(deviceID, line string) error {
	h.mu.RLock()
	s := h.sessions[deviceID]
	h.mu.RUnlock()
	if s == nil {
		return ErrNotConnected
	}
	s.wmu.Lock()
	s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := s.conn.Write([]byte(line))
	s.wmu.Unlock()

	e := LogEntry{Dir: "out", Remote: s.remote, DeviceID: deviceID, Text: line}
	if err != nil {
		e.Note = "write failed: " + err.Error()
	}
	h.record(e)
	if err == nil {
		h.mu.Lock()
		if d := h.devices[deviceID]; d != nil {
			d.LastCommand = line
		}
		h.mu.Unlock()
	}
	return err
}

// Devices returns a snapshot of all known devices, newest first.
func (h *Hub) Devices() []Device {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Device, 0, len(h.devices))
	for _, d := range h.devices {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

// Recent returns up to n of the newest log entries, newest first.
func (h *Hub) Recent(n int) []LogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n <= 0 || n > len(h.recent) {
		n = len(h.recent)
	}
	out := make([]LogEntry, n)
	for i := 0; i < n; i++ {
		out[i] = h.recent[len(h.recent)-1-i]
	}
	return out
}

func (h *Hub) record(e LogEntry) {
	if e.Time.IsZero() {
		e.Time = h.now().UTC()
	}
	if h.raw != nil {
		if err := h.raw.Write(e.Time, e); err != nil {
			log.Printf("raw log: %v", err)
		}
	}
	h.mu.Lock()
	h.recent = append(h.recent, e)
	if len(h.recent) > recentMax {
		h.recent = append([]LogEntry(nil), h.recent[len(h.recent)-recentMax:]...)
	}
	h.mu.Unlock()
}
