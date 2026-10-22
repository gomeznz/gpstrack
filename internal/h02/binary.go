package h02

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
)

// BinaryMinLen is the length of the fixed position part of a binary H02
// report. 4G units (e.g. CJ730 / MV730 Plus) send longer frames with cell
// and extra data after it, which is kept in the raw log but not decoded yet.
const BinaryMinLen = 29

// Binary H02 layout (BCD unless noted):
//
//	0      '$'
//	1-5    device ID, 10 digits
//	6-8    time hhmmss (UTC)
//	9-11   date ddmmyy
//	12-15  latitude ddmm.mmmm
//	16     battery / unknown (raw byte)
//	17-21  longitude dddmm.mmmm in the first 9 digits; last nibble = flags
//	       0x2 valid fix, 0x4 north, 0x8 east
//	22-24  speed (3 digits, knots) and course (3 digits)
//	25-28  status, 32-bit, same negative logic as the text form

// IsBinary reports whether b looks like a binary H02 frame.
func IsBinary(b []byte) bool {
	return len(b) >= BinaryMinLen && b[0] == '$' && isBCD(b[1:6])
}

// SplitBinary splits a chunk that may hold several binary frames from the
// same device back to back, using "$" + device ID as the separator.
func SplitBinary(b []byte) [][]byte {
	if !IsBinary(b) {
		return [][]byte{b}
	}
	sep := b[:6]
	var out [][]byte
	for {
		next := bytes.Index(b[1:], sep)
		if next < 0 {
			return append(out, b)
		}
		out = append(out, b[:next+1])
		b = b[next+1:]
	}
}

// ParseBinary decodes the position part of one binary H02 frame.
func ParseBinary(b []byte) (*Position, error) {
	if !IsBinary(b) {
		return nil, fmt.Errorf("h02: not a binary frame")
	}
	digits := hex.EncodeToString(b[:BinaryMinLen])[2:] // drop '$'
	// digits offsets: id 0-9, time 10-15, date 16-21, lat 22-29, x 30-31,
	// lon 32-40 + flags 41, speed 42-44, course 45-47, status 48-55
	for _, r := range [][2]int{{0, 30}, {32, 41}, {42, 48}} {
		if !isDigits(digits[r[0]:r[1]]) {
			return nil, fmt.Errorf("h02: binary frame has non-BCD data at %d", r[0])
		}
	}
	ts, err := parseTime(digits[16:22], digits[10:16])
	if err != nil {
		return nil, err
	}
	flags, _ := strconv.ParseUint(digits[41:42], 16, 8)
	latH, lonH := "S", "W"
	if flags&0x4 != 0 {
		latH = "N"
	}
	if flags&0x8 != 0 {
		lonH = "E"
	}
	lat, err := parseCoord(digits[22:24]+digits[24:26]+"."+digits[26:30], latH, 2)
	if err != nil {
		return nil, fmt.Errorf("h02: latitude: %w", err)
	}
	lon, err := parseCoord(digits[32:35]+digits[35:37]+"."+digits[37:41], lonH, 3)
	if err != nil {
		return nil, fmt.Errorf("h02: longitude: %w", err)
	}
	knots, _ := strconv.ParseFloat(digits[42:45], 64)
	course, _ := strconv.ParseFloat(digits[45:48], 64)
	status := binary.BigEndian.Uint32(b[25:29])

	p := &Position{
		DeviceID:   digits[0:10],
		Time:       ts,
		Valid:      flags&0x2 != 0,
		Lat:        lat,
		Lon:        lon,
		SpeedKmh:   math.Round(knots*1.852*10) / 10,
		Course:     course,
		StatusHex:  fmt.Sprintf("%08X", status),
		ActiveBits: ActiveBits(status),
	}
	return p, nil
}

func isBCD(b []byte) bool {
	for _, c := range b {
		if c>>4 > 9 || c&0x0f > 9 {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
