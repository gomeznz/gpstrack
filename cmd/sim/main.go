// Command sim pretends to be a CJ720 so the gateway can be tested without a
// real unit. It drives a loop around central Invercargill, sends H02 V1
// reports, and answers S20 relay commands with a V4 reply. After a fuel cut
// the speed bleeds off over a few reports, like the lift-pump run-on.
//
//	go run ./cmd/sim -addr 127.0.0.1:5013 -id 4106054111
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"time"
)

// A rough loop: Tay St, Dee St, Gala St, Kelvin St (lat, lon).
var route = [][2]float64{
	{-46.4130, 168.3475}, {-46.4127, 168.3600}, {-46.4045, 168.3603},
	{-46.4040, 168.3480}, {-46.4130, 168.3475},
}

func main() {
	addr := flag.String("addr", "127.0.0.1:5013", "gateway TCP address")
	id := flag.String("id", "4106054111", "device ID to report as")
	every := flag.Duration("every", 5*time.Second, "report interval")
	flag.Parse()

	c, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("connected to %s as %s", *addr, *id)

	var mu sync.Mutex
	cut := false

	go func() {
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadString('#')
			if err != nil {
				log.Fatalf("gateway closed: %v", err)
			}
			line = strings.TrimSpace(line)
			log.Printf("<- %s", line)
			f := strings.Split(strings.Trim(line, "*#"), ",")
			if len(f) >= 5 && f[2] == "S20" {
				mu.Lock()
				cut = f[4] == "1" && (len(f) < 6 || f[5] != "0")
				state := map[bool]string{true: "CUT", false: "RESUME"}[cut]
				mu.Unlock()
				now := time.Now().UTC()
				reply := fmt.Sprintf("*HQ,%s,V4,S20,%s,%s,%s#", *id, state, now.Format("150405"), now.Format("020106"))
				c.Write([]byte(reply))
				log.Printf("-> %s", reply)
			}
		}
	}()

	const cruiseKmh = 45.0
	speed, seg, frac := cruiseKmh, 0, 0.0
	for range time.Tick(*every) {
		mu.Lock()
		isCut := cut
		mu.Unlock()
		if isCut {
			speed = math.Max(0, speed-15) // run-on: slow down over a few reports
		} else {
			speed = cruiseKmh
		}

		a, b := route[seg], route[seg+1]
		segKm := distKm(a, b)
		frac += speed * every.Hours() / segKm
		for frac >= 1 {
			frac--
			seg = (seg + 1) % (len(route) - 1)
			a, b = route[seg], route[seg+1]
		}
		lat, lon := a[0]+(b[0]-a[0])*frac, a[1]+(b[1]-a[1])*frac
		course := math.Mod(math.Atan2(b[1]-a[1], b[0]-a[0])*180/math.Pi+360, 360)

		status := uint32(0xFFFFFFFF)
		if speed > 0 {
			status &^= 1 << 10 // pretend bit 10 = ignition
		}
		if isCut {
			status &^= 1 << 13 // pretend bit 13 = relay cut
		}
		now := time.Now().UTC()
		frame := fmt.Sprintf("*HQ,%s,V1,%s,A,%s,S,%s,E,%06.2f,%03.0f,%s,%08X,530,01,0,0#",
			*id, now.Format("150405"), nmea(-lat, 2), nmea(lon, 3), speed/1.852, course, now.Format("020106"), status)
		if _, err := c.Write([]byte(frame)); err != nil {
			log.Fatal(err)
		}
		log.Printf("-> %s", frame)
	}
}

func nmea(deg float64, degDigits int) string {
	d := math.Floor(deg)
	return fmt.Sprintf("%0*d%07.4f", degDigits, int(d), (deg-d)*60)
}

func distKm(a, b [2]float64) float64 {
	dy := (b[0] - a[0]) * 111.32
	dx := (b[1] - a[1]) * 111.32 * math.Cos(a[0]*math.Pi/180)
	return math.Hypot(dx, dy)
}
