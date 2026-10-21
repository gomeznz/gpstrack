package h02

import (
	"testing"
	"time"
)

// Invercargill, moving at 20 knots.
const sampleV1 = "*HQ,4106054111,V1,213045,A,4624.5260,S,16821.4340,E,020.00,090,211026,FFFFFBFF,530,01,1234,5678#"

func TestParseV1(t *testing.T) {
	m, err := ParseText(sampleV1)
	if err != nil {
		t.Fatal(err)
	}
	if m.DeviceID != "4106054111" || m.Type != "V1" || m.Maker != "HQ" {
		t.Fatalf("header: %+v", m)
	}
	p, err := m.Position()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 21, 21, 30, 45, 0, time.UTC)
	if !p.Time.Equal(want) {
		t.Errorf("time %v, want %v", p.Time, want)
	}
	if !p.Valid {
		t.Error("want valid fix")
	}
	if p.Lat != -46.408767 || p.Lon != 168.357233 {
		t.Errorf("lat/lon %v %v", p.Lat, p.Lon)
	}
	if p.SpeedKmh != 37 {
		t.Errorf("speed %v km/h, want 37", p.SpeedKmh)
	}
	if p.Course != 90 {
		t.Errorf("course %v", p.Course)
	}
	if p.StatusHex != "FFFFFBFF" || len(p.ActiveBits) != 1 || p.ActiveBits[0] != 10 {
		t.Errorf("status %s bits %v", p.StatusHex, p.ActiveBits)
	}
	if p.Cell != "530,01,1234,5678" {
		t.Errorf("cell %q", p.Cell)
	}
}

func TestParseV1NoFixEmptySpeed(t *testing.T) {
	m, err := ParseText("*HQ,1234567890,V1,000102.000,V,4624.5260,S,16821.4340,E,,,010126,FFFFFFFF#")
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.Position()
	if err != nil {
		t.Fatal(err)
	}
	if p.Valid || p.SpeedKmh != 0 || len(p.ActiveBits) != 0 || p.Cell != "" {
		t.Errorf("%+v", p)
	}
}

func TestNonPositionAndBadFrames(t *testing.T) {
	m, err := ParseText("*HQ,4106054111,V4,S20,DONE,213050,211026#")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Position(); err != ErrNotPosition {
		t.Errorf("V4: got %v", err)
	}
	if m.Fields[0] != "S20" {
		t.Errorf("fields %v", m.Fields)
	}
	for _, bad := range []string{"", "HQ,1,V1#", "*HQ#", "*HQ,,V1#"} {
		if _, err := ParseText(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
	m, _ = ParseText("*HQ,1,V1,213045,A,4624.5260,X,16821.4340,E,0,0,211026,FFFFFFFF#")
	if _, err := m.Position(); err == nil {
		t.Error("bad hemisphere: want error")
	}
}

func TestCommand(t *testing.T) {
	now := time.Date(2026, 10, 22, 1, 2, 3, 0, time.UTC)
	got := Command("4106054111", "S20", []string{"1", "1"}, now)
	if got != "*HQ,4106054111,S20,010203,1,1#" {
		t.Errorf("got %s", got)
	}
}

func TestFramer(t *testing.T) {
	var f Framer
	// Split mid-frame, two frames in one read, then trailing CRLF and binary.
	if out := f.Push([]byte(sampleV1[:20])); len(out) != 0 {
		t.Fatalf("partial: %v", out)
	}
	out := f.Push([]byte(sampleV1[20:] + "\r\n*HQ,1,LINK#"))
	if len(out) != 2 || out[0].Text != sampleV1 || out[1].Text != "*HQ,1,LINK#" {
		t.Fatalf("got %+v", out)
	}
	out = f.Push([]byte("\r\n$\x41\x06\x05\x41\x11*HQ,1,LINK#"))
	if len(out) != 2 || out[0].Binary == nil || out[0].Binary[0] != '$' || out[1].Text != "*HQ,1,LINK#" {
		t.Fatalf("binary: %+v", out)
	}
}

func TestFramerOverflow(t *testing.T) {
	var f Framer
	junk := make([]byte, MaxTextFrame+10)
	junk[0] = '*'
	out := f.Push(junk)
	if len(out) != 1 || len(out[0].Binary) != len(junk) {
		t.Fatalf("got %d frames", len(out))
	}
}
