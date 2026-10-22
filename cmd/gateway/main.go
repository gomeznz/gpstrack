// Command gateway is the Phase 0 tracker gateway: it listens for H02
// trackers on TCP, logs all traffic, decodes positions and serves a status
// page with a command tester.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gomeznz/gpstrack/internal/gateway"
	"github.com/gomeznz/gpstrack/internal/store"
	"github.com/gomeznz/gpstrack/internal/web"
)

func main() {
	tcpAddr := flag.String("tcp", envOr("GPSTRACK_TCP", ":5013"), "tracker listen address")
	udpAddr := flag.String("udp", envOr("GPSTRACK_UDP", ":5013"), "tracker UDP listen address (empty to disable)")
	httpAddr := flag.String("http", envOr("GPSTRACK_HTTP", ":8080"), "status page listen address")
	dataDir := flag.String("data", envOr("GPSTRACK_DATA", "./data"), "directory for raw and position logs")
	flag.Parse()
	token := os.Getenv("GPSTRACK_TOKEN")

	raw, err := store.NewDailyJSONL(filepath.Join(*dataDir, "raw"), "raw")
	if err != nil {
		log.Fatal(err)
	}
	positions, err := store.NewDailyJSONL(filepath.Join(*dataDir, "positions"), "positions")
	if err != nil {
		log.Fatal(err)
	}
	hub := gateway.NewHub(raw, positions)

	ln, err := net.Listen("tcp", *tcpAddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := hub.Serve(ln); err != nil {
			log.Fatal(err)
		}
	}()

	var pc net.PacketConn
	if *udpAddr != "" {
		if pc, err = net.ListenPacket("udp", *udpAddr); err != nil {
			log.Fatal(err)
		}
		go func() {
			if err := hub.ServeUDP(pc); err != nil {
				log.Fatal(err)
			}
		}()
	}

	srv := &http.Server{
		Addr:              *httpAddr,
		Handler:           (&web.Server{Hub: hub, Token: token}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	log.Printf("trackers on tcp %s and udp %q, status page on http %s, data in %s", *tcpAddr, *udpAddr, *httpAddr, *dataDir)
	if token == "" {
		log.Printf("GPSTRACK_TOKEN not set: status page is open read-only and commands are disabled")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Print("shutting down")
	ln.Close()
	if pc != nil {
		pc.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	raw.Close()
	positions.Close()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
