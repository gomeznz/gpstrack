// Package h02 decodes and encodes the H02 text protocol used by the CJ720
// (also sold as LK720) and many other low-cost vehicle trackers.
//
// A text frame looks like:
//
//	*HQ,4106054111,V1,121633,A,4624.5260,S,16821.4340,E,000.00,000,221026,FFFFFBFF,530,01,0,0#
//
// Fields after the message type are type-specific. Only V1 (position) is
// decoded into a Position; everything else is kept as raw fields so it can be
// studied from the logs during Phase 0.
package h02

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Message is one decoded text frame.
type Message struct {
	Maker    string   // usually "HQ"
	DeviceID string   // IMEI or the 10-digit ID on the unit's label
	Type     string   // V1, V4, NBR, LINK, ...
	Fields   []string // everything after Type
	Raw      string
}

// Position is a decoded V1 report.
type Position struct {
	DeviceID  string    `json:"deviceId"`
	Time      time.Time `json:"time"`  // device fix time, UTC
	Valid     bool      `json:"valid"` // GPS fix flag "A"
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedKmh  float64   `json:"speedKmh"`
	Course    float64   `json:"course"`
	StatusHex string    `json:"statusHex"`
	// ActiveBits lists status bits that are 0. H02 uses negative logic, so a
	// 0 bit usually means "on". Which bit means what (ACC, relay, power cut)
	// varies by firmware and is to be confirmed from Phase 0 captures.
	ActiveBits []int  `json:"activeBits"`
	Cell       string `json:"cell,omitempty"` // MCC,MNC,LAC,CID when present
}

var (
	ErrNotFrame    = errors.New("h02: not a text frame")
	ErrNotPosition = errors.New("h02: not a position message")
)

// ParseText decodes one complete text frame, including the leading '*' and
// trailing '#'.
func ParseText(frame string) (*Message, error) {
	s := strings.TrimSpace(frame)
	if len(s) < 3 || s[0] != '*' || s[len(s)-1] != '#' {
		return nil, ErrNotFrame
	}
	parts := strings.Split(s[1:len(s)-1], ",")
	if len(parts) < 3 || parts[1] == "" {
		return nil, fmt.Errorf("%w: too few fields", ErrNotFrame)
	}
	return &Message{
		Maker:    parts[0],
		DeviceID: parts[1],
		Type:     parts[2],
		Fields:   parts[3:],
		Raw:      s,
	}, nil
}

// Position decodes a V1 message.
// V1 fields: time, validity, lat, N/S, lon, E/W, speed(knots), course, date, status, [cell...]
func (m *Message) Position() (*Position, error) {
	if m.Type != "V1" {
		return nil, ErrNotPosition
	}
	f := m.Fields
	if len(f) < 10 {
		return nil, fmt.Errorf("h02: V1 has %d fields, want at least 10", len(f))
	}
	ts, err := parseTime(f[8], f[0])
	if err != nil {
		return nil, err
	}
	lat, err := parseCoord(f[2], f[3], 2)
	if err != nil {
		return nil, fmt.Errorf("h02: latitude: %w", err)
	}
	lon, err := parseCoord(f[4], f[5], 3)
	if err != nil {
		return nil, fmt.Errorf("h02: longitude: %w", err)
	}
	p := &Position{
		DeviceID: m.DeviceID,
		Time:     ts,
		Valid:    f[1] == "A",
		Lat:      lat,
		Lon:      lon,
	}
	if f[6] != "" {
		knots, err := strconv.ParseFloat(f[6], 64)
		if err != nil {
			return nil, fmt.Errorf("h02: speed: %w", err)
		}
		p.SpeedKmh = math.Round(knots*1.852*10) / 10
	}
	if f[7] != "" {
		p.Course, _ = strconv.ParseFloat(f[7], 64)
	}
	p.StatusHex = strings.ToUpper(f[9])
	if st, err := strconv.ParseUint(f[9], 16, 32); err == nil {
		p.ActiveBits = ActiveBits(uint32(st))
	}
	if len(f) >= 14 {
		p.Cell = strings.Join(f[10:14], ",")
	}
	return p, nil
}

// ActiveBits returns the indexes of bits that are 0 (active in H02's
// negative logic).
func ActiveBits(status uint32) []int {
	out := []int{}
	for i := 0; i < 32; i++ {
		if status&(1<<uint(i)) == 0 {
			out = append(out, i)
		}
	}
	return out
}

func parseTime(date, hms string) (time.Time, error) {
	if i := strings.IndexByte(hms, '.'); i >= 0 {
		hms = hms[:i]
	}
	if len(date) != 6 || len(hms) != 6 {
		return time.Time{}, fmt.Errorf("h02: bad date/time %q %q", date, hms)
	}
	t, err := time.Parse("020106150405", date+hms)
	if err != nil {
		return time.Time{}, fmt.Errorf("h02: bad date/time: %w", err)
	}
	return t.UTC(), nil
}

// parseCoord converts NMEA-style (d)ddmm.mmmm plus hemisphere to decimal degrees.
func parseCoord(v, hemi string, degDigits int) (float64, error) {
	if len(v) < degDigits+2 {
		return 0, fmt.Errorf("%q too short", v)
	}
	deg, err := strconv.ParseFloat(v[:degDigits], 64)
	if err != nil {
		return 0, err
	}
	min, err := strconv.ParseFloat(v[degDigits:], 64)
	if err != nil {
		return 0, err
	}
	d := deg + min/60
	switch hemi {
	case "S", "W":
		d = -d
	case "N", "E":
	default:
		return 0, fmt.Errorf("bad hemisphere %q", hemi)
	}
	return math.Round(d*1e6) / 1e6, nil
}

// Command builds a server-to-device command frame:
//
//	*HQ,<id>,<name>,<HHMMSS>[,<param>...]#
func Command(deviceID, name string, params []string, now time.Time) string {
	var b strings.Builder
	b.WriteString("*HQ,")
	b.WriteString(deviceID)
	b.WriteByte(',')
	b.WriteString(name)
	b.WriteByte(',')
	b.WriteString(now.UTC().Format("150405"))
	for _, p := range params {
		b.WriteByte(',')
		b.WriteString(p)
	}
	b.WriteByte('#')
	return b.String()
}
