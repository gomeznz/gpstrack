package h02

import (
	"encoding/hex"
	"testing"
	"time"
)

// Real frame from an MV730 Plus 4G posted on the Traccar forum.
const binarySample = "2473400069542126502402243349095900035294470e000000fffffbffff041f300000000001cc00ff00009d0243000000001a"

func TestParseBinary(t *testing.T) {
	b, _ := hex.DecodeString(binarySample)
	if !IsBinary(b) {
		t.Fatal("IsBinary false")
	}
	p, err := ParseBinary(b)
	if err != nil {
		t.Fatal(err)
	}
	if p.DeviceID != "7340006954" {
		t.Errorf("id %s", p.DeviceID)
	}
	if want := time.Date(2024, 2, 24, 21, 26, 50, 0, time.UTC); !p.Time.Equal(want) {
		t.Errorf("time %v", p.Time)
	}
	if !p.Valid || p.Lat != 33.818265 || p.Lon != 35.490783 {
		t.Errorf("fix %v %v %v", p.Valid, p.Lat, p.Lon)
	}
	if p.SpeedKmh != 0 || p.Course != 0 || p.StatusHex != "FFFFFBFF" {
		t.Errorf("speed %v course %v status %s", p.SpeedKmh, p.Course, p.StatusHex)
	}
}

func TestParseBinarySouthernMoving(t *testing.T) {
	// Invercargill: 46 24.5260 S, 168 21.4340 E, valid, 20 knots, course 90.
	b, _ := hex.DecodeString("24" + "4106054111" + "213045" + "221026" + "46245260" + "00" + "168214340a" + "020090" + "fffffbff")
	p, err := ParseBinary(b)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Valid || p.Lat != -46.408767 || p.Lon != 168.357233 || p.SpeedKmh != 37 || p.Course != 90 {
		t.Errorf("%+v", p)
	}
}

func TestSplitBinary(t *testing.T) {
	one, _ := hex.DecodeString(binarySample)
	two := append(append([]byte{}, one...), one...)
	parts := SplitBinary(two)
	if len(parts) != 2 || len(parts[0]) != len(one) || len(parts[1]) != len(one) {
		t.Fatalf("got %d parts", len(parts))
	}
	if _, err := ParseBinary([]byte("$notbcd-at-all-xxxxxxxxxxxxxxxxxxxx")); err == nil {
		t.Error("want error")
	}
}
