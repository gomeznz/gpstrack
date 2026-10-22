package gateway

import (
	"bufio"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gomeznz/gpstrack/internal/store"
)

func TestHubEndToEnd(t *testing.T) {
	dir := t.TempDir()
	raw, _ := store.NewDailyJSONL(dir, "raw")
	pos, _ := store.NewDailyJSONL(dir, "positions")
	h := NewHub(raw, pos)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go h.Serve(ln)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// A position split across two writes.
	frame := "*HQ,4106054111,V1,213045,A,4624.5260,S,16821.4340,E,020.00,090,211026,FFFFFBFF#"
	c.Write([]byte(frame[:30]))
	time.Sleep(20 * time.Millisecond)
	c.Write([]byte(frame[30:]))

	waitFor(t, func() bool { return len(h.Devices()) == 1 && h.Devices()[0].Position != nil })
	d := h.Devices()[0]
	if d.ID != "4106054111" || !d.Connected || d.Position.SpeedKmh != 37 {
		t.Fatalf("device %+v", d)
	}

	// Send a preset down the open session; the "tracker" should receive it.
	line, _ := BuildPreset("cut", d.ID, time.Date(2026, 10, 22, 1, 2, 3, 0, time.UTC))
	if err := h.Send(d.ID, line); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, err := bufio.NewReader(c).ReadString('#')
	if err != nil || got != "*HQ,4106054111,S20,010203,1,1#" {
		t.Fatalf("tracker got %q, %v", got, err)
	}

	// Device replies; it becomes LastReply.
	c.Write([]byte("*HQ,4106054111,V4,S20,DONE,010204,221026#"))
	waitFor(t, func() bool { return strings.Contains(h.Devices()[0].LastReply, "V4") })

	if err := h.Send("nope", "x"); err != ErrNotConnected {
		t.Errorf("unknown device: %v", err)
	}

	c.Close()
	waitFor(t, func() bool { return !h.Devices()[0].Connected })
	raw.Close()
	pos.Close()

	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(files) != 2 {
		t.Fatalf("log files %v", files)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "raw-"+time.Now().UTC().Format("2006-01-02")+".jsonl"))
	for _, want := range []string{`"dir":"in"`, `"dir":"out"`, "S20,010203", "disconnected"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("raw log missing %s", want)
		}
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestDescribeBinary(t *testing.T) {
	cases := map[string]string{
		"\x16\x03\x01\x02\x00": "HTTPS handshake",
		"GET / HTTP/1.1\r\n":   "plain HTTP",
		"SSH-2.0-OpenSSH":      "SSH",
		"$\x41\x06":            "binary H02",
		"TRVAP00353":           "TRV",
		"\x00\x01":             "unrecognised",
	}
	for in, want := range cases {
		if got := describeBinary([]byte(in)); !strings.Contains(got, want) {
			t.Errorf("%q: %q", in, got)
		}
	}
}

// 4G units (CJ730 / MV730 Plus) send binary H02. Sample from the Traccar forum.
const binarySample = "2473400069542126502402243349095900035294470e000000fffffbffff041f300000000001cc00ff00009d0243000000001a"

func TestBinaryOverTCPAndUDP(t *testing.T) {
	h := NewHub(nil, nil)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go h.Serve(ln)
	pc, _ := net.ListenPacket("udp", "127.0.0.1:0")
	defer pc.Close()
	go h.ServeUDP(pc)

	frame, _ := hex.DecodeString(binarySample)

	// TCP: two frames back to back, the first split mid-way.
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write(frame[:10])
	time.Sleep(20 * time.Millisecond)
	c.Write(append(append([]byte{}, frame[10:]...), frame...))
	waitFor(t, func() bool { d := h.Devices(); return len(d) == 1 && d[0].Messages == 2 })
	d := h.Devices()[0]
	if d.ID != "7340006954" || !d.Connected || d.Position == nil || d.Position.Lat != 33.818265 {
		t.Fatalf("tcp device %+v", d)
	}

	// UDP: same device ID from a datagram counts as another message.
	u, err := net.Dial("udp", pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	u.Write(frame)
	u.Write([]byte("*HQ,4106054111,V1,213045,A,4624.5260,S,16821.4340,E,0,0,211026,FFFFFFFF#"))
	waitFor(t, func() bool { return len(h.Devices()) == 2 })
	for _, d := range h.Devices() {
		if d.ID == "4106054111" && (d.Connected || !strings.HasPrefix(d.Remote, "udp:")) {
			t.Errorf("udp device %+v", d)
		}
	}
}
