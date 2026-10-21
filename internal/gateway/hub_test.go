package gateway

import (
	"bufio"
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
